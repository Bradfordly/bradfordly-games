package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bradfordly/bradfordly-games/internal/gateway"
)

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	gateway.AdminMux(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestWorldsEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/worlds", nil)
	gateway.AdminMux(gateway.NewRuntime(nil, gateway.NewMemoryEngine())).ServeHTTP(rec, req)
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

func TestValidateListen(t *testing.T) {
	if err := validateListen("127.0.0.1:8080", "25565"); err != nil {
		t.Fatal(err)
	}
	if err := validateListen(":8080", "25565"); err == nil {
		t.Fatal("wildcard admin must fail")
	}
	if err := validateListen("127.0.0.1:25565", "25565"); err == nil {
		t.Fatal("admin must not use 25565")
	}
}
