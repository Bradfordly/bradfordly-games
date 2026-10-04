package panel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Server is the games.bradfordly.com control plane.
type Server struct {
	mux       *http.ServeMux
	pages     *template.Template
	cookies   *cookieJar
	github    GitHubConfig
	allowlist Checker
	catalog   Catalog
	gateway   Gateway
	now       func() time.Time
}

// New builds the panel handler.
func New(opts Options) (*Server, error) {
	if opts.Allowlist == nil {
		return nil, errors.New("allowlist is required")
	}
	if opts.GitHub.ClientID == "" || opts.GitHub.RedirectURI == "" {
		return nil, errors.New("github oauth is not configured")
	}
	if opts.Catalog == nil {
		opts.Catalog = NewMemoryCatalog()
	}
	if opts.Gateway == nil {
		opts.Gateway = NoopGateway{}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	jar, err := newCookieJar(opts.SessionSecret, opts.Now)
	if err != nil {
		return nil, err
	}
	pages, err := template.New("").Funcs(template.FuncMap{
		"players": formatPlayers,
		"stamp":   formatStamp,
		"hours":   formatHours,
	}).ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{
		mux:       http.NewServeMux(),
		pages:     pages,
		cookies:   jar,
		github:    opts.GitHub,
		allowlist: opts.Allowlist,
		catalog:   opts.Catalog,
		gateway:   opts.Gateway,
		now:       opts.Now,
	}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /login", s.handleLogin)
	s.mux.HandleFunc("GET /auth/github", s.handleAuthStart)
	s.mux.HandleFunc("GET /auth/callback", s.handleAuthCallback)
	s.mux.HandleFunc("GET /logout", s.handleLogout)
	s.mux.HandleFunc("GET /denied", s.handleDenied)
	s.mux.HandleFunc("GET /{$}", s.handleList)
	s.mux.HandleFunc("GET /worlds/{id}", s.handleDetail)
	s.mux.HandleFunc("POST /worlds/{id}/power", s.handleHTMLPower)
	s.mux.HandleFunc("GET /api/worlds", s.handleAPIList)
	s.mux.HandleFunc("GET /api/worlds/{id}", s.handleAPIDetail)
	s.mux.HandleFunc("POST /api/worlds/{id}/power", s.handleAPIPower)
}

// ServeHTTP applies session rules. GET /healthz is the only operational
// route that skips authentication. /login, /auth/*, and /logout are the
// GitHub authorization-code flow itself.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		s.mux.ServeHTTP(w, r)
		return
	}
	if isAuthFlow(r.URL.Path) {
		s.mux.ServeHTTP(w, r)
		return
	}
	sess, err := s.cookies.readSession(r)
	if err != nil || sess == nil {
		s.unauthenticated(w, r)
		return
	}
	allowed, err := s.allowlist.Allow(r.Context(), sess.Subject, sess.Email)
	if err != nil {
		log.Printf("allowlist: %v", err)
		allowed = false
	}
	if !allowed {
		r = r.WithContext(withSession(r.Context(), sess))
		if isAPI(r.URL.Path) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "denied"})
			return
		}
		if r.URL.Path == "/denied" {
			s.mux.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "/denied", http.StatusFound)
		return
	}
	r = r.WithContext(withSession(r.Context(), sess))
	s.mux.ServeHTTP(w, r)
}

func isAuthFlow(path string) bool {
	return path == "/login" || path == "/auth/github" || path == "/auth/callback" || path == "/logout"
}

func isAPI(path string) bool {
	return strings.HasPrefix(path, "/api/")
}

func (s *Server) unauthenticated(w http.ResponseWriter, r *http.Request) {
	if isAPI(r.URL.Path) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusOK, "login.html", page{
		Title: "Login",
		User:  sessionFrom(r.Context()),
	})
}

func (s *Server) handleDenied(w http.ResponseWriter, r *http.Request) {
	s.render(w, http.StatusForbidden, "denied.html", page{
		Title: "Denied",
		User:  sessionFrom(r.Context()),
	})
}

