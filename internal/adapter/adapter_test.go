package adapter

import (
	"bytes"
	"testing"

	"github.com/bradfordly/bradfordly-games/internal/mcproto"
)

func TestRegistryGames(t *testing.T) {
	r := NewRegistry()

	if _, ok := r.ForGame(GameMinecraftJava); !ok {
		t.Fatal("missing minecraft-java adapter")
	}
	if _, ok := r.ForGame(GameValheim); !ok {
		t.Fatal("missing valheim adapter")
	}
	if _, ok := r.ForGame(GamePalworld); !ok {
		t.Fatal("missing palworld adapter")
	}
}

func TestMinecraftShouldWake(t *testing.T) {
	a := MinecraftJava()
	world := &World{ID: "survival", Game: GameMinecraftJava}

	if a.ShouldWake(Event{Intent: IntentStatus, World: world}) {
		t.Fatal("status must not wake")
	}
	if a.ShouldWake(Event{Intent: IntentOther, World: world}) {
		t.Fatal("unclassified noise must not wake")
	}
	if !a.ShouldWake(Event{Intent: IntentLogin, World: world}) {
		t.Fatal("login must be allowed to wake")
	}

	listed := &World{ID: "survival", Game: GameMinecraftJava, WakeWhitelist: []string{"Alex"}}
	if a.ShouldWake(Event{Intent: IntentLogin, Player: "bob", World: listed}) {
		t.Fatal("name not on wake_whitelist must not wake")
	}
	if !a.ShouldWake(Event{Intent: IntentLogin, Player: "alex", World: listed}) {
		t.Fatal("whitelisted name may wake")
	}
}

func TestMinecraftClassifyHandshake(t *testing.T) {
	a := MinecraftJava()
	var status, login bytes.Buffer
	if err := mcproto.WriteHandshake(&status, mcproto.Handshake{
		ProtocolVersion: 767, ServerAddress: "localhost", ServerPort: 25565, NextState: mcproto.NextStateStatus,
	}); err != nil {
		t.Fatal(err)
	}
	if err := mcproto.WriteHandshake(&login, mcproto.Handshake{
		ProtocolVersion: 767, ServerAddress: "localhost", ServerPort: 25565, NextState: mcproto.NextStateLogin,
	}); err != nil {
		t.Fatal(err)
	}
	if a.Classify(status.Bytes()) != IntentStatus {
		t.Fatal("intent 1 is status")
	}
	if a.Classify(login.Bytes()) != IntentLogin {
		t.Fatal("intent 2 is login")
	}
	if a.Classify([]byte{0x00}) != IntentOther {
		t.Fatal("garbage is other")
	}
}

func TestMinecraftMatchAndDefaults(t *testing.T) {
	a := MinecraftJava()
	world := &World{
		ID:         "survival",
		Game:       GameMinecraftJava,
		Allocation: Allocation{Host: "survival.games.bradfordly.com", Port: 25565, Protocol: "tcp"},
	}

	got := a.Match([]*World{world}, Allocation{Host: "survival.games.bradfordly.com"}, nil)
	if got != world {
		t.Fatalf("Match = %#v, want survival world", got)
	}
	if a.Match([]*World{world}, Allocation{Host: "other.example"}, nil) != nil {
		t.Fatal("Match should miss a different hostname")
	}
	if a.Occupy(world, StateAsleep) != OccupyKick {
		t.Fatal("minecraft occupy default is kick")
	}
	if players, unknown := a.Activity(world); players != 0 || unknown {
		t.Fatalf("Activity = %d, unknown=%v; want 0, known", players, unknown)
	}
	if err := a.GracefulStop(world); err != nil {
		t.Fatalf("GracefulStop stub: %v", err)
	}
}

func TestUDPNotImplemented(t *testing.T) {
	for _, game := range []string{GameValheim, GamePalworld} {
		a, ok := NewRegistry().ForGame(game)
		if !ok {
			t.Fatalf("missing %s", game)
		}
		if a.ShouldWake(Event{Intent: IntentLogin}) {
			t.Fatalf("%s must not wake", game)
		}
		if err := a.ServeStatus(nil, nil, StateAsleep, 0); err != ErrNotImplemented {
			t.Fatalf("%s ServeStatus = %v, want ErrNotImplemented", game, err)
		}
		if err := a.GracefulStop(nil); err != ErrNotImplemented {
			t.Fatalf("%s GracefulStop = %v, want ErrNotImplemented", game, err)
		}
		if a.Occupy(nil, StateAsleep) != OccupyRetry {
			t.Fatalf("%s occupy must be retry", game)
		}
		if _, unknown := a.Activity(nil); !unknown {
			t.Fatalf("%s Activity must be unknown", game)
		}
	}
}
