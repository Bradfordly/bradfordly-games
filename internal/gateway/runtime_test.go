package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

func survival() *adapter.World {
	return &adapter.World{
		ID:         "survival",
		Game:       adapter.GameMinecraftJava,
		Allocation: adapter.Allocation{Host: "survival.games.bradfordly.com", Port: 25565, Protocol: "tcp"},
		Backend:    adapter.Backend{Container: "survival", Address: "survival:25565"},
	}
}

func creative() *adapter.World {
	return &adapter.World{
		ID:         "creative",
		Game:       adapter.GameMinecraftJava,
		Allocation: adapter.Allocation{Host: "creative.games.bradfordly.com", Port: 25565, Protocol: "tcp"},
		Backend:    adapter.Backend{Container: "creative", Address: "creative:25565"},
	}
}

func TestStatusDoesNotStart(t *testing.T) {
	engine := NewMemoryEngine()
	rt := NewRuntime([]*adapter.World{survival()}, engine)

	got := rt.Handle(context.Background(), adapter.Event{
		Intent: adapter.IntentStatus,
		World:  survival(),
	})
	if got.Err != nil {
		t.Fatalf("status: %v", got.Err)
	}
	if got.State != adapter.StateAsleep {
		t.Fatalf("state = %s, want asleep", got.State)
	}
	if len(engine.Starts()) != 0 {
		t.Fatalf("docker start called on status: %v", engine.Starts())
	}
}

func TestLoginStartsMatchedWorld(t *testing.T) {
	engine := NewMemoryEngine()
	world := survival()
	rt := NewRuntime([]*adapter.World{world}, engine)

	got := rt.Handle(context.Background(), adapter.Event{
		Intent: adapter.IntentLogin,
		Player: "Steve",
		World:  world,
	})
	if got.Err != nil {
		t.Fatalf("login: %v", got.Err)
	}
	if !got.Started {
		t.Fatal("expected docker start")
	}
	if got.State != adapter.StateStarting {
		t.Fatalf("state = %s, want starting", got.State)
	}
	if got.Occupy != adapter.OccupyKick {
		t.Fatal("default occupy is kick")
	}
	if got.WakeCause != "minecraft_login:Steve" {
		t.Fatalf("cause = %q", got.WakeCause)
	}
	if starts := engine.Starts(); len(starts) != 1 || starts[0] != "survival" {
		t.Fatalf("starts = %v", starts)
	}
}

func TestWakeWhitelist(t *testing.T) {
	engine := NewMemoryEngine()
	world := survival()
	world.WakeWhitelist = []string{"Steve"}
	rt := NewRuntime([]*adapter.World{world}, engine)

	got := rt.Handle(context.Background(), adapter.Event{
		Intent: adapter.IntentLogin,
		Player: "Alex",
		World:  world,
	})
	if !errors.Is(got.Err, ErrWhitelist) {
		t.Fatalf("err = %v, want whitelist", got.Err)
	}
	if len(engine.Starts()) != 0 {
		t.Fatal("whitelisted reject must not start")
	}
}

func TestRateLimit(t *testing.T) {
	engine := NewMemoryEngine()
	world := survival()
	rt := NewRuntime([]*adapter.World{world}, engine)
	now := time.Unix(1_700_000_000, 0)
	rt.now = func() time.Time { return now }

	first := rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: world})
	if first.Err != nil {
		t.Fatal(first.Err)
	}
	rt.setState(world.ID, adapter.StateAsleep)
	second := rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: world})
	if !errors.Is(second.Err, ErrRateLimited) {
		t.Fatalf("err = %v, want rate limit", second.Err)
	}

	now = now.Add(StartCooldown)
	third := rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: world})
	if third.Err != nil {
		t.Fatalf("after cooldown: %v", third.Err)
	}
}

func TestOneWorldAtATime(t *testing.T) {
	engine := NewMemoryEngine()
	a, b := survival(), creative()
	rt := NewRuntime([]*adapter.World{a, b}, engine)

	if got := rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: a}); got.Err != nil {
		t.Fatal(got.Err)
	}
	got := rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Alex", World: b})
	if !errors.Is(got.Err, ErrBusy) {
		t.Fatalf("err = %v, want busy", got.Err)
	}
	if starts := engine.Starts(); len(starts) != 1 {
		t.Fatalf("starts = %v", starts)
	}
}

