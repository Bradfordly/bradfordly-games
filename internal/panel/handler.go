package panel

import (
	"crypto/subtle"
	"io"
	"net/http"
	"strings"
)

// Handler returns the control-plane HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealthz)
	mux.HandleFunc("GET /login", s.handleLogin)
	mux.HandleFunc("GET /oauth/callback", s.handleCallback)
	mux.HandleFunc("GET /logout", s.handleLogout)
	mux.HandleFunc("GET /denied", s.handleDenied)
	mux.HandleFunc("GET /{$}", s.handleHome)
	mux.HandleFunc("GET /worlds/{id}", s.handleWorldDetail)
	mux.HandleFunc("GET /api/worlds", s.handleListWorlds)
	mux.HandleFunc("POST /api/worlds", s.handleCreateWorld)
	mux.HandleFunc("GET /api/worlds/{id}", s.handleGetWorld)
	mux.HandleFunc("PATCH /api/worlds/{id}", s.handlePatchWorld)
	return s.requireSession(mux)
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func publicPath(path string) bool {
	switch path {
	case "/healthz", "/login", "/oauth/callback":
		return true
	default:
		return false
	}
}

func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		sess, ok := s.readSession(r)
		if !ok {
			s.denyUnauthenticated(w, r)
			return
		}
		if r.URL.Path == "/denied" || r.URL.Path == "/logout" {
			next.ServeHTTP(w, r)
			return
		}
		if !sess.Allowed {
			s.denyNotAllowlisted(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) denyUnauthenticated(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) denyNotAllowlisted(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api") {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	http.Redirect(w, r, "/denied", http.StatusFound)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := s.writeState(w)
	if err != nil {
		http.Error(w, "login failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, s.cfg.Exchanger.AuthCodeURL(state), http.StatusFound)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	want := readState(r)
	got := r.URL.Query().Get("state")
	if want == "" || subtle.ConstantTimeCompare([]byte(want), []byte(got)) != 1 {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, newCookie(stateCookie, "", -1))

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	id, err := s.cfg.Exchanger.Exchange(r.Context(), code)
	if err != nil {
		http.Error(w, "oidc exchange failed", http.StatusBadGateway)
		return
	}

	allowed := s.cfg.Allowlist.Contains(id)
	if err := s.writeSession(w, id, allowed); err != nil {
		http.Error(w, "session failed", http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Redirect(w, r, "/denied", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleDenied(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	_, _ = io.WriteString(w, "<!DOCTYPE html><html><body>authenticated but not allowlisted</body></html>")
}
