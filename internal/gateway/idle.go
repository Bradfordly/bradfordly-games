package gateway

import (
	"log"
	"net"
	"sync"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

const (
	defaultIdleTimeout = 15 * time.Minute
	defaultStopTimeout = 2 * time.Minute
)

type activity struct {
	mu     sync.Mutex
	n      map[string]int
	timers map[string]*time.Timer
}

func newActivity() *activity {
	return &activity{n: map[string]int{}, timers: map[string]*time.Timer{}}
}

func (a *activity) count(id string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.n[id]
}

func (s *Server) noteJoin(worldID string) {
	s.activity.mu.Lock()
	if t, ok := s.activity.timers[worldID]; ok {
		t.Stop()
		delete(s.activity.timers, worldID)
	}
	s.activity.n[worldID]++
	s.activity.mu.Unlock()
	if w := s.Catalog.Get(worldID); w != nil && w.State == adapter.StateIdleWait {
		s.Catalog.SetState(worldID, adapter.StateOnline)
	}
}

func (s *Server) noteLeave(worldID string) {
	s.activity.mu.Lock()
	s.activity.n[worldID]--
	if s.activity.n[worldID] < 0 {
		s.activity.n[worldID] = 0
	}
	idle := s.activity.n[worldID] == 0
	s.activity.mu.Unlock()
	if !idle {
		return
	}
	w := s.Catalog.Get(worldID)
	if w == nil || (w.State != adapter.StateOnline && w.State != adapter.StateIdleWait) {
		return
	}
	s.Catalog.SetState(worldID, adapter.StateIdleWait)
	timeout := w.IdleTimeout
	if timeout <= 0 {
		timeout = defaultIdleTimeout
	}
	s.activity.mu.Lock()
	if t, ok := s.activity.timers[worldID]; ok {
		t.Stop()
	}
	s.activity.timers[worldID] = time.AfterFunc(timeout, func() { s.idleStop(worldID) })
	s.activity.mu.Unlock()
}

func (s *Server) idleStop(worldID string) {
	if s.activity.count(worldID) > 0 {
		return
	}
	lock := s.wakes.lock(worldID)
	lock.Lock()
	defer lock.Unlock()
	if s.activity.count(worldID) > 0 {
		return
	}
	w := s.Catalog.Get(worldID)
	if w == nil || w.State != adapter.StateIdleWait {
		return
	}
	s.Catalog.SetState(worldID, adapter.StateStopping)
	if mc, ok := s.Adapters.ForGame(w.Game); ok {
		_ = mc.GracefulStop(w)
	}
	stopFor := w.StopTimeout
	if stopFor <= 0 {
		stopFor = defaultStopTimeout
	}
	waitGone(w.Backend, stopFor)
	if err := s.Scaler.SetReplicas(worldID, 0); err != nil {
		log.Printf("scale_down failed world=%s: %v", worldID, err)
		s.Catalog.SetState(worldID, adapter.StateFailed)
		return
	}
	s.Catalog.SetReplicas(worldID, 0)
	s.Catalog.SetState(worldID, adapter.StateAsleep)
}

func waitGone(addr string, timeout time.Duration) {
	if addr == "" {
		return
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 30*time.Millisecond)
		if err != nil {
			return
		}
		_ = c.Close()
		time.Sleep(20 * time.Millisecond)
	}
}
