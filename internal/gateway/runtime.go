package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

const (
	// StartCooldown is the anti-false-wake rate limit from edge-gateway.md.
	StartCooldown = 30 * time.Second
	// DefaultHoldWindow is the short occupy-kick wait before the client is dropped.
	DefaultHoldWindow = 2 * time.Second
	// DefaultHoldTimeout is how long occupy=hold waits for the backend TCP port.
	DefaultHoldTimeout = 25 * time.Second
)

var (
	ErrNoWorld     = errors.New("no matching world")
	ErrNotWake     = errors.New("event is not a wake")
	ErrWhitelist   = errors.New("player is not on wake_whitelist")
	ErrBusy        = errors.New("another world is already running")
	ErrRateLimited = errors.New("world start rate-limited")
	ErrStartFailed = errors.New("docker start failed")
)

// Result is the outcome of a classified Minecraft event.
type Result struct {
	World     *adapter.World
	State     adapter.WorldState
	Occupy    adapter.OccupyAction
	Started   bool
	WakeCause string
	Err       error
}

type worldRuntime struct {
	world         *adapter.World
	state         adapter.WorldState
	lastStart     time.Time
	lastWakeCause string
	lastError     string
}

// Runtime owns per-world state, docker start, and anti-false-wake rules.
// Idle scale-down is issue #62 and is not implemented here.
type Runtime struct {
	mu       sync.Mutex
	worlds   []*adapter.World
	byID     map[string]*worldRuntime
	docker   ContainerEngine
	adapters *adapter.Registry
	now      func() time.Time
	cooldown time.Duration
	holdWait time.Duration
}

// NewRuntime tracks worlds and syncs running containers into online.
func NewRuntime(worlds []*adapter.World, docker ContainerEngine) *Runtime {
	r := &Runtime{
		worlds:   worlds,
		byID:     map[string]*worldRuntime{},
		docker:   docker,
		adapters: adapter.NewRegistry(),
		now:      time.Now,
		cooldown: StartCooldown,
		holdWait: DefaultHoldTimeout,
	}
	for _, w := range worlds {
		if w == nil || w.ID == "" {
			continue
		}
		st := adapter.StateAsleep
		if docker != nil && w.Backend.Container != "" {
			if running, err := docker.Running(context.Background(), w.Backend.Container); err == nil && running {
				st = adapter.StateOnline
			}
		}
		r.byID[w.ID] = &worldRuntime{world: w, state: st}
	}
	return r
}

func (r *Runtime) Worlds() []*adapter.World {
	return r.worlds
}

// Snapshot is the panel-facing view of one world.
type Snapshot struct {
	ID            string             `json:"id"`
	Name          string             `json:"name,omitempty"`
	Game          string             `json:"game"`
	State         adapter.WorldState `json:"state"`
	Allocation    adapter.Allocation `json:"allocation"`
	LastWakeCause string             `json:"last_wake_cause,omitempty"`
	LastError     string             `json:"last_error,omitempty"`
	Players       int                `json:"players"`
}

func (r *Runtime) Snapshots() []Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Snapshot, 0, len(r.worlds))
	for _, w := range r.worlds {
		if w == nil {
			continue
		}
		wr := r.byID[w.ID]
		snap := Snapshot{
			ID:         w.ID,
			Name:       w.Name,
			Game:       w.Game,
			State:      adapter.StateAsleep,
			Allocation: w.Allocation,
		}
		if wr != nil {
			snap.State = wr.state
			snap.LastWakeCause = wr.lastWakeCause
			snap.LastError = wr.lastError
		}
		out = append(out, snap)
	}
	return out
}

func (r *Runtime) WorldByHost(host string) *adapter.World {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	for _, w := range r.worlds {
		if w == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSuffix(w.Allocation.Host, "."), host) {
			return w
		}
	}
	return nil
}

func (r *Runtime) adapterFor(world *adapter.World) adapter.Adapter {
	if world == nil {
		return adapter.MinecraftJava()
	}
	if a, ok := r.adapters.ForGame(world.Game); ok {
		return a
	}
	return adapter.MinecraftJava()
}

func activeState(s adapter.WorldState) bool {
	switch s {
	case adapter.StateStarting, adapter.StateOnline, adapter.StateIdleWait, adapter.StateStopping:
		return true
	default:
		return false
	}
}

func (r *Runtime) otherWorldRunning(id string) bool {
	for otherID, wr := range r.byID {
		if otherID == id {
			continue
		}
		if activeState(wr.state) {
			return true
		}
	}
	return false
}

