package world

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPGatewayDrainAndReload(t *testing.T) {
	var drain, reload string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /worlds/{id}/drain", func(w http.ResponseWriter, r *http.Request) {
		drain = r.PathValue("id")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /reload", func(w http.ResponseWriter, _ *http.Request) {
		reload = "ok"
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	g := NewHTTPGateway(srv.URL + "/")
	if err := g.Drain(context.Background(), "abc"); err != nil {
		t.Fatal(err)
	}
	if err := g.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if drain != "abc" || reload != "ok" {
		t.Fatalf("drain=%q reload=%q", drain, reload)
	}
}

func TestHTTPGatewayUnreachable(t *testing.T) {
	g := NewHTTPGateway("http://127.0.0.1:1")
	if err := g.Reload(context.Background()); err == nil {
		t.Fatal("expected dial error")
	}
}

func TestHTTPGatewayErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	g := NewHTTPGateway(srv.URL)
	if err := g.Drain(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
}
