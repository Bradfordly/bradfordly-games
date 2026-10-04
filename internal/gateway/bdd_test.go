package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

// TestBDD is the local BDD environment for issue #61.
// Scenarios live in features/minecraft_wake.feature.
func TestBDD(t *testing.T) {
	feature := readFeature(t)
	requireScenario(t, feature, "status ping does not start a container")
	requireScenario(t, feature, "login starts the matched world")
	requireScenario(t, feature, "wake whitelist rejects an unknown player")
	requireScenario(t, feature, "one start per world per 30 seconds")
	requireScenario(t, feature, "do not start a second world while another is running")
	requireScenario(t, feature, "healthz and worlds on the localhost admin port")

	t.Run("status ping does not start a container", func(t *testing.T) {
		w := newBDDWorld(t)
		w.givenAsleep(t, "survival", "survival.games.bradfordly.com")
		w.whenStatus(t, "survival")
		w.thenNoStart(t)
		w.thenState(t, "survival", adapter.StateAsleep)
	})
	t.Run("login starts the matched world", func(t *testing.T) {
		w := newBDDWorld(t)
		w.givenAsleep(t, "survival", "survival.games.bradfordly.com")
		w.whenLogin(t, "Steve", "survival")
		w.thenStarted(t, "survival")
		w.thenState(t, "survival", adapter.StateStarting)
		if w.last.Occupy != adapter.OccupyKick {
			t.Fatal("occupy must be kick")
		}
		if adapter.StartingMessage(w.world("survival")) == "" {
			t.Fatal("starting message")
		}
	})
	t.Run("wake whitelist rejects an unknown player", func(t *testing.T) {
		w := newBDDWorld(t)
		w.givenAsleep(t, "survival", "survival.games.bradfordly.com")
		w.world("survival").WakeWhitelist = []string{"Steve"}
		w.whenLogin(t, "Alex", "survival")
		w.thenNoStart(t)
		w.thenState(t, "survival", adapter.StateAsleep)
	})
	t.Run("one start per world per 30 seconds", func(t *testing.T) {
		w := newBDDWorld(t)
		w.givenAsleep(t, "survival", "survival.games.bradfordly.com")
		now := time.Unix(1_700_000_000, 0)
		w.rt.now = func() time.Time { return now }
		w.whenLogin(t, "Steve", "survival")
		w.rt.setState("survival", adapter.StateAsleep)
		w.whenLogin(t, "Steve", "survival")
		if !errors.Is(w.last.Err, ErrRateLimited) {
			t.Fatalf("want rate limit, got %v", w.last.Err)
		}
	})
	t.Run("do not start a second world while another is running", func(t *testing.T) {
		w := newBDDWorld(t)
		w.givenAsleep(t, "survival", "survival.games.bradfordly.com")
		w.givenAsleep(t, "creative", "creative.games.bradfordly.com")
		w.whenLogin(t, "Steve", "survival")
		w.whenLogin(t, "Alex", "creative")
		w.thenStarted(t, "survival")
		for _, name := range w.engine.Starts() {
			if name == "creative" {
				t.Fatal("creative must not start")
			}
		}
	})
	t.Run("healthz and worlds on the localhost admin port", func(t *testing.T) {
		w := newBDDWorld(t)
		w.givenAsleep(t, "survival", "survival.games.bradfordly.com")
		if rec := w.admin(http.MethodGet, "/healthz", nil); rec.Code != http.StatusOK {
			t.Fatalf("healthz %d", rec.Code)
		}
		rec := w.admin(http.MethodGet, "/worlds", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("worlds %d", rec.Code)
		}
		var snaps []Snapshot
		if err := json.NewDecoder(rec.Body).Decode(&snaps); err != nil {
			t.Fatal(err)
		}
		if len(snaps) != 1 || snaps[0].ID != "survival" || snaps[0].State != adapter.StateAsleep {
			t.Fatalf("snaps = %#v", snaps)
		}
		if !AdminIsLocalhost(DefaultAdminAddr) || AdminIsPlayerPort(DefaultAdminAddr) {
			t.Fatal("admin must be localhost and not 25565")
		}
	})
}

type bddWorld struct {
	engine *MemoryEngine
	rt     *Runtime
	list   []*adapter.World
	last   Result
}

func newBDDWorld(t *testing.T) *bddWorld {
	t.Helper()
	return &bddWorld{engine: NewMemoryEngine()}
}

func (w *bddWorld) givenAsleep(t *testing.T, id, host string) {
	t.Helper()
	world := &adapter.World{
		ID:           id,
		Game:         adapter.GameMinecraftJava,
		Allocation:   adapter.Allocation{Host: host, Port: 25565, Protocol: "tcp"},
		Backend:      adapter.Backend{Container: id, Address: id + ":25565"},
		StartingMOTD: adapter.DefaultStartingMOTD,
	}
	w.list = append(w.list, world)
	w.rt = NewRuntime(w.list, w.engine)
}

func (w *bddWorld) world(id string) *adapter.World {
	for _, world := range w.list {
		if world.ID == id {
			return world
		}
	}
	return nil
}

func (w *bddWorld) whenStatus(t *testing.T, id string) {
	t.Helper()
	w.last = w.rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentStatus, World: w.world(id)})
}

func (w *bddWorld) whenLogin(t *testing.T, player, id string) {
	t.Helper()
	w.last = w.rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: player, World: w.world(id)})
}

func (w *bddWorld) thenNoStart(t *testing.T) {
	t.Helper()
	if len(w.engine.Starts()) != 0 {
		t.Fatalf("starts = %v", w.engine.Starts())
	}
}

func (w *bddWorld) thenStarted(t *testing.T, id string) {
	t.Helper()
	for _, name := range w.engine.Starts() {
		if name == id {
			return
		}
	}
	t.Fatalf("missing start %s in %v", id, w.engine.Starts())
}

func (w *bddWorld) thenState(t *testing.T, id string, want adapter.WorldState) {
	t.Helper()
	if got := w.rt.stateOf(id); got != want {
		t.Fatalf("state = %s, want %s", got, want)
	}
}

func (w *bddWorld) admin(method, path string, body []byte) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(string(body)))
	AdminMux(w.rt).ServeHTTP(rec, req)
	return rec
}

func readFeature(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "features", "minecraft_wake.feature")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func requireScenario(t *testing.T, feature, name string) {
	t.Helper()
	if !strings.Contains(feature, "Scenario: "+name) {
		t.Fatalf("feature file missing %q", name)
	}
}
