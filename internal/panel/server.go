package panel

import (
	"crypto/rand"
	"log"
	"os"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/world"
)

// Config is process settings for the control-plane HTTP server.
type Config struct {
	SessionSecret []byte
	ClientID      string
	ClientSecret  string
	RedirectURL   string
	Allowlist     Allowlist
	Exchanger     Exchanger
	Now           func() time.Time
	Store         world.Store
	States        States
	Reconcile     Reconciler
	Gateway       Gateway
}

// Server is the authenticated control plane.
type Server struct {
	cfg Config
}

// FromEnv loads panel settings from the process environment.
func FromEnv() (Config, error) {
	secret := []byte(os.Getenv("PANEL_SESSION_SECRET"))
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			return Config{}, err
		}
		log.Print("PANEL_SESSION_SECRET unset; generated an ephemeral secret")
	}
	allowlist, err := LoadAllowlist(os.Getenv("PANEL_ALLOWLIST"), os.Getenv("PANEL_ALLOWLIST_FILE"))
	if err != nil {
		return Config{}, err
	}
	redirect := os.Getenv("PANEL_OIDC_REDIRECT_URL")
	if redirect == "" {
		redirect = "https://games.bradfordly.com/oauth/callback"
	}
	return Config{
		SessionSecret: secret,
		ClientID:      os.Getenv("PANEL_GITHUB_CLIENT_ID"),
		ClientSecret:  os.Getenv("PANEL_GITHUB_CLIENT_SECRET"),
		RedirectURL:   redirect,
		Allowlist:     allowlist,
	}, nil
}

// New builds a panel server. GitHub is the v1 issuer.
func New(cfg Config) *Server {
	if cfg.Exchanger == nil {
		cfg.Exchanger = newGitHubOIDC(cfg.ClientID, cfg.ClientSecret, cfg.RedirectURL)
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Store == nil {
		cfg.Store = world.NewMemoryStore()
	}
	if cfg.Reconcile == nil {
		cfg.Reconcile = sleepingReconcilerFromEnv()
	}
	if cfg.Gateway == nil {
		cfg.Gateway = &MemoryGateway{}
	}
	return &Server{cfg: cfg}
}

func (s *Server) now() time.Time {
	return s.cfg.Now()
}
