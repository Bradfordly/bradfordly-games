package panel

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"
)

type bddWorld struct {
	env    *testEnv
	client *cookieClient
	last   *http.Response
	body   string
}

func (w *bddWorld) reset(t *testing.T) {
	w.env = newTestEnv(t)
	w.client = newCookieClient(w.env.server)
	w.last = nil
	w.body = ""
}

func (w *bddWorld) githubAuth(email, subject string) error {
	w.env.identity = githubIdentity{Subject: subject, Email: email}
	return nil
}

func (w *bddWorld) allowlistContains(member string) error {
	w.env.allow.Members = []string{member}
	return nil
}

func (w *bddWorld) worldExists(id, allocation string) error {
	host, port := splitHostPort(allocation)
	w.env.catalog.Reset(World{
		ID:   id,
		Name: id,
		Game: "minecraft-java",
		Allocation: Allocation{
			Host:     host,
			Port:     port,
			Protocol: "tcp",
		},
	})
	return nil
}

func (w *bddWorld) worldState(id, state string) error {
	w.env.gateway.Set(Snapshot{ID: id, State: state})
	return nil
}

func (w *bddWorld) worldFailedOps(id string, players int, wake string, hours float64) error {
	n := players
	wakeAt := w.env.server.now()
	forced := wakeAt.Add(-time.Hour)
	w.env.gateway.Set(Snapshot{
		ID:                 id,
		State:              "failed",
		Players:            &n,
		LastWakeCause:      wake,
		LastWakeAt:         &wakeAt,
		LastForcedStopAt:   &forced,
		HoursOnlineLastDay: hours,
		LastError:          "boot timeout",
		ContainerRunning:   false,
	})
	return nil
}

func (w *bddWorld) allowlistedSignedIn() error {
	_ = w.githubAuth("owner@example.com", "1")
	_ = w.allowlistContains("owner@example.com")
	return w.signIn()
}

func (w *bddWorld) anonGet(path string) error {
	rec := w.client.get(bddT, path)
	w.last = rec.Result()
	w.body = rec.Body.String()
	return nil
}

func (w *bddWorld) theyGet(path string) error {
	return w.anonGet(path)
}

func (w *bddWorld) signIn() error {
	rec := w.env.signIn(bddT, w.client)
	w.last = rec.Result()
	w.body = rec.Body.String()
	return nil
}

func (w *bddWorld) postPower(action, path string) error {
	body := fmt.Sprintf(`{"action":%q}`, action)
	rec := w.client.do(bddT, http.MethodPost, path, body, "application/json")
	w.last = rec.Result()
	w.body = rec.Body.String()
	return nil
}

func (w *bddWorld) statusIs(code int) error {
	if w.last == nil {
		return fmt.Errorf("no response")
	}
	if w.last.StatusCode != code {
		return fmt.Errorf("status %d, want %d body=%s", w.last.StatusCode, code, w.body)
	}
	return nil
}

func (w *bddWorld) locationIs(loc string) error {
	got := w.last.Header.Get("Location")
	if got != loc {
		return fmt.Errorf("Location %q, want %q", got, loc)
	}
	return nil
}

func (w *bddWorld) pageContains(text string) error {
	if !contains(w.body, text) {
		return fmt.Errorf("page missing %q in %s", text, w.body)
	}
	return nil
}

func (w *bddWorld) theyLandOn(path string) error {
	return w.locationIs(path)
}

func (w *bddWorld) gatewayReceived(action, id string) error {
	for _, call := range w.env.gateway.Calls() {
		if call.ID == id && call.Action == action {
			return nil
		}
	}
	return fmt.Errorf("missing power %s for %s in %#v", action, id, w.env.gateway.Calls())
}

func (w *bddWorld) sessionCookieLocked() error {
	c := w.client.cookies[sessionCookieName]
	if c == nil {
		return fmt.Errorf("missing session cookie")
	}
	if !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode {
		return fmt.Errorf("cookie flags Secure=%v HttpOnly=%v SameSite=%v", c.Secure, c.HttpOnly, c.SameSite)
	}
	return nil
}

func splitHostPort(allocation string) (string, int) {
	i := strings.LastIndex(allocation, ":")
	if i < 0 {
		return allocation, 0
	}
	port, _ := strconv.Atoi(allocation[i+1:])
	return allocation[:i], port
}

var bddT *testing.T

func TestFeatures(t *testing.T) {
	bddT = t
	suite := godog.TestSuite{
		Name: "panel",
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			var w bddWorld
			ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
				w.reset(t)
				return ctx, nil
			})
			ctx.Step(`^an anonymous client gets "([^"]*)"$`, w.anonGet)
			ctx.Step(`^they get "([^"]*)"$`, w.theyGet)
			ctx.Step(`^the response status is (\d+)$`, w.statusIs)
			ctx.Step(`^the Location header is "([^"]*)"$`, w.locationIs)
			ctx.Step(`^the page contains "([^"]*)"$`, w.pageContains)
			ctx.Step(`^GitHub will authenticate "([^"]*)" with subject "([^"]*)"$`, w.githubAuth)
			ctx.Step(`^the allowlist contains "([^"]*)"$`, w.allowlistContains)
			ctx.Step(`^the user signs in with GitHub$`, w.signIn)
			ctx.Step(`^they land on "([^"]*)"$`, w.theyLandOn)
			ctx.Step(`^a world "([^"]*)" exists with allocation "([^"]*)"$`, w.worldExists)
			ctx.Step(`^world "([^"]*)" is in gateway state "([^"]*)"$`, w.worldState)
			ctx.Step(`^world "([^"]*)" is failed with (\d+) players, wake "([^"]*)", a forced stop, and ([\d.]+) hours online$`, w.worldFailedOps)
			ctx.Step(`^an allowlisted signed-in user$`, w.allowlistedSignedIn)
			ctx.Step(`^they post power action "([^"]*)" to "([^"]*)"$`, w.postPower)
			ctx.Step(`^the gateway received power "([^"]*)" for "([^"]*)"$`, w.gatewayReceived)
			ctx.Step(`^the session cookie is Secure, HttpOnly, and SameSite=Lax$`, w.sessionCookieLocked)
		},
		Options: &godog.Options{
			TestingT: t,
			Paths:    []string{featuresPath()},
			Format:   "pretty",
			Strict:   true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("bdd scenarios failed")
	}
}

func featuresPath() string {
	if _, err := os.Stat("features/panel.feature"); err == nil {
		return "features"
	}
	return "../../features"
}
