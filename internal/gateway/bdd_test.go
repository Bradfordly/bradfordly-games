package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
	"github.com/cucumber/godog"
)

type idleBDD struct {
	sup   *Supervisor
	rt    *FakeRuntime
	plays *adapter.PlayCounts
	id    string
	idle  time.Duration
	stop  time.Duration
}

func (w *idleBDD) reset() {
	w.plays = adapter.NewPlayCounts()
	w.rt = NewFakeRuntime()
	w.sup = NewSupervisor(adapter.NewRegistryWithPlayCounts(w.plays), w.rt, w.plays)
	w.id = ""
	w.idle = 20 * time.Millisecond
	w.stop = 20 * time.Millisecond
}

func (w *idleBDD) addMinecraft(id, container string) error {
	w.id = id
	w.sup.AddWorld(adapter.World{
		ID:          id,
		Game:        adapter.GameMinecraftJava,
		Container:   container,
		IdleTimeout: w.idle,
		StopTimeout: w.stop,
	})
	return nil
}

func (w *idleBDD) addValheim(id, container string) error {
	w.id = id
	w.sup.AddWorld(adapter.World{
		ID:          id,
		Game:        adapter.GameValheim,
		Container:   container,
		IdleTimeout: w.idle,
		StopTimeout: w.stop,
	})
	return nil
}

func (w *idleBDD) setIdle(d string) error {
	parsed, err := time.ParseDuration(d)
	if err != nil {
		return err
	}
	w.idle = parsed
	w.patchCurrentTimeouts()
	return nil
}

func (w *idleBDD) setStop(d string) error {
	parsed, err := time.ParseDuration(d)
	if err != nil {
		return err
	}
	w.stop = parsed
	w.patchCurrentTimeouts()
	return nil
}

func (w *idleBDD) patchCurrentTimeouts() {
	if w.id == "" {
		return
	}
	w.sup.mu.Lock()
	defer w.sup.mu.Unlock()
	if world, ok := w.sup.worlds[w.id]; ok {
		world.spec.IdleTimeout = w.idle
		world.spec.StopTimeout = w.stop
	}
}

func (w *idleBDD) worldOnline() error {
	container := w.id
	if snap := w.snapshot(); snap.Container != "" {
		container = snap.Container
	}
	w.rt.SetRunning(container, true)
	w.sup.MarkOnline(w.id)
	return nil
}

func (w *idleBDD) reportPlayers(n int) error {
	w.sup.SetPlayConnections(w.id, n)
	return nil
}

func (w *idleBDD) ignoreTERM() error {
	snap := w.snapshot()
	w.rt.IgnoreTERM(snap.Container)
	return nil
}

func (w *idleBDD) tick() error {
	w.sup.Tick()
	return nil
}

func (w *idleBDD) idleElapses() error {
	time.Sleep(w.idle + 5*time.Millisecond)
	return nil
}

func (w *idleBDD) stateIs(want string) error {
	got := string(w.sup.State(w.id))
	if got != want {
		return fmt.Errorf("state = %s, want %s", got, want)
	}
	return nil
}

func (w *idleBDD) becomes(want string) error {
	if !w.sup.WaitState(w.id, adapter.WorldState(want), time.Second) {
		return fmt.Errorf("state = %s, want %s", w.sup.State(w.id), want)
	}
	return nil
}

func (w *idleBDD) received(signal, container string) error {
	for _, sig := range w.rt.Signals() {
		if sig.Name == signal && sig.Container == container {
			return nil
		}
	}
	return fmt.Errorf("missing %s for %s in %#v", signal, container, w.rt.Signals())
}

func (w *idleBDD) noForced() error {
	if w.sup.LastForcedStop(w.id) != nil {
		return fmt.Errorf("unexpected forced stop")
	}
	return nil
}

func (w *idleBDD) forced() error {
	if w.sup.LastForcedStop(w.id) == nil {
		return fmt.Errorf("panel missing last_forced_stop")
	}
	return nil
}

func (w *idleBDD) hostNotStopped() error {
	if w.rt.HostStops() != 0 {
		return fmt.Errorf("EC2 host was stopped")
	}
	return nil
}

func (w *idleBDD) notStopped() error {
	if len(w.rt.Signals()) != 0 {
		return fmt.Errorf("container stopped: %#v", w.rt.Signals())
	}
	return nil
}

func (w *idleBDD) snapshot() Snapshot {
	for _, snap := range w.sup.List() {
		if snap.ID == w.id {
			return snap
		}
	}
	return Snapshot{}
}

func TestIdleStopFeatures(t *testing.T) {
	suite := godog.TestSuite{
		Name: "idle_stop",
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			w := &idleBDD{}
			ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
				w.reset()
				return ctx, nil
			})
			ctx.Step(`^a Minecraft world "([^"]*)" in container "([^"]*)"$`, w.addMinecraft)
			ctx.Step(`^a Valheim world "([^"]*)" in container "([^"]*)"$`, w.addValheim)
			ctx.Step(`^idle_timeout is (\S+)$`, w.setIdle)
			ctx.Step(`^stop_timeout is (\S+)$`, w.setStop)
			ctx.Step(`^the world is online$`, w.worldOnline)
			ctx.Step(`^the adapter reports (\d+) play connections?$`, w.reportPlayers)
			ctx.Step(`^the container ignores SIGTERM$`, w.ignoreTERM)
			ctx.Step(`^the gateway ticks once$`, w.tick)
			ctx.Step(`^the idle timer elapses$`, w.idleElapses)
			ctx.Step(`^the world state is "([^"]*)"$`, w.stateIs)
			ctx.Step(`^the world becomes "([^"]*)"$`, w.becomes)
			ctx.Step(`^the runtime received (SIGTERM|SIGKILL) for "([^"]*)"$`, w.received)
			ctx.Step(`^the panel does not show a forced stop$`, w.noForced)
			ctx.Step(`^the panel shows a forced stop$`, w.forced)
			ctx.Step(`^the host instance was not stopped$`, w.hostNotStopped)
			ctx.Step(`^the container was not stopped$`, w.notStopped)
		},
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"../../features"},
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("BDD scenarios failed")
	}
}
