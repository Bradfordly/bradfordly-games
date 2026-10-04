package panel

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestHealthzIsOpen(t *testing.T) {
	env := newTestEnv(t)
	rec := newCookieClient(env.server).get(t, "/healthz")
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", rec.Code)
	}
}

func TestUnauthenticatedRoutes(t *testing.T) {
	env := newTestEnv(t)
	c := newCookieClient(env.server)
	html := c.get(t, "/")
	if html.Code != http.StatusFound || html.Header().Get("Location") != "/login" {
		t.Fatalf("html = %d loc=%s", html.Code, html.Header().Get("Location"))
	}
	api := c.get(t, "/api/worlds")
	if api.Code != http.StatusUnauthorized {
		t.Fatalf("api = %d", api.Code)
	}
	login := c.get(t, "/login")
	if login.Code != http.StatusOK || !contains(login.Body.String(), "Sign in with GitHub") {
		t.Fatalf("login page = %d %s", login.Code, login.Body.String())
	}
}

func TestDeniedAndAllowlistedLogin(t *testing.T) {
	env := newTestEnv(t)
	env.identity = githubIdentity{Subject: "99", Email: "outsider@example.com"}
	c := newCookieClient(env.server)
	cb := env.signIn(t, c)
	if cb.Code != http.StatusFound || cb.Header().Get("Location") != "/denied" {
		t.Fatalf("denied redirect = %d %s", cb.Code, cb.Header().Get("Location"))
	}
	sess := c.cookies[sessionCookieName]
	if sess == nil || !sess.Secure || !sess.HttpOnly || sess.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie = %#v", sess)
	}
	denied := c.get(t, "/denied")
	if denied.Code != http.StatusForbidden || !contains(denied.Body.String(), "Denied") {
		t.Fatalf("denied page = %d %s", denied.Code, denied.Body.String())
	}
	if c.get(t, "/").Header().Get("Location") != "/denied" {
		t.Fatal("denied user must stay off the list")
	}
	if c.get(t, "/api/worlds").Code != http.StatusForbidden {
		t.Fatal("denied API")
	}

	env.identity = githubIdentity{Subject: "1", Email: "owner@example.com"}
	ok := newCookieClient(env.server)
	if loc := env.signIn(t, ok).Header().Get("Location"); loc != "/" {
		t.Fatalf("allowlisted loc = %s", loc)
	}
	list := ok.get(t, "/")
	if list.Code != http.StatusOK || !contains(list.Body.String(), "Worlds") {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
}

func TestAllowlistErrorFailsClosed(t *testing.T) {
	env := newTestEnv(t)
	env.allow.Members = []string{"owner@example.com"}
	c := newCookieClient(env.server)
	env.signIn(t, c)
	env.server.allowlist = failChecker{}
	rec := c.get(t, "/")
	if rec.Header().Get("Location") != "/denied" {
		t.Fatalf("fail closed loc = %s body=%s", rec.Header().Get("Location"), rec.Body.String())
	}
}

type failChecker struct{}

func (failChecker) Allow(context.Context, string, string) (bool, error) {
	return false, errors.New("ssm down")
}

