package gateway

import (
	"sort"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

// Snapshot is the panel-facing world view from GET /worlds.
type Snapshot struct {
	ID             string     `json:"id"`
	Game           string     `json:"game"`
	State          string     `json:"state"`
	Container      string     `json:"container"`
	Players        *int       `json:"players"`
	IdleTimeout    string     `json:"idle_timeout"`
	StopTimeout    string     `json:"stop_timeout"`
	LastForcedStop *time.Time `json:"last_forced_stop"`
}

// List returns a stable snapshot of every world.
func (s *Supervisor) List() []Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Snapshot, 0, len(s.worlds))
	for _, w := range s.worlds {
		players, unknown := s.activity(w)
		snap := Snapshot{
			ID:             w.spec.ID,
			Game:           w.spec.Game,
			State:          string(w.state),
			Container:      w.spec.Container,
			IdleTimeout:    idleTimeoutOf(w).String(),
			StopTimeout:    stopTimeoutOf(w).String(),
			LastForcedStop: cloneTime(w.lastForcedStop),
		}
		if !unknown {
			n := players
			snap.Players = &n
		}
		out = append(out, snap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func cloneTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}

// State returns the current state, or empty if the world is unknown.
func (s *Supervisor) State(id string) adapter.WorldState {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.worlds[id]
	if !ok {
		return ""
	}
	return w.state
}

// LastForcedStop returns the last forced-stop time for the panel.
func (s *Supervisor) LastForcedStop(id string) *time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.worlds[id]
	if !ok {
		return nil
	}
	return cloneTime(w.lastForcedStop)
}

// WaitState blocks until the world reaches want or the timeout elapses.
func (s *Supervisor) WaitState(id string, want adapter.WorldState, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.State(id) == want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return s.State(id) == want
}
