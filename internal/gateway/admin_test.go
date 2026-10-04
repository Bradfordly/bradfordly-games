package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	AdminMux(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d", rec.Code)
	}
}

func TestWorldsEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/worlds", nil)
	AdminMux(NewRuntime(nil, NewMemoryEngine())).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var worlds []Snapshot
	if err := json.NewDecoder(rec.Body).Decode(&worlds); err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 0 {
		t.Fatalf("got %d worlds", len(worlds))
	}
}

func TestWorldsAfterLogin(t *testing.T) {
	world := survival()
	rt := NewRuntime([]*adapter.World{world}, NewMemoryEngine())
	_ = rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: world})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/worlds", nil)
	AdminMux(rt).ServeHTTP(rec, req)
	var worlds []Snapshot
	if err := json.NewDecoder(rec.Body).Decode(&worlds); err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 1 || worlds[0].State != adapter.StateStarting {
		t.Fatalf("worlds = %#v", worlds)
	}
	if worlds[0].LastWakeCause != "minecraft_login:Steve" {
		t.Fatalf("cause = %q", worlds[0].LastWakeCause)
	}
}

func TestScaleWebhookStartsOnLogin(t *testing.T) {
	engine := NewMemoryEngine()
	rt := NewRuntime([]*adapter.World{survival()}, engine)
	body, _ := json.Marshal(ScaleRequest{Action: "up", ServerAddress: "survival.games.bradfordly.com", Backend: "survival:25565"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/scale", bytes.NewReader(body))
	AdminMux(rt).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if len(engine.Starts()) != 1 {
		t.Fatalf("starts = %v", engine.Starts())
	}
}

func TestScaleDownIsNoop(t *testing.T) {
	engine := NewMemoryEngine()
	rt := NewRuntime([]*adapter.World{survival()}, engine)
	body, _ := json.Marshal(ScaleRequest{Action: "down", ServerAddress: "survival.games.bradfordly.com"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/scale", bytes.NewReader(body))
	AdminMux(rt).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(engine.Starts()) != 0 {
		t.Fatal("down must not start")
	}
}

func TestScaleUnknownWorld(t *testing.T) {
	rt := NewRuntime([]*adapter.World{survival()}, NewMemoryEngine())
	body, _ := json.Marshal(ScaleRequest{Action: "up", ServerAddress: "missing.example"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/scale", bytes.NewReader(body))
	AdminMux(rt).ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestScaleInvalidJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/scale", bytes.NewReader([]byte("{")))
	AdminMux(NewRuntime(nil, NewMemoryEngine())).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestScaleUnsupportedAction(t *testing.T) {
	body, _ := json.Marshal(ScaleRequest{Action: "sideways"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/scale", bytes.NewReader(body))
	AdminMux(NewRuntime([]*adapter.World{survival()}, NewMemoryEngine())).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestScaleRateLimited(t *testing.T) {
	engine := NewMemoryEngine()
	world := survival()
	rt := NewRuntime([]*adapter.World{world}, engine)
	now := time.Unix(1_700_000_000, 0)
	rt.now = func() time.Time { return now }
	_ = rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: world})
	rt.setState(world.ID, adapter.StateAsleep)
	body, _ := json.Marshal(ScaleRequest{Action: "up", ServerAddress: "survival.games.bradfordly.com"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/scale", bytes.NewReader(body))
	AdminMux(rt).ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestScaleBusyWorld(t *testing.T) {
	engine := NewMemoryEngine()
	a, b := survival(), creative()
	rt := NewRuntime([]*adapter.World{a, b}, engine)
	_ = rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: a})
	body, _ := json.Marshal(ScaleRequest{Action: "up", ServerAddress: "creative.games.bradfordly.com"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/internal/scale", bytes.NewReader(body))
	AdminMux(rt).ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAdminBindRules(t *testing.T) {
	if !AdminIsLocalhost("127.0.0.1:8080") {
		t.Fatal("127.0.0.1 is localhost")
	}
	if AdminIsLocalhost(":8080") {
		t.Fatal("wildcard must not pass")
	}
	if AdminIsLocalhost("0.0.0.0:8080") {
		t.Fatal("0.0.0.0 must not pass")
	}
	if !AdminIsPlayerPort("127.0.0.1:25565") {
		t.Fatal("25565 is the player port")
	}
	if !AdminIsLocalhost("localhost:8080") {
		t.Fatal("localhost name")
	}
}
