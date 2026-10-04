package panel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNoopAndMemoryGateway(t *testing.T) {
	ctx := context.Background()
	states, err := NoopGateway{}.States(ctx)
	if err != nil || len(states) != 0 {
		t.Fatalf("noop states = %#v err=%v", states, err)
	}
	if err := (NoopGateway{}).Power(ctx, "w", "start"); err != nil {
		t.Fatal(err)
	}

	mem := NewMemoryGateway()
	wake := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	players := 2
	mem.Set(Snapshot{ID: "survival", State: "online", Players: &players, LastWakeCause: "manual", LastWakeAt: &wake})
	got, err := mem.States(ctx)
	if err != nil || got["survival"].State != "online" {
		t.Fatalf("memory states = %#v err=%v", got, err)
	}
	if err := mem.Power(ctx, "survival", "stop"); err != nil {
		t.Fatal(err)
	}
	calls := mem.Calls()
	if len(calls) != 1 || calls[0].Action != "stop" {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestSnapshotKnownPlayers(t *testing.T) {
	n := 0
	known := false
	if _, ok := (Snapshot{Players: &n, PlayersKnown: &known}).KnownPlayers(); ok {
		t.Fatal("explicit unknown must hide zero")
	}
	if _, ok := (Snapshot{}).KnownPlayers(); ok {
		t.Fatal("missing players is unknown")
	}
	if p, ok := (Snapshot{Players: &n}).KnownPlayers(); !ok || *p != 0 {
		t.Fatal("zero can be known")
	}
}

func TestHTTPGatewayStatesAndPower(t *testing.T) {
	var lastAction string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/worlds":
			_ = json.NewEncoder(w).Encode([]Snapshot{{ID: "survival", State: "failed", LastError: "boot timeout"}})
		case r.Method == http.MethodPost && r.URL.Path == "/worlds/survival/power":
			var body struct {
				Action string `json:"action"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			lastAction = body.Action
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	gw := NewHTTPGateway(srv.URL, srv.Client())
	states, err := gw.States(context.Background())
	if err != nil || states["survival"].State != "failed" {
		t.Fatalf("states = %#v err=%v", states, err)
	}
	if err := gw.Power(context.Background(), "survival", "start"); err != nil {
		t.Fatal(err)
	}
	if lastAction != "start" {
		t.Fatalf("action = %q", lastAction)
	}
}

func TestHTTPGatewayPowerNoopOnMissingRoute(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	gw := NewHTTPGateway(srv.URL+"/", srv.Client())
	if err := gw.Power(context.Background(), "survival", "stop"); err != nil {
		t.Fatalf("404 must no-op: %v", err)
	}

	method := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	t.Cleanup(method.Close)
	if err := NewHTTPGateway(method.URL, method.Client()).Power(context.Background(), "survival", "start"); err != nil {
		t.Fatalf("405 must no-op: %v", err)
	}
}

func TestHTTPGatewayErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	gw := NewHTTPGateway(srv.URL, srv.Client())
	if _, err := gw.States(context.Background()); err == nil {
		t.Fatal("states 500")
	}
	if err := gw.Power(context.Background(), "x", "start"); err == nil {
		t.Fatal("power 500")
	}
}