func TestWorldPagesAndAPI(t *testing.T) {
	env := newTestEnv(t)
	env.catalog.Reset(World{
		ID:   "survival",
		Name: "Survival",
		Game: "minecraft-java",
		Allocation: Allocation{
			Host:     "survival.games.bradfordly.com",
			Port:     25565,
			Protocol: "tcp",
		},
	})
	players := 2
	wake := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	forced := time.Date(2026, 10, 4, 7, 0, 0, 0, time.UTC)
	idle := time.Date(2026, 10, 4, 12, 5, 0, 0, time.UTC)
	env.gateway.Set(Snapshot{
		ID:                 "survival",
		State:              "idle_wait",
		Players:            &players,
		IdleUntil:          &idle,
		LastWakeCause:      "manual",
		LastWakeAt:         &wake,
		LastForcedStopAt:   &forced,
		HoursOnlineLastDay: 4.5,
		LastError:          "",
		ContainerRunning:   true,
	})
	c := newCookieClient(env.server)
	env.signIn(t, c)

	list := c.get(t, "/")
	body := list.Body.String()
	for _, want := range []string{"survival.games.bradfordly.com:25565", "idle_wait", "manual", "4.5", "5m0s"} {
		if !contains(body, want) {
			t.Fatalf("list missing %q in %s", want, body)
		}
	}

	env.gateway.Set(Snapshot{ID: "survival", State: "failed", LastError: "boot timeout", ContainerRunning: false})
	detail := c.get(t, "/worlds/survival")
	body = detail.Body.String()
	for _, want := range []string{"failed", "boot timeout", "survival.games.bradfordly.com:25565", "Hours online"} {
		if !contains(body, want) {
			t.Fatalf("detail missing %q in %s", want, body)
		}
	}

	env.gateway.Set(Snapshot{ID: "survival", State: "asleep"})
	asleep := c.get(t, "/")
	if !contains(asleep.Body.String(), "asleep — join the game to start") {
		t.Fatalf("asleep hint missing: %s", asleep.Body.String())
	}

	api := c.get(t, "/api/worlds")
	var views []View
	if err := json.Unmarshal(api.Body.Bytes(), &views); err != nil || len(views) != 1 {
		t.Fatalf("api list = %s err=%v", api.Body.String(), err)
	}
	one := c.get(t, "/api/worlds/survival")
	if one.Code != http.StatusOK {
		t.Fatalf("api detail = %d", one.Code)
	}
	missing := c.get(t, "/api/worlds/nope")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing = %d", missing.Code)
	}
	if c.get(t, "/worlds/nope").Code != http.StatusNotFound {
		t.Fatal("html missing world")
	}
	if c.get(t, "/files").Code != http.StatusNotFound {
		t.Fatal("file manager must not exist")
	}
}

func TestPowerAPIAndHTML(t *testing.T) {
	env := newTestEnv(t)
	env.catalog.Reset(World{ID: "survival", Name: "Survival", Game: "minecraft-java"})
	players := 2
	env.gateway.Set(Snapshot{ID: "survival", State: "online", Players: &players})
	c := newCookieClient(env.server)
	env.signIn(t, c)

	bad := c.do(t, http.MethodPost, "/api/worlds/survival/power", `{"action":"reboot"}`, "application/json")
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad action = %d", bad.Code)
	}
	start := c.do(t, http.MethodPost, "/api/worlds/survival/power", `{"action":"start"}`, "application/json")
	if start.Code != http.StatusOK {
		t.Fatalf("start = %d %s", start.Code, start.Body.String())
	}
	html := c.do(t, http.MethodPost, "/worlds/survival/power", "action=stop", "application/x-www-form-urlencoded")
	if html.Code != http.StatusSeeOther {
		t.Fatalf("html stop = %d", html.Code)
	}
	detail := c.get(t, "/worlds/survival")
	if !contains(detail.Body.String(), `data-confirm-online="true"`) {
		t.Fatalf("confirm missing: %s", detail.Body.String())
	}
	calls := env.gateway.Calls()
	if len(calls) != 2 || calls[0].Action != "start" || calls[1].Action != "stop" {
		t.Fatalf("calls = %#v", calls)
	}
	missing := c.do(t, http.MethodPost, "/api/worlds/nope/power", `{"action":"start"}`, "application/json")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing power = %d", missing.Code)
	}
}

func TestNewValidation(t *testing.T) {
	if _, err := New(Options{}); err == nil {
		t.Fatal("empty options")
	}
	if _, err := New(Options{Allowlist: StaticChecker{}}); err == nil {
		t.Fatal("github required")
	}
}

