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

	listed := &World{ID: "survival", Game: GameMinecraftJava, WakeWhitelist: []string{"Steve"}}
	if a.ShouldWake(Event{Intent: IntentLogin, World: listed, Player: "Alex"}) {
		t.Fatal("non-whitelisted login must not wake")
	}
	if !a.ShouldWake(Event{Intent: IntentLogin, World: listed, Player: "steve"}) {
		t.Fatal("whitelisted login must wake")
	}
	if !a.ShouldWake(Event{Intent: IntentLogin, World: listed, WhitelistChecked: true, Player: "Alex"}) {
		t.Fatal("mc-router already applied the whitelist")
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
	if a.Occupy(nil, StateAsleep) != OccupyKick {
		t.Fatal("nil world occupy is kick")
	}
	if a.Occupy(world, StateAsleep) != OccupyKick {
		t.Fatal("minecraft occupy default is kick")
	}
	hold := *world
	hold.OccupyMode = OccupyModeHold
	if a.Occupy(&hold, StateStarting) != OccupyHold {
		t.Fatal("occupy_mode hold must hold")
	}
	retry := *world
	retry.OccupyMode = OccupyModeRetry
	if a.Occupy(&retry, StateStarting) != OccupyRetry {
		t.Fatal("occupy_mode retry must retry")
	}
	if a.Classify(nil) != IntentOther {
		t.Fatal("Classify must not parse a handshake")
	}
	if err := a.ServeStatus(nil, world, StateAsleep); err != nil {
		t.Fatal(err)
	}
	if StartingMessage(nil) != DefaultStartingMOTD {
		t.Fatal("default starting message")
	}
	world.StartingMOTD = "booting"
	if StartingMessage(world) != "booting" {
		t.Fatal("custom starting message")
	}
	if WakeAllowed(nil, "Steve") != true {
		t.Fatal("nil world allows wake")
	}
	listed := &World{WakeWhitelist: []string{"Steve"}}
	if WakeAllowed(listed, "") {
		t.Fatal("empty player is not on the whitelist")
	}
	if a.Match([]*World{nil, world}, Allocation{Host: "other"}, nil) != nil {
		t.Fatal("nil and mismatch")
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
		if a.Match(nil, Allocation{}, nil) != nil {
			t.Fatalf("%s Match must miss", game)
		}
		if a.Classify(nil) != IntentOther {
			t.Fatalf("%s Classify must be other", game)
		}
	}
}
