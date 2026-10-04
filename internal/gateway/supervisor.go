package gateway

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

// Supervisor is the per-world idle state machine.
type Supervisor struct {
	mu        sync.Mutex
	worlds    map[string]*managedWorld
	adapters  *adapter.Registry
	runtime   Runtime
	plays     *adapter.PlayCounts
	tickEvery time.Duration
	now       func() time.Time
}

type managedWorld struct {
	spec           adapter.World
	state          adapter.WorldState
	idleSince      time.Time
	lastForcedStop *time.Time
	power          sync.Mutex
}

// NewSupervisor watches worlds using adapters for Activity and runtime for stop.
func NewSupervisor(adapters *adapter.Registry, runtime Runtime, plays *adapter.PlayCounts) *Supervisor {
	if adapters == nil {
		adapters = adapter.NewRegistryWithPlayCounts(plays)
	}
	return &Supervisor{
		worlds:    map[string]*managedWorld{},
		adapters:  adapters,
		runtime:   runtime,
		plays:     plays,
		tickEvery: time.Second,
		now:       time.Now,
	}
}

// SetPlayConnections is the smallest hook for proxied play sessions (#61).
func (s *Supervisor) SetPlayConnections(id string, n int) {
	if s.plays != nil {
		s.plays.Set(id, n)
	}
}

// AddWorld registers a world. Container defaults to the world id.
func (s *Supervisor) AddWorld(w adapter.World) {
	if w.Container == "" {
		w.Container = w.ID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.worlds[w.ID] = &managedWorld{
		spec:  w,
		state: adapter.StateAsleep,
	}
}

// MarkOnline is the smallest wake hook. #61 calls this when the backend is ready.
func (s *Supervisor) MarkOnline(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.worlds[id]
	if !ok {
		return
	}
	w.state = adapter.StateOnline
	w.idleSince = time.Time{}
}

// ObserveRunning rebuilds state after a gateway restart when the container is up.
// Unknown activity enters idle_wait without starting the timer.
func (s *Supervisor) ObserveRunning(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.worlds[id]
	if !ok {
		return
	}
	players, unknown := s.activity(w)
	if unknown {
		w.state = adapter.StateIdleWait
		w.idleSince = time.Time{}
		return
	}
	if players > 0 {
		w.state = adapter.StateOnline
		w.idleSince = time.Time{}
		return
	}
	w.state = adapter.StateIdleWait
	w.idleSince = s.now()
}

// Reconcile sets asleep/idle_wait from the Docker running bit.
func (s *Supervisor) Reconcile(ctx context.Context) {
	s.mu.Lock()
	ids := make([]string, 0, len(s.worlds))
	for id := range s.worlds {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	for _, id := range ids {
		s.mu.Lock()
		w, ok := s.worlds[id]
		if !ok {
			s.mu.Unlock()
			continue
		}
		container := w.spec.Container
		s.mu.Unlock()
		running, err := s.runtime.Running(ctx, container)
		if err != nil {
			continue
		}
		if running {
			s.ObserveRunning(id)
			continue
		}
		s.mu.Lock()
		if w, ok := s.worlds[id]; ok {
			w.state = adapter.StateAsleep
			w.idleSince = time.Time{}
		}
		s.mu.Unlock()
	}
}

// Tick advances idle_wait and starts scale-down when the timer fires.
func (s *Supervisor) Tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for id, w := range s.worlds {
		switch w.state {
		case adapter.StateOnline, adapter.StateIdleWait:
			s.advanceIdle(id, w, now)
		}
	}
}

func (s *Supervisor) advanceIdle(id string, w *managedWorld, now time.Time) {
	players, unknown := s.activity(w)
	if unknown {
		w.idleSince = time.Time{}
		return
	}
	if players > 0 {
		w.state = adapter.StateOnline
		w.idleSince = time.Time{}
		return
	}
	if w.state == adapter.StateOnline {
		w.state = adapter.StateIdleWait
		w.idleSince = now
		return
	}
	if w.idleSince.IsZero() {
		w.idleSince = now
		return
	}
	if now.Sub(w.idleSince) < idleTimeoutOf(w) {
		return
	}
	w.state = adapter.StateStopping
	go s.scaleDown(id)
}

func (s *Supervisor) scaleDown(id string) {
	s.mu.Lock()
	w, ok := s.worlds[id]
	if !ok {
		s.mu.Unlock()
		return
	}
	container := w.spec.Container
	stopFor := stopTimeoutOf(w)
	ad, _ := s.adapters.ForGame(w.spec.Game)
	worldCopy := w.spec
	s.mu.Unlock()

	w.power.Lock()
	defer w.power.Unlock()

	s.mu.Lock()
	if w.state != adapter.StateStopping {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	if ad != nil {
		if err := ad.GracefulStop(&worldCopy); err != nil && err != adapter.ErrNotImplemented {
			log.Printf("gateway: graceful stop %s: %v", id, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), stopFor+time.Second)
	defer cancel()
	forced, err := s.runtime.Stop(ctx, container, stopFor)
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		log.Printf("gateway: docker stop %s: %v", id, err)
		w.state = adapter.StateFailed
		return
	}
	if forced {
		t := now
		w.lastForcedStop = &t
		log.Printf("gateway: forced stop %s", id)
	}
	w.state = adapter.StateAsleep
	w.idleSince = time.Time{}
}

func (s *Supervisor) activity(w *managedWorld) (int, bool) {
	a, ok := s.adapters.ForGame(w.spec.Game)
	if !ok {
		return 0, true
	}
	world := w.spec
	return a.Activity(&world)
}

func idleTimeoutOf(w *managedWorld) time.Duration {
	if w.spec.IdleTimeout > 0 && w.spec.IdleTimeout < MinIdleTimeout {
		return w.spec.IdleTimeout
	}
	return IdleTimeout(w.spec.IdleTimeout)
}

func stopTimeoutOf(w *managedWorld) time.Duration {
	return StopTimeout(w.spec.StopTimeout)
}

// Run ticks until ctx is done.
func (s *Supervisor) Run(ctx context.Context) {
	s.Reconcile(ctx)
	t := time.NewTicker(s.tickEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick()
		}
	}
}
