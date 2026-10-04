package gateway

import (
	"encoding/json"
	"net/http"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

// ScaleRequest is the itzg/mc-router webhook scaler body.
type ScaleRequest struct {
	Action        string `json:"action"`
	ServerAddress string `json:"serverAddress"`
	Backend       string `json:"backend"`
}

type ScaleResponse struct {
	Backend string `json:"backend,omitempty"`
	Occupy  string `json:"occupy,omitempty"`
	Message string `json:"message,omitempty"`
}

func handleScale(w http.ResponseWriter, r *http.Request, rt *Runtime) {
	var req ScaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid scale request", http.StatusBadRequest)
		return
	}
	if req.Action == "down" {
		// Idle stop is issue #62.
		w.WriteHeader(http.StatusOK)
		return
	}
	if req.Action != "up" || rt == nil {
		http.Error(w, "unsupported scale action", http.StatusBadRequest)
		return
	}
	world := rt.WorldByHost(req.ServerAddress)
	if world == nil {
		http.Error(w, "unknown world", http.StatusNotFound)
		return
	}
	result := rt.Handle(r.Context(), adapter.Event{
		Intent:           adapter.IntentLogin,
		World:            world,
		WhitelistChecked: true, // mc-router already applied wake_whitelist
	})
	writeScaleResult(w, result)
}

func writeScaleResult(w http.ResponseWriter, result Result) {
	w.Header().Set("Content-Type", "application/json")
	resp := ScaleResponse{Occupy: occupyName(result.Occupy)}
	if result.World != nil {
		resp.Backend = result.World.Backend.Address
		resp.Message = adapter.StartingMessage(result.World)
	}
	status := http.StatusOK
	switch {
	case result.Err == ErrWhitelist:
		status = http.StatusForbidden
	case result.Err == ErrBusy:
		status = http.StatusConflict
	case result.Err == ErrRateLimited:
		status = http.StatusTooManyRequests
	case result.Err == ErrNoWorld:
		status = http.StatusNotFound
	case result.Err != nil && !result.Started:
		status = http.StatusServiceUnavailable
	case result.Occupy == adapter.OccupyKick:
		// First login drops after the short hold window in mc-router.
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}
