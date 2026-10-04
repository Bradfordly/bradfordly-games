package gateway

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
)

// DefaultAdminAddr is localhost-only so the admin port is never the player listener.
const DefaultAdminAddr = "127.0.0.1:8080"

// AdminMux serves GET /healthz, GET /worlds, and the mc-router scale webhook.
func AdminMux(rt *Runtime) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /worlds", func(w http.ResponseWriter, r *http.Request) {
		handleWorlds(w, r, rt)
	})
	mux.HandleFunc("POST /internal/scale", func(w http.ResponseWriter, r *http.Request) {
		handleScale(w, r, rt)
	})
	return mux
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func handleWorlds(w http.ResponseWriter, _ *http.Request, rt *Runtime) {
	w.Header().Set("Content-Type", "application/json")
	worlds := []Snapshot{}
	if rt != nil {
		worlds = rt.Snapshots()
	}
	_ = json.NewEncoder(w).Encode(worlds)
}

// AdminIsLocalhost reports whether addr binds only to a loopback interface.
func AdminIsLocalhost(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" {
		return false
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(host, "localhost")
}

func AdminIsPlayerPort(addr string) bool {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	return port == "25565"
}
