package adapter

import "testing"

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

func TestMinecraftActivityFromPlayCounts(t *testing.T) {
	plays := NewPlayCounts()
	a := NewMinecraftJava(plays)
	world := &World{ID: "survival", Game: GameMinecraftJava}

	plays.Set("survival", 3)
	players, unknown := a.Activity(world)
	if players != 3 || unknown {
		t.Fatalf("Activity = %d, unknown=%v; want 3, known", players, unknown)
	}

	plays.Set("survival", 0)
	players, unknown = a.Activity(world)
	if players != 0 || unknown {
		t.Fatalf("Activity after empty = %d, unknown=%v; want 0, known", players, unknown)
	}

	if n, unknown := NewPlayCounts().Get("missing"); n != 0 || unknown {
		t.Fatalf("missing world Get = %d, unknown=%v; want 0, known", n, unknown)
	}

	var none *PlayCounts
	none.Set("survival", 1)
	if n, unknown := none.Get("survival"); n != 0 || unknown {
		t.Fatalf("nil PlayCounts Get = %d, unknown=%v", n, unknown)
	}
	empty := &PlayCounts{}
	empty.Set("survival", 2)
	if n, unknown := empty.Get("survival"); n != 2 || unknown {
		t.Fatalf("lazy PlayCounts = %d, unknown=%v", n, unknown)
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
		if err := a.ServeStatus(nil, nil, StateAsleep); err != ErrNotImplemented {
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