func TestOccupyHoldMarksOnline(t *testing.T) {
	engine := NewMemoryEngine()
	world := survival()
	world.OccupyMode = adapter.OccupyModeHold
	rt := NewRuntime([]*adapter.World{world}, engine)
	rt.holdWait = time.Second

	got := rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: world})
	if got.Err != nil {
		t.Fatal(got.Err)
	}
	if got.Occupy != adapter.OccupyHold {
		t.Fatal("want hold")
	}
	if got.State != adapter.StateOnline {
		t.Fatalf("state = %s, want online", got.State)
	}
}

func TestRebuildRunningAsOnline(t *testing.T) {
	engine := NewMemoryEngine("survival")
	rt := NewRuntime([]*adapter.World{survival()}, engine)
	if rt.stateOf("survival") != adapter.StateOnline {
		t.Fatalf("state = %s, want online", rt.stateOf("survival"))
	}
}

func TestHandleNoWorld(t *testing.T) {
	rt := NewRuntime(nil, NewMemoryEngine())
	got := rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin})
	if !errors.Is(got.Err, ErrNoWorld) {
		t.Fatalf("err = %v", got.Err)
	}
}

func TestWorldByHost(t *testing.T) {
	rt := NewRuntime([]*adapter.World{survival()}, NewMemoryEngine())
	if rt.WorldByHost("Survival.games.bradfordly.com.") == nil {
		t.Fatal("expected host match")
	}
	if rt.WorldByHost("nope") != nil {
		t.Fatal("unexpected match")
	}
}

func TestRepeatLoginWhileStarting(t *testing.T) {
	engine := NewMemoryEngine()
	world := survival()
	rt := NewRuntime([]*adapter.World{world}, engine)
	_ = rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: world})
	got := rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: world})
	if got.Err != nil || got.Started {
		t.Fatalf("second login while starting: %#v", got)
	}
	if len(engine.Starts()) != 1 {
		t.Fatalf("starts = %v", engine.Starts())
	}
}

func TestStartFailure(t *testing.T) {
	engine := NewMemoryEngine()
	engine.startFn = func(string) error { return errors.New("boom") }
	world := survival()
	rt := NewRuntime([]*adapter.World{world}, engine)
	got := rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: world})
	if !errors.Is(got.Err, ErrStartFailed) {
		t.Fatalf("err = %v", got.Err)
	}
	if got.State != adapter.StateFailed {
		t.Fatalf("state = %s", got.State)
	}
}

func TestMissingContainerEngine(t *testing.T) {
	world := survival()
	rt := NewRuntime([]*adapter.World{world}, nil)
	got := rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: world})
	if !errors.Is(got.Err, ErrStartFailed) {
		t.Fatalf("err = %v", got.Err)
	}
}

type stuckEngine struct {
	starts []string
}

func (s *stuckEngine) Start(_ context.Context, container string) error {
	s.starts = append(s.starts, container)
	return nil
}

func (*stuckEngine) Running(context.Context, string) (bool, error) { return false, nil }

func (*stuckEngine) ListRunning(context.Context) ([]string, error) { return nil, nil }

func TestHoldTimeout(t *testing.T) {
	engine := &stuckEngine{}
	world := survival()
	world.OccupyMode = adapter.OccupyModeHold
	rt := NewRuntime([]*adapter.World{world}, engine)
	rt.holdWait = time.Millisecond
	got := rt.Handle(context.Background(), adapter.Event{Intent: adapter.IntentLogin, Player: "Steve", World: world})
	if got.Err == nil {
		t.Fatal("expected hold timeout")
	}
	if got.State != adapter.StateStarting {
		t.Fatalf("state = %s", got.State)
	}
}

func TestWorldsAndListRunning(t *testing.T) {
	engine := NewMemoryEngine("survival")
	world := survival()
	rt := NewRuntime([]*adapter.World{world, nil}, engine)
	if len(rt.Worlds()) != 2 {
		t.Fatalf("worlds = %d", len(rt.Worlds()))
	}
	names, err := engine.ListRunning(context.Background())
	if err != nil || len(names) != 1 {
		t.Fatalf("list = %v %v", names, err)
	}
	snaps := rt.Snapshots()
	if len(snaps) != 1 || snaps[0].State != adapter.StateOnline {
		t.Fatalf("snaps = %#v", snaps)
	}
}
