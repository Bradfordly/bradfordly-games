package panel

import (
	"testing"
)

func TestFromEnvRequiresSecrets(t *testing.T) {
	t.Setenv("PANEL_SESSION_SECRET", "")
	t.Setenv("PANEL_GITHUB_CLIENT_ID", "id")
	if _, err := FromEnv(); err == nil {
		t.Fatal("missing session secret")
	}
	t.Setenv("PANEL_SESSION_SECRET", "secret")
	t.Setenv("PANEL_GITHUB_CLIENT_ID", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("missing github client id")
	}
}

func TestFromEnvWithLocalSecret(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("PANEL_SESSION_SECRET", "secret")
	t.Setenv("PANEL_GITHUB_CLIENT_ID", "id")
	t.Setenv("PANEL_GITHUB_CLIENT_SECRET", "from-env")
	t.Setenv("PANEL_PUBLIC_URL", "https://games.bradfordly.com/")
	t.Setenv("PANEL_ADDR", ":9090")
	t.Setenv("PANEL_GATEWAY_URL", "http://127.0.0.1:8080")
	t.Setenv("PANEL_ALLOWLIST_PARAMETER", "/custom/allow")
	opts, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if opts.Addr != ":9090" || opts.PublicURL != "https://games.bradfordly.com" {
		t.Fatalf("opts = %#v", opts)
	}
	if opts.GitHub.ClientSecret != "from-env" || opts.GitHub.RedirectURI != "https://games.bradfordly.com/auth/callback" {
		t.Fatalf("github = %#v", opts.GitHub)
	}
	if _, isHTTP := opts.Gateway.(*HTTPGateway); !isHTTP {
		t.Fatalf("gateway type %T", opts.Gateway)
	}
	store, ok := opts.Allowlist.(StoreChecker)
	if !ok || store.Name != "/custom/allow" {
		t.Fatalf("allowlist = %#v", opts.Allowlist)
	}
}

func TestGetenvFallback(t *testing.T) {
	t.Setenv("PANEL_ADDR", "  ")
	if getenv("PANEL_ADDR", ":8081") != ":8081" {
		t.Fatal("blank env should fall back")
	}
}
