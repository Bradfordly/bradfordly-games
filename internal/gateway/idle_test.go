package gateway

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
	"github.com/bradfordly/bradfordly-games/internal/mcproto"
)

func serveEchoBackend(t *testing.T) (addr string, closeFn func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				close(done)
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(io.Discard, conn)
			}(c)
		}
	}()
	return ln.Addr().String(), func() { _ = ln.Close(); <-done }
}

func waitState(t *testing.T, srv *Server, id string, want adapter.WorldState, timeout time.Duration) *adapter.World {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		w := srv.Catalog.Get(id)
		if w != nil && w.State == want {
			return w
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("world %s did not reach %s, last=%#v", id, want, srv.Catalog.Get(id))
	return nil
}

func TestIdleTimeoutScalesToZero(t *testing.T) {
	backend, stop := serveEchoBackend(t)
	defer stop()
	world := &adapter.World{
		ID:           "survival",
		Game:         adapter.GameMinecraftJava,
		Allocation:   adapter.Allocation{Host: "survival.games.bradfordly.com"},
		State:        adapter.StateAsleep,
		Backend:      backend,
		StartTimeout: time.Second,
		IdleTimeout:  80 * time.Millisecond,
		StopTimeout:  20 * time.Millisecond,
	}
	srv, addr := startTestServer(t, world)
	conn := loginAndMaybeProxy(t, addr, "survival.games.bradfordly.com", "alex")
	waitState(t, srv, "survival", adapter.StateOnline, time.Second)
	_ = conn.Close()
	waitState(t, srv, "survival", adapter.StateIdleWait, time.Second)
	waitState(t, srv, "survival", adapter.StateAsleep, time.Second)
	got := srv.Catalog.Get("survival")
	if got.Replicas != 0 {
		t.Fatalf("replicas = %d after idle, want 0", got.Replicas)
	}
	calls := srv.Scaler.(*RecordingScaler).Calls
	if len(calls) < 2 || calls[len(calls)-1].Replicas != 0 {
		t.Fatalf("scale calls = %#v, want final 0", calls)
	}
}

func TestReturningPlayerCancelsIdleTimer(t *testing.T) {
	backend, stop := serveEchoBackend(t)
	defer stop()
	world := &adapter.World{
		ID:           "survival",
		Game:         adapter.GameMinecraftJava,
		Allocation:   adapter.Allocation{Host: "survival.games.bradfordly.com"},
		State:        adapter.StateAsleep,
		Backend:      backend,
		StartTimeout: time.Second,
		IdleTimeout:  400 * time.Millisecond,
		StopTimeout:  20 * time.Millisecond,
	}
	srv, addr := startTestServer(t, world)
	c1 := loginAndMaybeProxy(t, addr, "survival.games.bradfordly.com", "alex")
	waitState(t, srv, "survival", adapter.StateOnline, time.Second)
	_ = c1.Close()
	waitState(t, srv, "survival", adapter.StateIdleWait, time.Second)
	c2 := loginAndMaybeProxy(t, addr, "survival.games.bradfordly.com", "alex")
	defer c2.Close()
	waitState(t, srv, "survival", adapter.StateOnline, time.Second)
	time.Sleep(500 * time.Millisecond)
	got := srv.Catalog.Get("survival")
	if got.State != adapter.StateOnline || got.Replicas != 1 {
		t.Fatalf("returning player should stay online: %#v", got)
	}
}

func TestStatusPingDoesNotCountAsActivity(t *testing.T) {
	backend, stop := serveEchoBackend(t)
	defer stop()
	world := &adapter.World{
		ID:           "survival",
		Game:         adapter.GameMinecraftJava,
		Allocation:   adapter.Allocation{Host: "survival.games.bradfordly.com"},
		State:        adapter.StateAsleep,
		AsleepMOTD:   "asleep",
		StartingMOTD: "starting",
		Backend:      backend,
		StartTimeout: time.Second,
		IdleTimeout:  80 * time.Millisecond,
		StopTimeout:  20 * time.Millisecond,
	}
	srv, addr := startTestServer(t, world)
	conn := loginAndMaybeProxy(t, addr, "survival.games.bradfordly.com", "alex")
	waitState(t, srv, "survival", adapter.StateOnline, time.Second)
	_ = conn.Close()
	waitState(t, srv, "survival", adapter.StateIdleWait, time.Second)
	_ = pingStatus(t, addr, "survival.games.bradfordly.com", mcproto.NextStateStatus)
	waitState(t, srv, "survival", adapter.StateAsleep, time.Second)
}
