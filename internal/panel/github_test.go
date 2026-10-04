package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubAuthCodeURL(t *testing.T) {
	c := GitHubConfig{
		ClientID:     "id",
		RedirectURI:  "https://games.bradfordly.com/auth/callback",
		AuthorizeURL: "https://github.com/login/oauth/authorize",
	}
	u := c.authCodeURL("st")
	if u == "" || !containsAll(u, "client_id=id", "state=st", "user%3Aemail") {
		t.Fatalf("auth url = %s", u)
	}
}

func TestGitHubExchange(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
		case "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 7, "email": ""})
		case "/user/emails":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"email": "other@x.com", "primary": false, "verified": true},
				{"email": "owner@example.com", "primary": true, "verified": true},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(gh.Close)

	c := GitHubConfig{
		ClientID:     "id",
		ClientSecret: "sec",
		RedirectURI:  "https://games.bradfordly.com/auth/callback",
		TokenURL:     gh.URL + "/login/oauth/access_token",
		UserURL:      gh.URL + "/user",
		EmailsURL:    gh.URL + "/user/emails",
		HTTPClient:   gh.Client(),
	}
	ident, err := c.exchange("code")
	if err != nil {
		t.Fatal(err)
	}
	if ident.Subject != "7" || ident.Email != "owner@example.com" {
		t.Fatalf("ident = %#v", ident)
	}
}

func TestGitHubExchangeErrors(t *testing.T) {
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/oauth/access_token" {
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad_code", "error_description": "nope"})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(gh.Close)
	c := GitHubConfig{TokenURL: gh.URL + "/login/oauth/access_token", HTTPClient: gh.Client(), ClientID: "id", ClientSecret: "s"}
	if _, err := c.exchange("x"); err == nil {
		t.Fatal("token error must surface")
	}

	failHTTP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	t.Cleanup(failHTTP.Close)
	c = GitHubConfig{TokenURL: failHTTP.URL, HTTPClient: failHTTP.Client(), ClientID: "id", ClientSecret: "s"}
	if _, err := c.exchange("x"); err == nil {
		t.Fatal("token http error must surface")
	}

	missingID := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
		case "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 0, "email": "a@b.com"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(missingID.Close)
	c = GitHubConfig{
		TokenURL: missingID.URL + "/token", UserURL: missingID.URL + "/user",
		HTTPClient: missingID.Client(), ClientID: "id", ClientSecret: "s",
	}
	if _, err := c.exchange("x"); err == nil {
		t.Fatal("missing github id must fail")
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
