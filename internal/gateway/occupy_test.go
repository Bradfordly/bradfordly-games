package gateway

import (
	"bufio"
	"testing"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
	"github.com/bradfordly/bradfordly-games/internal/mcproto"
)

func TestKickWhenStarting(t *testing.T) {
	world := &adapter.World{
		ID:           "survival",
		Game:         adapter.GameMinecraftJava,
		Allocation:   adapter.Allocation{Host: "survival.games.bradfordly.com"},
		State:        adapter.StateAsleep,
		Backend:      "127.0.0.1:1",
		StartTimeout: time.Second,
		OccupyMode:   adapter.OccupyKick,
	}
	srv, addr := startTestServer(t, world)
	conn := loginAndMaybeProxy(t, addr, "survival.games.bradfordly.com", "alex")
	defer conn.Close()

	reason, err := mcproto.ReadDisconnectText(bufio.NewReader(conn))
	if err != nil {
		t.Fatal(err)
	}
	if reason != mcproto.StartingKickMessage {
		t.Fatalf("kick = %q", reason)
	}
	time.Sleep(150 * time.Millisecond)
	got := srv.Catalog.Get("survival")
	if got.State != adapter.StateStarting || got.Replicas != 1 {
		t.Fatalf("kick should leave world starting: %#v", got)
	}
	if srv.Scaler.(*RecordingScaler).Count() != 1 {
		t.Fatalf("kick must not scale back to 0, calls=%#v", srv.Scaler.(*RecordingScaler).Calls)
	}
}

func TestWhitelistBlocksWake(t *testing.T) {
	world := &adapter.World{
		ID:            "survival",
		Game:          adapter.GameMinecraftJava,
		Allocation:    adapter.Allocation{Host: "survival.games.bradfordly.com"},
		State:         adapter.StateAsleep,
		Backend:       "127.0.0.1:1",
		StartTimeout:  80 * time.Millisecond,
		WakeWhitelist: []string{"alex"},
	}
	srv, addr := startTestServer(t, world)
	conn := loginAndMaybeProxy(t, addr, "survival.games.bradfordly.com", "bob")
	defer conn.Close()
	time.Sleep(50 * time.Millisecond)
	if srv.Scaler.(*RecordingScaler).Count() != 0 {
		t.Fatalf("non-whitelisted login scaled: %#v", srv.Scaler.(*RecordingScaler).Calls)
	}
	if got := srv.Catalog.Get("survival"); got.State != adapter.StateAsleep {
		t.Fatalf("state = %s, want asleep", got.State)
	}
}

func TestWhitelistAllowsWake(t *testing.T) {
	world := &adapter.World{
		ID:            "survival",
		Game:          adapter.GameMinecraftJava,
		Allocation:    adapter.Allocation{Host: "survival.games.bradfordly.com"},
		State:         adapter.StateAsleep,
		Backend:       "127.0.0.1:1",
		StartTimeout:  80 * time.Millisecond,
		OccupyMode:    adapter.OccupyHold,
		WakeWhitelist: []string{"Alex"},
	}
	srv, addr := startTestServer(t, world)
	conn := loginAndMaybeProxy(t, addr, "survival.games.bradfordly.com", "alex")
	defer conn.Close()
	time.Sleep(200 * time.Millisecond)
	if srv.Scaler.(*RecordingScaler).Count() < 1 || srv.Scaler.(*RecordingScaler).Calls[0].Replicas != 1 {
		t.Fatalf("whitelisted login should scale: %#v", srv.Scaler.(*RecordingScaler).Calls)
	}
}
