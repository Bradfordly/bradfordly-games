package main

import (
	"encoding/json"
	"net/http"

	"github.com/bradfordly/bradfordly-games/internal/gateway"
)

type worldLister interface {
	List() []gateway.Snapshot
}

func adminMux(worlds worldLister) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /worlds", func(w http.ResponseWriter, r *http.Request) {
		handleWorlds(w, r, worlds)
	})
	return mux
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func handleWorlds(w http.ResponseWriter, _ *http.Request, worlds worldLister) {
	w.Header().Set("Content-Type", "application/json")
	list := []gateway.Snapshot{}
	if worlds != nil {
		list = worlds.List()
		if list == nil {
			list = []gateway.Snapshot{}
		}
	}
	_ = json.NewEncoder(w).Encode(list)
}
