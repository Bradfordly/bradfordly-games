package world

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

// Handler serves the control-plane world API.
type Handler struct {
	Service *Service
}

func (h Handler) Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /api/worlds", h.list)
	mux.HandleFunc("POST /api/worlds", h.create)
	mux.HandleFunc("GET /api/worlds/{id}", h.get)
	mux.HandleFunc("PATCH /api/worlds/{id}", h.update)
	mux.HandleFunc("DELETE /api/worlds/{id}", h.delete)
	return mux
}

func (h Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (h Handler) list(w http.ResponseWriter, _ *http.Request) {
	worlds, err := h.Service.List()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, worlds)
}

func (h Handler) get(w http.ResponseWriter, r *http.Request) {
	world, err := h.Service.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, world)
}

func (h Handler) create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	world, err := h.Service.Create(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, world)
}

func (h Handler) update(w http.ResponseWriter, r *http.Request) {
	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	world, err := h.Service.Update(r.Context(), r.PathValue("id"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, world)
}

func (h Handler) delete(w http.ResponseWriter, r *http.Request) {
	destroy := false
	if raw := r.URL.Query().Get("destroy"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			http.Error(w, `{"error":"destroy must be a boolean"}`, http.StatusBadRequest)
			return
		}
		destroy = parsed
	}
	if err := h.Service.Delete(r.Context(), r.PathValue("id"), destroy); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrConflict):
		status = http.StatusConflict
	case errors.Is(err, ErrInvalidGame), errors.Is(err, ErrNameRequired), errors.Is(err, ErrIdleTimeout), errors.Is(err, ErrInvalid):
		status = http.StatusBadRequest
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
