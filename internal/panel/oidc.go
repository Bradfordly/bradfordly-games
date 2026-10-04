package panel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// GitHubIssuer is the v1 OIDC/OAuth issuer for the panel.
const GitHubIssuer = "https://github.com/login/oauth"

const (
	githubAuthorizeURL = "https://github.com/login/oauth/authorize"
	githubTokenURL     = "https://github.com/login/oauth/access_token"
	githubUserURL      = "https://api.github.com/user"
	githubEmailsURL    = "https://api.github.com/user/emails"
)

// Identity is a GitHub user after the authorization-code exchange.
type Identity struct {
	Subject string
	Login   string
	Email   string
}

// Exchanger runs the GitHub authorization-code flow.
type Exchanger interface {
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (Identity, error)
}

type githubOIDC struct {
	clientID     string
	clientSecret string
	redirectURL  string
	httpClient   *http.Client
}

func newGitHubOIDC(clientID, clientSecret, redirectURL string) *githubOIDC {
	return &githubOIDC{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (g *githubOIDC) AuthCodeURL(state string) string {
	q := url.Values{
		"client_id":    {g.clientID},
		"redirect_uri": {g.redirectURL},
		"scope":        {"read:user user:email"},
		"state":        {state},
	}
	return githubAuthorizeURL + "?" + q.Encode()
}

func (g *githubOIDC) Exchange(ctx context.Context, code string) (Identity, error) {
	form := url.Values{
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"code":          {code},
		"redirect_uri":  {g.redirectURL},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, githubTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var token struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := g.decodeJSON(req, &token); err != nil {
		return Identity{}, err
	}
	if token.Error != "" {
		return Identity{}, fmt.Errorf("github token: %s", token.Error)
	}
	if token.AccessToken == "" {
		return Identity{}, fmt.Errorf("github token: empty access_token")
	}

	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
	}
	if err := g.getJSON(ctx, githubUserURL, token.AccessToken, &user); err != nil {
		return Identity{}, err
	}
	if user.ID == 0 || user.Login == "" {
		return Identity{}, fmt.Errorf("github user: missing id or login")
	}

	id := Identity{
		Subject: strconv.FormatInt(user.ID, 10),
		Login:   user.Login,
		Email:   user.Email,
	}
	if id.Email == "" {
		id.Email, _ = g.primaryEmail(ctx, token.AccessToken)
	}
	return id, nil
}

func (g *githubOIDC) primaryEmail(ctx context.Context, accessToken string) (string, error) {
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := g.getJSON(ctx, githubEmailsURL, accessToken, &emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified && e.Email != "" {
			return e.Email, nil
		}
	}
	return "", nil
}

func (g *githubOIDC) getJSON(ctx context.Context, rawURL, accessToken string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	return g.decodeJSON(req, dest)
}

func (g *githubOIDC) decodeJSON(req *http.Request, dest any) error {
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("github %s: %s", req.URL.Path, resp.Status)
	}
	return json.Unmarshal(body, dest)
}
