package panel

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

// Allocation is the player-facing host and port.
type Allocation struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// String returns host:port.
func (a Allocation) String() string {
	if a.Host == "" && a.Port == 0 {
		return ""
	}
	return a.Host + ":" + strconv.Itoa(a.Port)
}

// World is a panel record. Persistence is issue #60; this issue only reads.
type World struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Game       string     `json:"game"`
	Allocation Allocation `json:"allocation"`
}

// Catalog lists world records without creating them.
type Catalog interface {
	List() []World
	Get(id string) (World, bool)
}

// MemoryCatalog is the stand-in until SQLite lands in #60.
type MemoryCatalog struct {
	mu     sync.RWMutex
	worlds []World
}

// NewMemoryCatalog stores the given worlds.
func NewMemoryCatalog(worlds ...World) *MemoryCatalog {
	copied := append([]World(nil), worlds...)
	return &MemoryCatalog{worlds: copied}
}

// Reset replaces the catalog.
func (c *MemoryCatalog) Reset(worlds ...World) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.worlds = append([]World(nil), worlds...)
}

// List returns a copy of the records.
func (c *MemoryCatalog) List() []World {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]World, len(c.worlds))
	copy(out, c.worlds)
	return out
}

// Get returns one record.
func (c *MemoryCatalog) Get(id string) (World, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, w := range c.worlds {
		if w.ID == id {
			return w, true
		}
	}
	return World{}, false
}

// View is a world plus gateway operations the panel must not hide.
type View struct {
	World
	AllocationString   string     `json:"allocation_string"`
	State              string     `json:"state"`
	Players            *int       `json:"players"`
	IdleTimer          string     `json:"idle_timer,omitempty"`
	LastWakeCause      string     `json:"last_wake_cause,omitempty"`
	LastWakeAt         *time.Time `json:"last_wake_at,omitempty"`
	LastForcedStopAt   *time.Time `json:"last_forced_stop_at,omitempty"`
	HoursOnlineLastDay float64    `json:"hours_online_last_day"`
	LastError          string     `json:"last_error,omitempty"`
	ContainerRunning   bool       `json:"container_running"`
	AsleepHint         bool       `json:"-"`
	ConfirmStop        bool       `json:"-"`
}

func mergeView(world World, snap Snapshot, now time.Time) View {
	state := snap.State
	if state == "" {
		state = string(adapter.StateAsleep)
	}
	players, known := snap.KnownPlayers()
	v := View{
		World:              world,
		AllocationString:   world.Allocation.String(),
		State:              state,
		Players:            players,
		LastWakeCause:      snap.LastWakeCause,
		LastWakeAt:         snap.LastWakeAt,
		LastForcedStopAt:   snap.LastForcedStopAt,
		HoursOnlineLastDay: snap.HoursOnlineLastDay,
		LastError:          snap.LastError,
		ContainerRunning:   snap.ContainerRunning,
		AsleepHint:         state == string(adapter.StateAsleep),
		ConfirmStop:        known && players != nil && *players > 0,
	}
	if state == string(adapter.StateIdleWait) && snap.IdleUntil != nil {
		remain := snap.IdleUntil.Sub(now)
		if remain < 0 {
			remain = 0
		}
		v.IdleTimer = remain.Round(time.Second).String()
	}
	return v
}

func (s *Server) views(ctx context.Context) ([]View, error) {
	snaps, err := s.gateway.States(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	worlds := s.catalog.List()
	out := make([]View, 0, len(worlds))
	for _, w := range worlds {
		out = append(out, mergeView(w, snaps[w.ID], now))
	}
	return out, nil
}

func (s *Server) view(ctx context.Context, id string) (View, bool, error) {
	world, ok := s.catalog.Get(id)
	if !ok {
		return View{}, false, nil
	}
	snaps, err := s.gateway.States(ctx)
	if err != nil {
		return View{}, true, err
	}
	return mergeView(world, snaps[id], s.now()), true, nil
}