// Handle applies a handshake already classified by mc-router.
func (r *Runtime) Handle(ctx context.Context, e adapter.Event) Result {
	world := e.World
	if world == nil {
		return Result{Err: ErrNoWorld}
	}
	e.World = world
	ad := r.adapterFor(world)

	if e.Intent == adapter.IntentStatus {
		_ = ad.ServeStatus(nil, world, r.stateOf(world.ID))
		return Result{World: world, State: r.stateOf(world.ID)}
	}

	if !ad.ShouldWake(e) {
		reason := ErrNotWake
		if e.Intent == adapter.IntentLogin && !adapter.WakeAllowed(world, e.Player) {
			reason = ErrWhitelist
			log.Printf("false-wake reject world=%s player=%s reason=wake_whitelist", world.ID, e.Player)
		}
		return Result{World: world, State: r.stateOf(world.ID), Err: reason}
	}

	r.mu.Lock()
	wr := r.byID[world.ID]
	if wr == nil {
		r.mu.Unlock()
		return Result{World: world, Err: ErrNoWorld}
	}
	if activeState(wr.state) && wr.world.ID == world.ID && wr.state != adapter.StateStarting {
		occupy := ad.Occupy(world, wr.state)
		r.mu.Unlock()
		return Result{World: world, State: wr.state, Occupy: occupy}
	}
	if wr.state == adapter.StateStarting {
		occupy := ad.Occupy(world, wr.state)
		r.mu.Unlock()
		return Result{World: world, State: wr.state, Occupy: occupy}
	}
	if r.otherWorldRunning(world.ID) {
		r.mu.Unlock()
		log.Printf("false-wake reject world=%s reason=another_world_running", world.ID)
		return Result{World: world, State: wr.state, Err: ErrBusy}
	}
	now := r.now()
	if !wr.lastStart.IsZero() && now.Sub(wr.lastStart) < r.cooldown {
		r.mu.Unlock()
		log.Printf("false-wake reject world=%s reason=rate_limit", world.ID)
		return Result{World: world, State: wr.state, Err: ErrRateLimited}
	}

	wr.lastStart = now
	wr.state = adapter.StateStarting
	cause := "minecraft_login"
	if e.Player != "" {
		cause = "minecraft_login:" + e.Player
	}
	wr.lastWakeCause = cause
	wr.lastError = ""
	occupy := ad.Occupy(world, wr.state)
	r.mu.Unlock()

	log.Printf("wake world=%s cause=%s occupy=%s", world.ID, cause, occupyName(occupy))

	if r.docker == nil || world.Backend.Container == "" {
		r.setFailed(world.ID, "missing container engine or name")
		return Result{World: world, State: adapter.StateFailed, Occupy: occupy, WakeCause: cause, Err: ErrStartFailed}
	}
	if err := r.docker.Start(ctx, world.Backend.Container); err != nil {
		r.setFailed(world.ID, err.Error())
		log.Printf("scale error world=%s err=%v", world.ID, err)
		return Result{World: world, State: adapter.StateFailed, Occupy: occupy, WakeCause: cause, Err: fmt.Errorf("%w: %v", ErrStartFailed, err)}
	}

	if occupy == adapter.OccupyHold {
		if err := r.waitRunning(ctx, world.Backend.Container, r.holdWait); err != nil {
			log.Printf("hold timeout world=%s err=%v", world.ID, err)
			return Result{World: world, State: adapter.StateStarting, Occupy: occupy, Started: true, WakeCause: cause, Err: err}
		}
		r.setState(world.ID, adapter.StateOnline)
		return Result{World: world, State: adapter.StateOnline, Occupy: occupy, Started: true, WakeCause: cause}
	}

	return Result{World: world, State: adapter.StateStarting, Occupy: occupy, Started: true, WakeCause: cause}
}

func (r *Runtime) waitRunning(ctx context.Context, container string, timeout time.Duration) error {
	deadline := r.now().Add(timeout)
	for {
		running, err := r.docker.Running(ctx, container)
		if err == nil && running {
			return nil
		}
		if r.now().After(deadline) {
			if err != nil {
				return err
			}
			return fmt.Errorf("backend not ready within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (r *Runtime) stateOf(id string) adapter.WorldState {
	r.mu.Lock()
	defer r.mu.Unlock()
	if wr, ok := r.byID[id]; ok {
		return wr.state
	}
	return adapter.StateAsleep
}

func (r *Runtime) setFailed(id, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if wr, ok := r.byID[id]; ok {
		wr.state = adapter.StateFailed
		wr.lastError = message
	}
}

func (r *Runtime) setState(id string, state adapter.WorldState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if wr, ok := r.byID[id]; ok {
		wr.state = state
	}
}

func occupyName(a adapter.OccupyAction) string {
	switch a {
	case adapter.OccupyHold:
		return "hold"
	case adapter.OccupyRetry:
		return "retry"
	default:
		return "kick"
	}
}
