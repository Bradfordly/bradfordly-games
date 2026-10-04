package main

import (
	"encoding/json"
	"net/http"
)

func adminMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /worlds", handleWorlds)
	mux.HandleFunc("POST /worlds/{id}/drain", handleDrain)
	mux.HandleFunc("POST /reload", handleReload)
	return mux
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func handleWorlds(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]any{})
}

func handleDrain(w http.ResponseWriter, _ *http.Request) {
	// Wake/idle (#61/#62) own real drain. A stopped world is already drained.
	w.WriteHeader(http.StatusNoContent)
}

func handleReload(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
