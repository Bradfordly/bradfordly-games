package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubOIDC struct {
	identity Identity
	err      error
}

func (s stubOIDC) AuthCodeURL(state string) string {
	return "https://github.com/login/oauth/authorize?state=" + state
}

func (s stubOIDC) Exchange(_ context.Context, code string) (Identity, error) {
	if s.err != nil {
		return Identity{}, s.err
	}
	if code != "ok" {
		return Identity{}, errTest("bad code")
	}
	return s.identity, nil
}

type errTest string

func (e errTest) Error() string { return string(e) }

func testServer(allowCSV string, id Identity) *Server {
	allowlist, err := LoadAllowlist(allowCSV, "")
	if err != nil {
		panic(err)
	}
	return New(Config{
		SessionSecret: []byte("test-session-secret"),
		ClientID:      "test-client",
		ClientSecret:  "test-secret",
		RedirectURL:   "https://games.bradfordly.com/oauth/callback",
		Allowlist:     allowlist,
		Exchanger:     stubOIDC{identity: id},
	})
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	testServer("", Identity{}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHTMLRequiresSession(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	testServer("bradfordly", Identity{}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("GET / Location = %q, want /login", loc)
	}
}

func TestAPIRequiresSession(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/worlds", nil)

	testServer("bradfordly", Identity{}).Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/worlds status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestOIDCAllowlistDeny(t *testing.T) {
	srv := testServer("bradfordly", Identity{Subject: "9", Login: "stranger", Email: "x@example.com"})
	rec := callback(t, srv, "ok")

	if rec.Code != http.StatusFound {
		t.Fatalf("callback status = %d, want %d", rec.Code, http.StatusFound)
	}
	if loc := rec.Header().Get("Location"); loc != "/denied" {
		t.Fatalf("callback Location = %q, want /denied", loc)
	}
	assertCookieFlags(t, rec, sessionCookie)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookieFrom(t, rec, sessionCookie))
	home := httptest.NewRecorder()
	srv.Handler().ServeHTTP(home, req)
	if home.Code != http.StatusFound || home.Header().Get("Location") != "/denied" {
		t.Fatalf("allowlist deny HTML: status=%d loc=%q", home.Code, home.Header().Get("Location"))
	}

	api := httptest.NewRecorder()
	apiReq := httptest.NewRequest(http.MethodGet, "/api/worlds", nil)
	apiReq.AddCookie(cookieFrom(t, rec, sessionCookie))
	srv.Handler().ServeHTTP(api, apiReq)
	if api.Code != http.StatusForbidden {
		t.Fatalf("allowlist deny API status = %d, want %d", api.Code, http.StatusForbidden)
	}

	denied := httptest.NewRecorder()
	deniedReq := httptest.NewRequest(http.MethodGet, "/denied", nil)
	deniedReq.AddCookie(cookieFrom(t, rec, sessionCookie))
	srv.Handler().ServeHTTP(denied, deniedReq)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("GET /denied status = %d, want %d", denied.Code, http.StatusForbidden)
	}
	if !strings.Contains(denied.Body.String(), "not allowlisted") {
		t.Fatalf("GET /denied body = %q", denied.Body.String())
	}
}

func TestOIDCAllowlistAllow(t *testing.T) {
	srv := testServer("bradfordly", Identity{Subject: "1", Login: "bradfordly", Email: "me@example.com"})
	rec := callback(t, srv, "ok")

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("callback status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	assertCookieFlags(t, rec, sessionCookie)

	home := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookieFrom(t, rec, sessionCookie))
	srv.Handler().ServeHTTP(home, req)
	if home.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", home.Code, http.StatusOK)
	}
}

func TestLogoutClearsSession(t *testing.T) {
	srv := testServer("bradfordly", Identity{Login: "bradfordly"})
	loggedIn := callback(t, srv, "ok")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/logout", nil)
	req.AddCookie(cookieFrom(t, loggedIn, sessionCookie))
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
		t.Fatalf("logout status=%d loc=%q", rec.Code, rec.Header().Get("Location"))
	}
	cleared := cookieFrom(t, rec, sessionCookie)
	if cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Fatalf("logout cookie value=%q maxAge=%d", cleared.Value, cleared.MaxAge)
	}
	assertCookieFlags(t, rec, sessionCookie)

	home := httptest.NewRecorder()
	homeReq := httptest.NewRequest(http.MethodGet, "/", nil)
	homeReq.AddCookie(cleared)
	srv.Handler().ServeHTTP(home, homeReq)
	if home.Code != http.StatusFound || home.Header().Get("Location") != "/login" {
		t.Fatalf("after logout GET / status=%d loc=%q", home.Code, home.Header().Get("Location"))
	}
}

func TestLoginRedirectsToGitHubIssuer(t *testing.T) {
	srv := testServer("bradfordly", Identity{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("GET /login status = %d, want %d", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://github.com/login/oauth/authorize") {
		t.Fatalf("login Location = %q, want GitHub issuer authorize URL", loc)
	}
	assertCookieFlags(t, rec, stateCookie)
}

func callback(t *testing.T, srv *Server, code string) *httptest.ResponseRecorder {
	t.Helper()
	login := httptest.NewRecorder()
	srv.Handler().ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/login", nil))
	state := cookieFrom(t, login, stateCookie)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/oauth/callback?code="+code+"&state="+state.Value, nil)
	req.AddCookie(state)
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func cookieFrom(t *testing.T, rec *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}

func assertCookieFlags(t *testing.T, rec *httptest.ResponseRecorder, name string) {
	t.Helper()
	c := cookieFrom(t, rec, name)
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("%s flags: secure=%v httpOnly=%v sameSite=%v", name, c.Secure, c.HttpOnly, c.SameSite)
	}
}