func (s *Server) handleAuthStart(w http.ResponseWriter, r *http.Request) {
	state, err := randomState()
	if err != nil {
		http.Error(w, "could not start login", http.StatusInternalServerError)
		return
	}
	s.cookies.writeState(w, state)
	http.Redirect(w, r, s.github.authCodeURL(state), http.StatusFound)
}

func (s *Server) handleAuthCallback(w http.ResponseWriter, r *http.Request) {
	want, err := s.cookies.readState(r)
	s.cookies.clearState(w)
	if err != nil || r.URL.Query().Get("state") != want {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	ident, err := s.github.exchange(code)
	if err != nil {
		log.Printf("github exchange: %v", err)
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	allowed, err := s.allowlist.Allow(r.Context(), ident.Subject, ident.Email)
	if err != nil {
		log.Printf("allowlist: %v", err)
		allowed = false
	}
	now := s.now()
	sess := Session{
		Subject:  ident.Subject,
		Email:    ident.Email,
		Allowed:  allowed,
		IssuedAt: now,
		Expires:  now.Add(sessionTTL),
	}
	if err := s.cookies.writeSession(w, sess); err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}
	if !allowed {
		http.Redirect(w, r, "/denied", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.cookies.clearSession(w)
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	views, err := s.views(r.Context())
	if err != nil {
		log.Printf("worlds: %v", err)
		http.Error(w, "could not load worlds", http.StatusBadGateway)
		return
	}
	s.render(w, http.StatusOK, "list.html", page{
		Title:  "Worlds",
		User:   sessionFrom(r.Context()),
		Worlds: views,
	})
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	view, ok, err := s.view(r.Context(), r.PathValue("id"))
	if err != nil {
		log.Printf("world: %v", err)
		http.Error(w, "could not load world", http.StatusBadGateway)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.render(w, http.StatusOK, "detail.html", page{
		Title: view.Name,
		User:  sessionFrom(r.Context()),
		World: &view,
	})
}

func (s *Server) handleAPIList(w http.ResponseWriter, r *http.Request) {
	views, err := s.views(r.Context())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "gateway"})
		return
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *Server) handleAPIDetail(w http.ResponseWriter, r *http.Request) {
	view, ok, err := s.view(r.Context(), r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "gateway"})
		return
	}
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleAPIPower(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	action, err := readPowerAction(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.power(r.Context(), id, action); err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, errUnknownWorld) {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	view, ok, err := s.view(r.Context(), id)
	if err != nil || !ok {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleHTMLPower(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	action, err := readPowerAction(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.power(r.Context(), id, action); err != nil {
		if errors.Is(err, errUnknownWorld) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "could not change power", http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, "/worlds/"+id, http.StatusSeeOther)
}

var errUnknownWorld = errors.New("not found")

func (s *Server) power(ctx context.Context, id, action string) error {
	if _, ok := s.catalog.Get(id); !ok {
		return errUnknownWorld
	}
	return s.gateway.Power(ctx, id, action)
}

func readPowerAction(r *http.Request) (string, error) {
	if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		var body struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&body); err != nil {
			return "", fmt.Errorf("invalid json")
		}
		return validateAction(body.Action)
	}
	if err := r.ParseForm(); err != nil {
		return "", fmt.Errorf("invalid form")
	}
	return validateAction(r.FormValue("action"))
}

func validateAction(action string) (string, error) {
	switch action {
	case "start", "stop":
		return action, nil
	default:
		return "", fmt.Errorf("action must be start or stop")
	}
}

type page struct {
	Title  string
	User   *Session
	Worlds []View
	World  *View
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.pages.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("template %s: %v", name, err)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func formatPlayers(p *int) string {
	if p == nil {
		return "unknown"
	}
	return strconv.Itoa(*p)
}

func formatStamp(t *time.Time) string {
	if t == nil {
		return "none"
	}
	return t.UTC().Format(time.RFC3339)
}

func formatHours(h float64) string {
	return fmt.Sprintf("%.1f", h)
}

type sessionKey struct{}

func withSession(ctx context.Context, sess *Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, sess)
}

func sessionFrom(ctx context.Context) *Session {
	sess, _ := ctx.Value(sessionKey{}).(*Session)
	return sess
}
