package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

func newTestSupervisor(t *testing.T) (*Supervisor, *FakeRuntime, *adapter.PlayCounts) {
	t.Helper()
	plays := adapter.NewPlayCounts()
	reg := adapter.NewRegistryWithPlayCounts(plays)
	rt := NewFakeRuntime()
	sup := NewSupervisor(reg, rt, plays)
	sup.AddWorld(adapter.World{
		ID:          "survival",
		Game:        adapter.GameMinecraftJava,
		Container:   "mc-survival",
		IdleTimeout: 20 * time.Millisecond,
		StopTimeout: 20 * time.Millisecond,
	})
	return sup, rt, plays
}

func TestIdleWaitThenGracefulStop(t *testing.T) {
	sup, rt, _ := newTestSupervisor(t)
	rt.SetRunning("mc-survival", true)
	sup.MarkOnline("survival")

	sup.Tick()
	if got := sup.State("survival"); got != adapter.StateIdleWait {
		t.Fatalf("state = %s, want idle_wait", got)
	}
	if snaps := sup.List(); len(snaps) != 1 || snaps[0].LastForcedStop != nil {
		t.Fatalf("snapshot = %#v", snaps)
	}

	time.Sleep(25 * time.Millisecond)
	sup.Tick()
	if !sup.WaitState("survival", adapter.StateAsleep, time.Second) {
		t.Fatalf("state = %s, want asleep", sup.State("survival"))
	}
	sigs := rt.Signals()
	if len(sigs) != 1 || sigs[0].Name != "SIGTERM" || sigs[0].Container != "mc-survival" {
		t.Fatalf("signals = %#v", sigs)
	}
	if sup.LastForcedStop("survival") != nil {
		t.Fatal("graceful stop must not set last_forced_stop")
	}
	if rt.HostStops() != 0 {
		t.Fatal("must not stop the EC2 host")
	}
}

func TestPlayerCancelsIdleWait(t *testing.T) {
	sup, rt, plays := newTestSupervisor(t)
	rt.SetRunning("mc-survival", true)
	sup.MarkOnline("survival")
	sup.Tick()
	if sup.State("survival") != adapter.StateIdleWait {
		t.Fatal(sup.State("survival"))
	}

	sup.SetPlayConnections("survival", 1)
	if n, _ := plays.Get("survival"); n != 1 {
		t.Fatalf("play count = %d", n)
	}
	sup.Tick()
	if got := sup.State("survival"); got != adapter.StateOnline {
		t.Fatalf("state = %s, want online", got)
	}

	time.Sleep(25 * time.Millisecond)
	sup.Tick()
	if got := sup.State("survival"); got != adapter.StateOnline {
		t.Fatalf("still online after timeout with players, got %s", got)
	}
	if len(rt.Signals()) != 0 {
		t.Fatalf("stopped while players present: %#v", rt.Signals())
	}
}

func TestUnknownActivityDoesNotIdleStop(t *testing.T) {
	plays := adapter.NewPlayCounts()
	rt := NewFakeRuntime()
	sup := NewSupervisor(adapter.NewRegistryWithPlayCounts(plays), rt, plays)
	sup.AddWorld(adapter.World{
		ID:          "vale",
		Game:        adapter.GameValheim,
		IdleTimeout: 10 * time.Millisecond,
	})
	rt.SetRunning("vale", true)
	sup.MarkOnline("vale")
	sup.Tick()
	if got := sup.State("vale"); got != adapter.StateOnline {
		t.Fatalf("unknown activity must stay online, got %s", got)
	}
	time.Sleep(20 * time.Millisecond)
	sup.Tick()
	if got := sup.State("vale"); got != adapter.StateOnline {
		t.Fatalf("unknown activity must not stop, got %s", got)
	}
	if len(rt.Signals()) != 0 {
		t.Fatalf("signals = %#v", rt.Signals())
	}
}

func TestForcedStopAfterStopTimeout(t *testing.T) {
	sup, rt, _ := newTestSupervisor(t)
	rt.IgnoreTERM("mc-survival")
	sup.MarkOnline("survival")
	sup.Tick()
	time.Sleep(25 * time.Millisecond)
	sup.Tick()
	if !sup.WaitState("survival", adapter.StateAsleep, time.Second) {
		t.Fatalf("state = %s, want asleep", sup.State("survival"))
	}
	var term, kill bool
	for _, sig := range rt.Signals() {
		if sig.Name == "SIGTERM" {
			term = true
		}
		if sig.Name == "SIGKILL" {
			kill = true
		}
	}
	if !term || !kill {
		t.Fatalf("signals = %#v", rt.Signals())
	}
	if sup.LastForcedStop("survival") == nil {
		t.Fatal("panel must show a forced stop")
	}
	snaps := sup.List()
	if len(snaps) != 1 || snaps[0].LastForcedStop == nil || snaps[0].State != string(adapter.StateAsleep) {
		t.Fatalf("snapshot = %#v", snaps)
	}
}