func TestLogoutAndBadCallback(t *testing.T) {
	env := newTestEnv(t)
	c := newCookieClient(env.server)
	env.signIn(t, c)
	out := c.get(t, "/logout")
	if out.Header().Get("Location") != "/login" {
		t.Fatalf("logout loc = %s", out.Header().Get("Location"))
	}
	if c.get(t, "/").Header().Get("Location") != "/login" {
		t.Fatal("session must clear")
	}
	if c.get(t, "/auth/callback?code=x&state=nope").Header().Get("Location") != "/login" {
		t.Fatal("bad state")
	}
}

func TestFormatHelpers(t *testing.T) {
	if formatPlayers(nil) != "unknown" {
		t.Fatal("unknown players")
	}
	n := 4
	if formatPlayers(&n) != "4" {
		t.Fatal("known players")
	}
	if formatStamp(nil) != "none" {
		t.Fatal("nil stamp")
	}
	ts := time.Date(2026, 10, 4, 1, 2, 3, 0, time.UTC)
	if formatStamp(&ts) != "2026-10-04T01:02:03Z" {
		t.Fatalf("stamp = %s", formatStamp(&ts))
	}
	if formatHours(4.5) != "4.5" {
		t.Fatal("hours")
	}
}

func TestReadPowerActionJSONInvalid(t *testing.T) {
	env := newTestEnv(t)
	c := newCookieClient(env.server)
	env.signIn(t, c)
	rec := c.do(t, http.MethodPost, "/api/worlds/survival/power", `{`, "application/json")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid json = %d", rec.Code)
	}
}

func TestGitHubDefaults(t *testing.T) {
	c := GitHubConfig{}
	if c.authorizeURL() != githubAuthorizeURL || c.tokenURL() != githubTokenURL || c.userURL() != githubUserURL || c.emailsURL() != githubEmailsURL {
		t.Fatal("defaults")
	}
	if c.http() == nil {
		t.Fatal("http client")
	}
}

type failGW struct{}

func (failGW) States(context.Context) (map[string]Snapshot, error) {
	return nil, errors.New("down")
}

func (failGW) Power(context.Context, string, string) error {
	return errors.New("down")
}

func TestGatewayErrorsSurface(t *testing.T) {
	env := newTestEnv(t)
	env.catalog.Reset(World{ID: "survival", Name: "Survival"})
	c := newCookieClient(env.server)
	env.signIn(t, c)
	env.server.gateway = failGW{}
	if c.get(t, "/").Code != http.StatusBadGateway {
		t.Fatal("list gateway down")
	}
	if c.get(t, "/worlds/survival").Code != http.StatusBadGateway {
		t.Fatal("detail gateway down")
	}
	if c.get(t, "/api/worlds").Code != http.StatusBadGateway {
		t.Fatal("api list gateway down")
	}
	if c.get(t, "/api/worlds/survival").Code != http.StatusBadGateway {
		t.Fatal("api detail gateway down")
	}
	if c.do(t, http.MethodPost, "/api/worlds/survival/power", `{"action":"start"}`, "application/json").Code != http.StatusBadGateway {
		t.Fatal("api power gateway down")
	}
	if c.do(t, http.MethodPost, "/worlds/survival/power", "action=stop", "application/x-www-form-urlencoded").Code != http.StatusBadGateway {
		t.Fatal("html power gateway down")
	}
	if c.do(t, http.MethodPost, "/worlds/missing/power", "action=stop", "application/x-www-form-urlencoded").Code != http.StatusNotFound {
		t.Fatal("html power missing")
	}
}

func TestNewFillsDefaults(t *testing.T) {
	srv, err := New(Options{
		SessionSecret: []byte("secret"),
		Allowlist:     StaticChecker{},
		GitHub:        GitHubConfig{ClientID: "id", RedirectURI: "https://games.bradfordly.com/auth/callback"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if srv.catalog == nil || srv.gateway == nil || srv.now == nil {
		t.Fatal("defaults")
	}
}
