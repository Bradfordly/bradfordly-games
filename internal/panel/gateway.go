package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Snapshot is gateway-owned runtime state for one world.
type Snapshot struct {
	ID                 string     `json:"id"`
	State              string     `json:"state"`
	Players            *int       `json:"players"`
	PlayersKnown       *bool      `json:"players_known"`
	IdleUntil          *time.Time `json:"idle_until"`
	LastWakeCause      string     `json:"last_wake_cause"`
	LastWakeAt         *time.Time `json:"last_wake_at"`
	LastForcedStopAt   *time.Time `json:"last_forced_stop_at"`
	HoursOnlineLastDay float64    `json:"hours_online_last_day"`
	LastError          string     `json:"last_error"`
	ContainerRunning   bool       `json:"container_running"`
}

// KnownPlayers returns the count when the gateway knows it.
func (s Snapshot) KnownPlayers() (*int, bool) {
	if s.PlayersKnown != nil && !*s.PlayersKnown {
		return nil, false
	}
	if s.Players == nil {
		return nil, false
	}
	return s.Players, true
}

// Gateway is the panel's view of the edge process.
type Gateway interface {
	States(ctx context.Context) (map[string]Snapshot, error)
	Power(ctx context.Context, id, action string) error
}

// NoopGateway stands in until issues #61 and #62 ship a real admin API.
type NoopGateway struct{}

// States returns no runtime data.
func (NoopGateway) States(context.Context) (map[string]Snapshot, error) {
	return map[string]Snapshot{}, nil
}

// Power succeeds without contacting a process.
func (NoopGateway) Power(context.Context, string, string) error {
	return nil
}

// MemoryGateway is an in-process gateway for tests and local BDD.
type MemoryGateway struct {
	mu     sync.Mutex
	states map[string]Snapshot
	powers []PowerCall
}

// PowerCall is a recorded start or stop.
type PowerCall struct {
	ID     string
	Action string
}

// NewMemoryGateway returns an empty in-memory gateway.
func NewMemoryGateway() *MemoryGateway {
	return &MemoryGateway{states: map[string]Snapshot{}}
}

// Set replaces the snapshot for a world.
func (g *MemoryGateway) Set(s Snapshot) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.states == nil {
		g.states = map[string]Snapshot{}
	}
	g.states[s.ID] = s
}

// Calls returns recorded power actions.
func (g *MemoryGateway) Calls() []PowerCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]PowerCall, len(g.powers))
	copy(out, g.powers)
	return out
}

// States copies the in-memory snapshots.
func (g *MemoryGateway) States(context.Context) (map[string]Snapshot, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make(map[string]Snapshot, len(g.states))
	for k, v := range g.states {
		out[k] = v
	}
	return out, nil
}

// Power records the action.
func (g *MemoryGateway) Power(_ context.Context, id, action string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.powers = append(g.powers, PowerCall{ID: id, Action: action})
	return nil
}

// HTTPGateway talks to the gateway admin port.
type HTTPGateway struct {
	base   string
	client *http.Client
}

// NewHTTPGateway sends state and power calls to base (no trailing slash).
func NewHTTPGateway(base string, client *http.Client) *HTTPGateway {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPGateway{base: strings.TrimRight(base, "/"), client: client}
}

// States loads GET /worlds.
func (g *HTTPGateway) States(ctx context.Context) (map[string]Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.base+"/worlds", nil)
	if err != nil {
		return nil, err
	}
	res, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("gateway worlds: status %d", res.StatusCode)
	}
	var list []Snapshot
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&list); err != nil {
		return nil, err
	}
	out := make(map[string]Snapshot, len(list))
	for _, item := range list {
		out[item.ID] = item
	}
	return out, nil
}

// Power posts to /worlds/:id/power. 404 and 405 are no-ops so today's
// admin mux (healthz and GET /worlds only) does not fail the panel.
func (g *HTTPGateway) Power(ctx context.Context, id, action string) error {
	body, err := json.Marshal(map[string]string{"action": action})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.base+"/worlds/"+url.PathEscape(id)+"/power", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	if res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusMethodNotAllowed {
		return nil
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("gateway power: status %d", res.StatusCode)
	}
	return nil
}