func TestDockerStopErrorFailsWorld(t *testing.T) {
	sup, rt, _ := newTestSupervisor(t)
	rt.stopErr = errors.New("docker down")
	rt.SetRunning("mc-survival", true)
	sup.MarkOnline("survival")
	sup.Tick()
	time.Sleep(25 * time.Millisecond)
	sup.Tick()
	if !sup.WaitState("survival", adapter.StateFailed, time.Second) {
		t.Fatalf("state = %s, want failed", sup.State("survival"))
	}
}

func TestReconcileRunningAndStopped(t *testing.T) {
	sup, rt, _ := newTestSupervisor(t)
	rt.SetRunning("mc-survival", true)
	sup.Reconcile(context.Background())
	if got := sup.State("survival"); got != adapter.StateIdleWait {
		t.Fatalf("running + zero players = %s, want idle_wait", got)
	}

	rt.SetRunning("mc-survival", false)
	sup.Reconcile(context.Background())
	if got := sup.State("survival"); got != adapter.StateAsleep {
		t.Fatalf("stopped container = %s, want asleep", got)
	}
}

func TestObserveRunningUnknownStaysIdleWaitWithoutTimer(t *testing.T) {
	plays := adapter.NewPlayCounts()
	rt := NewFakeRuntime()
	sup := NewSupervisor(adapter.NewRegistryWithPlayCounts(plays), rt, plays)
	sup.AddWorld(adapter.World{ID: "vale", Game: adapter.GameValheim, IdleTimeout: 10 * time.Millisecond})
	sup.ObserveRunning("vale")
	if got := sup.State("vale"); got != adapter.StateIdleWait {
		t.Fatalf("state = %s", got)
	}
	time.Sleep(20 * time.Millisecond)
	sup.Tick()
	if len(rt.Signals()) != 0 {
		t.Fatal("unknown activity must not fire idle stop")
	}
}

func TestDefaultsOnAddWorld(t *testing.T) {
	sup := NewSupervisor(nil, NewFakeRuntime(), nil)
	sup.AddWorld(adapter.World{ID: "survival", Game: adapter.GameMinecraftJava})
	snaps := sup.List()
	if len(snaps) != 1 {
		t.Fatal(snaps)
	}
	if snaps[0].Container != "survival" {
		t.Fatalf("container default = %s", snaps[0].Container)
	}
	if snaps[0].IdleTimeout != DefaultIdleTimeout.String() {
		t.Fatalf("idle default = %s", snaps[0].IdleTimeout)
	}
	if snaps[0].Players == nil || *snaps[0].Players != 0 {
		t.Fatalf("players = %#v", snaps[0].Players)
	}
}

func TestMarkOnlineUnknownWorld(t *testing.T) {
	sup := NewSupervisor(nil, NewFakeRuntime(), adapter.NewPlayCounts())
	sup.MarkOnline("missing")
	if got := sup.State("missing"); got != "" {
		t.Fatalf("missing state = %s", got)
	}
	if sup.LastForcedStop("missing") != nil {
		t.Fatal("missing last forced stop")
	}
}

func TestObserveRunningWithPlayers(t *testing.T) {
	sup, _, plays := newTestSupervisor(t)
	plays.Set("survival", 2)
	sup.ObserveRunning("survival")
	if got := sup.State("survival"); got != adapter.StateOnline {
		t.Fatalf("state = %s", got)
	}
}

func TestListPlayersUnknown(t *testing.T) {
	sup := NewSupervisor(nil, NewFakeRuntime(), nil)
	sup.AddWorld(adapter.World{ID: "vale", Game: adapter.GameValheim})
	snaps := sup.List()
	if len(snaps) != 1 || snaps[0].Players != nil {
		t.Fatalf("unknown players must be null: %#v", snaps)
	}
}

func TestFakeRuntimeRunningError(t *testing.T) {
	rt := NewFakeRuntime()
	rt.runningErr = errors.New("inspect failed")
	if _, err := rt.Running(context.Background(), "x"); err == nil {
		t.Fatal("want running error")
	}
	rt.StopHost()
	if rt.HostStops() != 1 {
		t.Fatal("StopHost should record an instance stop")
	}
}

func TestWaitStateTimeout(t *testing.T) {
	sup, _, _ := newTestSupervisor(t)
	if sup.WaitState("survival", adapter.StateOnline, 15*time.Millisecond) {
		t.Fatal("want timeout")
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	sup, _, _ := newTestSupervisor(t)
	sup.tickEvery = 5 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		sup.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit")
	}
}
