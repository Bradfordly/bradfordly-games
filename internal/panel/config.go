package panel

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

const (
	defaultAddr            = ":8081"
	defaultPublicURL       = "https://games.bradfordly.com"
	defaultAllowlistParam  = "/bradfordly-games/panel/allowlist"
	defaultOIDCSecretParam = "/bradfordly-games/panel/oidc-client-secret"
)

// Options wires the HTTP server. Tests inject fakes; FromEnv uses SSM.
type Options struct {
	Addr          string
	PublicURL     string
	SessionSecret []byte
	GitHub        GitHubConfig
	Allowlist     Checker
	Catalog       Catalog
	Gateway       Gateway
	Now           func() time.Time
}

// FromEnv builds production options. The allowlist and (unless overridden)
// the GitHub client secret come from SSM Parameter Store.
func FromEnv() (Options, error) {
	secret := os.Getenv("PANEL_SESSION_SECRET")
	if secret == "" {
		return Options{}, fmt.Errorf("PANEL_SESSION_SECRET is required")
	}
	clientID := os.Getenv("PANEL_GITHUB_CLIENT_ID")
	if clientID == "" {
		return Options{}, fmt.Errorf("PANEL_GITHUB_CLIENT_ID is required")
	}
	publicURL := strings.TrimRight(getenv("PANEL_PUBLIC_URL", defaultPublicURL), "/")
	addr := getenv("PANEL_ADDR", defaultAddr)

	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return Options{}, fmt.Errorf("aws config: %w", err)
	}
	store := NewSSMStore(ssm.NewFromConfig(awsCfg))

	clientSecret := os.Getenv("PANEL_GITHUB_CLIENT_SECRET")
	if clientSecret == "" {
		param := getenv("PANEL_GITHUB_CLIENT_SECRET_PARAMETER", defaultOIDCSecretParam)
		clientSecret, err = store.Get(ctx, param)
		if err != nil {
			return Options{}, fmt.Errorf("github client secret: %w", err)
		}
	}

	allowParam := getenv("PANEL_ALLOWLIST_PARAMETER", defaultAllowlistParam)
	var gateway Gateway = NoopGateway{}
	if gwURL := strings.TrimSpace(os.Getenv("PANEL_GATEWAY_URL")); gwURL != "" {
		gateway = NewHTTPGateway(gwURL, nil)
	}

	return Options{
		Addr:          addr,
		PublicURL:     publicURL,
		SessionSecret: []byte(secret),
		GitHub: GitHubConfig{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURI:  publicURL + "/auth/callback",
		},
		Allowlist: StoreChecker{Store: store, Name: allowParam},
		Catalog:   NewMemoryCatalog(),
		Gateway:   gateway,
	}, nil
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
