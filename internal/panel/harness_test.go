package panel

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testEnv struct {
	server   *Server
	github   *httptest.Server
	allow    *StaticChecker
	catalog  *MemoryCatalog
	gateway  *MemoryGateway
	identity githubIdentity
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	env := &testEnv{
		allow:    &StaticChecker{Members: []string{"owner@example.com", "1"}},
		catalog:  NewMemoryCatalog(),
		gateway:  NewMemoryGateway(),
		identity: githubIdentity{Subject: "1", Email: "owner@example.com"},
	}
	env.github = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok"})
		case "/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": mustInt(env.identity.Subject), "email": env.identity.Email})
		case "/user/emails":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"email": env.identity.Email, "primary": true, "verified": true},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(env.github.Close)

	srv, err := New(Options{
		SessionSecret: []byte("test-secret-at-least-32-bytes-long"),
		PublicURL:     "https://games.bradfordly.com",
		GitHub: GitHubConfig{
			ClientID:     "id",
			ClientSecret: "sec",
			RedirectURI:  "https://games.bradfordly.com/auth/callback",
			AuthorizeURL: env.github.URL + "/login/oauth/authorize",
			TokenURL:     env.github.URL + "/login/oauth/access_token",
			UserURL:      env.github.URL + "/user",
			EmailsURL:    env.github.URL + "/user/emails",
			HTTPClient:   env.github.Client(),
		},
		Allowlist: env.allow,
		Catalog:   env.catalog,
		Gateway:   env.gateway,
		Now:       func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	env.server = srv
	return env
}

func mustInt(s string) int64 {
	n := int64(0)
	for _, c := range s {
		if c < '0' || c > '9' {
			return 1
		}
		n = n*10 + int64(c-'0')
	}
	if n == 0 {
		return 1
	}
	return n
}

type cookieClient struct {
	handler http.Handler
	cookies map[string]*http.Cookie
}

func newCookieClient(h http.Handler) *cookieClient {
	return &cookieClient{handler: h, cookies: map[string]*http.Cookie{}}
}

func (c *cookieClient) do(t *testing.T, method, path, body, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for _, cookie := range c.cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	c.handler.ServeHTTP(rec, req)
	for _, cookie := range rec.Result().Cookies() {
		c.cookies[cookie.Name] = cookie
	}
	return rec
}

func (c *cookieClient) get(t *testing.T, path string) *httptest.ResponseRecorder {
	return c.do(t, http.MethodGet, path, "", "")
}

func (env *testEnv) signIn(t *testing.T, client *cookieClient) *httptest.ResponseRecorder {
	t.Helper()
	start := client.get(t, "/auth/github")
	if start.Code != http.StatusFound {
		t.Fatalf("start status = %d", start.Code)
	}
	loc := start.Header().Get("Location")
	state := queryValue(loc, "state")
	return client.get(t, "/auth/callback?code=ok&state="+state)
}

func queryValue(raw, key string) string {
	idx := strings.Index(raw, key+"=")
	if idx < 0 {
		return ""
	}
	val := raw[idx+len(key)+1:]
	if i := strings.IndexAny(val, "&"); i >= 0 {
		val = val[:i]
	}
	return val
}
