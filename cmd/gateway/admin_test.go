package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	adminMux().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /healthz status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestDrainAndReload(t *testing.T) {
	rec := httptest.NewRecorder()
	adminMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/worlds/abc/drain", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("drain status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	rec = httptest.NewRecorder()
	adminMux().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/reload", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reload status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

func TestWorldsEmpty(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/worlds", nil)

	adminMux().ServeHTTP(rec, req)

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
