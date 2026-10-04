package panel

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/bradfordly/bradfordly-games/internal/world"
)

func (s *Server) handleListWorlds(w http.ResponseWriter, r *http.Request) {
	items, err := s.cfg.Store.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]world.View, 0, len(items))
	for _, item := range items {
		out = append(out, s.view(r.Context(), item))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateWorld(w http.ResponseWriter, r *http.Request) {
	var rec world.Record
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	world.ApplyDefaults(&rec)
	if err := world.Validate(rec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	created, err := s.cfg.Store.Create(r.Context(), rec.World())
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusCreated, s.view(r.Context(), created))
}

func (s *Server) handleGetWorld(w http.ResponseWriter, r *http.Request) {
	item, err := s.cfg.Store.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, world.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.view(r.Context(), item))
}

func (s *Server) handlePatchWorld(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.cfg.Store.Get(r.Context(), id)
	if errors.Is(err, world.ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var patch world.Record
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	rec := existing.Record()
	world.Merge(&rec, patch)
	rec.ID = id
	if err := world.Validate(rec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	updated, err := s.cfg.Store.Update(r.Context(), rec.World())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, s.view(r.Context(), updated))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
