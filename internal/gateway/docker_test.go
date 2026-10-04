package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDockerRuntimeGracefulStop(t *testing.T) {
	var mu sync.Mutex
	running := true
	var signals []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/json"):
			_ = json.NewEncoder(w).Encode(map[string]any{"State": map[string]any{"Running": running}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/kill"):
			sig := r.URL.Query().Get("signal")
			signals = append(signals, sig)
			if sig == "SIGTERM" {
				running = false
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	rt := NewDockerHTTPRuntime(srv.Client(), srv.URL)
	forced, err := rt.Stop(context.Background(), "mc-survival", 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if forced {
		t.Fatal("graceful stop must not be forced")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(signals) != 1 || signals[0] != "SIGTERM" {
		t.Fatalf("signals = %#v", signals)
	}
}

func TestDockerRuntimeForcedKill(t *testing.T) {
	var mu sync.Mutex
	running := true
	var signals []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/json"):
			_ = json.NewEncoder(w).Encode(map[string]any{"State": map[string]any{"Running": running}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/kill"):
			sig := r.URL.Query().Get("signal")
			signals = append(signals, sig)
			if sig == "SIGKILL" {
				running = false
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	rt := NewDockerHTTPRuntime(srv.Client(), srv.URL)
	forced, err := rt.Stop(context.Background(), "mc-survival", 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if !forced {
		t.Fatal("want forced stop after timeout")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(signals) < 2 || signals[0] != "SIGTERM" || signals[len(signals)-1] != "SIGKILL" {
		t.Fatalf("signals = %#v", signals)
	}
}

func TestDockerRuntimeAlreadyStopped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/kill") {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"message":"not running"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	rt := NewDockerHTTPRuntime(srv.Client(), srv.URL)
	forced, err := rt.Stop(context.Background(), "mc-survival", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if forced {
		t.Fatal("already stopped is not forced")
	}
}

func TestDockerRuntimeContainerGoneAfterTerm(t *testing.T) {
	var mu sync.Mutex
	killed := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/kill"):
			killed = true
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodGet && killed:
			http.NotFound(w, r)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"State": map[string]any{"Running": true}})
		}
	}))
	defer srv.Close()

	rt := NewDockerHTTPRuntime(srv.Client(), srv.URL)
	forced, err := rt.Stop(context.Background(), "mc-survival", 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if forced {
		t.Fatal("disappeared container is a graceful stop")
	}
}

func TestDockerRuntimeHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/kill") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(strings.Repeat("x", 220)))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("inspect failed"))
	}))
	defer srv.Close()
	rt := NewDockerHTTPRuntime(srv.Client(), srv.URL)
	if _, err := rt.Running(context.Background(), "mc"); err == nil {
		t.Fatal("want inspect error")
	}
	if _, err := rt.Stop(context.Background(), "mc", time.Millisecond); err == nil {
		t.Fatal("want kill error")
	}
}

func TestNewDockerRuntime(t *testing.T) {
	rt := NewDockerRuntime("")
	if rt == nil || rt.base != "http://docker" {
		t.Fatalf("runtime = %#v", rt)
	}
}

func TestDockerErrorHelpers(t *testing.T) {
	err := &dockerError{status: 404, msg: "missing"}
	if err.Error() == "" || !isNotFound(err) || isNotRunning(err) {
		t.Fatalf("helpers = %v", err)
	}
	if bytesPreview([]byte("short")) != "short" {
		t.Fatal("short preview")
	}
}

func TestDockerRuntimeInspectErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	rt := NewDockerHTTPRuntime(srv.Client(), srv.URL)
	if _, err := rt.Running(context.Background(), "missing"); err == nil {
		t.Fatal("want inspect error")
	}
}
