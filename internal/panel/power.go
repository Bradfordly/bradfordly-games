package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/bradfordly/bradfordly-games/internal/world"
)

// Gateway forwards power actions. The panel never patches replica counts.
type Gateway interface {
	Power(ctx context.Context, worldID, action string) error
}

// MemoryGateway records start/stop calls for tests and local use.
type MemoryGateway struct {
	mu    sync.Mutex
	Calls []PowerCall
}

// PowerCall is one forwarded start or stop.
type PowerCall struct {
	WorldID string
	Action  string
}

// Power records the action. It does not touch Kubernetes replicas.
func (g *MemoryGateway) Power(_ context.Context, worldID, action string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Calls = append(g.Calls, PowerCall{WorldID: worldID, Action: action})
	return nil
}

func (s *Server) handlePower(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.cfg.Store.Get(r.Context(), id); errors.Is(err, world.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var body struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Action != "start" && body.Action != "stop" {
		http.Error(w, "action must be start or stop", http.StatusBadRequest)
		return
	}
	if s.cfg.Gateway == nil {
		http.Error(w, "gateway unavailable", http.StatusBadGateway)
		return
	}
	if err := s.cfg.Gateway.Power(r.Context(), id, body.Action); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id, "action": body.Action})
}
