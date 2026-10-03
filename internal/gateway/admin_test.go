package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	New("127.0.0.1:0", "127.0.0.1:0").AdminHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWorldsEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/worlds", nil)
	New("127.0.0.1:0", "127.0.0.1:0").AdminHandler().ServeHTTP(rec, req)
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

func TestWorldsListsCatalog(t *testing.T) {
	srv := New("127.0.0.1:0", "127.0.0.1:0")
	srv.Catalog.Put(&adapter.World{
		ID:    "survival",
		Game:  adapter.GameMinecraftJava,
		State: adapter.StateAsleep,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/worlds", nil)
	srv.AdminHandler().ServeHTTP(rec, req)
	var worlds []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&worlds); err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 1 || worlds[0]["id"] != "survival" || worlds[0]["state"] != "asleep" {
		t.Fatalf("worlds = %#v", worlds)
	}
}
