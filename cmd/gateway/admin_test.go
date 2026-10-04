package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
	"github.com/bradfordly/bradfordly-games/internal/gateway"
)

func TestNewGatewayLoadsWorlds(t *testing.T) {
	sup, handler, err := newGateway("", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sup.List()) != 0 {
		t.Fatalf("empty worlds file should load none: %#v", sup.List())
	}
	if handler == nil {
		t.Fatal("missing admin handler")
	}

	dir := t.TempDir()
	path := dir + "/worlds.json"
	if err := os.WriteFile(path, []byte(`[{"id":"survival"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	sup, _, err = newGateway(path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sup.List()) != 1 || sup.List()[0].ID != "survival" {
		t.Fatalf("loaded = %#v", sup.List())
	}
	if _, _, err := newGateway("/no/such.json", ""); err == nil {
		t.Fatal("want worlds file error")
	}
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	adminMux(nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWorldsEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/worlds", nil)

	adminMux(nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /worlds status = %d, want %d", rec.Code, http.StatusOK)
	}

	var worlds []json.RawMessage
	if err := json.NewDecoder(rec.Body).Decode(&worlds); err != nil {
		t.Fatalf("GET /worlds JSON: %v", err)
	}
	if len(worlds) != 0 {
		t.Fatalf("GET /worlds returned %d worlds, want empty list", len(worlds))
	}
}

func TestWorldsShowsForcedStop(t *testing.T) {
	plays := adapter.NewPlayCounts()
	rt := gateway.NewFakeRuntime()
	rt.IgnoreTERM("mc-survival")
	sup := gateway.NewSupervisor(adapter.NewRegistryWithPlayCounts(plays), rt, plays)
	sup.AddWorld(adapter.World{
		ID:          "survival",
		Game:        adapter.GameMinecraftJava,
		Container:   "mc-survival",
		IdleTimeout: 15 * time.Millisecond,
		StopTimeout: 15 * time.Millisecond,
	})
	sup.MarkOnline("survival")
	sup.Tick()
	time.Sleep(20 * time.Millisecond)
	sup.Tick()
	if !sup.WaitState("survival", adapter.StateAsleep, time.Second) {
		t.Fatalf("state = %s", sup.State("survival"))
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/worlds", nil)
	adminMux(sup).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var worlds []gateway.Snapshot
	if err := json.NewDecoder(rec.Body).Decode(&worlds); err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 1 {
		t.Fatalf("worlds = %#v", worlds)
	}
	if worlds[0].State != string(adapter.StateAsleep) {
		t.Fatalf("state = %s", worlds[0].State)
	}
	if worlds[0].LastForcedStop == nil {
		t.Fatal("panel must show last_forced_stop")
	}
}
