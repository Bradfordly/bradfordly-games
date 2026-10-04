package gateway

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
	"github.com/bradfordly/bradfordly-games/internal/mcproto"
)

func loginAndMaybeProxy(t *testing.T, gameAddr, host, player string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", gameAddr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := mcproto.WriteHandshake(conn, mcproto.Handshake{
		ProtocolVersion: 767,
		ServerAddress:   host,
		ServerPort:      25565,
		NextState:       mcproto.NextStateLogin,
	}); err != nil {
		t.Fatal(err)
	}
	var name []byte
	name = appendVarIntForTest(name, 0)
	name = appendStringForTest(name, player)
	if err := writeFrameForTest(conn, name); err != nil {
		t.Fatal(err)
	}
	return conn
}

func appendVarIntForTest(dst []byte, value int) []byte {
	v := uint32(value)
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		dst = append(dst, b)
		if v == 0 {
			return dst
		}
	}
}

func appendStringForTest(dst []byte, s string) []byte {
	dst = appendVarIntForTest(dst, len(s))
	return append(dst, s...)
}

func writeFrameForTest(w io.Writer, payload []byte) error {
	if _, err := w.Write(appendVarIntForTest(nil, len(payload))); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func TestLoginScalesAndProxies(t *testing.T) {
	backendLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backendLn.Close() })
	got := make(chan []byte, 1)
	go func() {
		c, err := backendLn.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 256)
		n, _ := c.Read(buf)
		got <- buf[:n]
		_, _ = c.Write([]byte("ok"))
	}()

	world := &adapter.World{
		ID:           "survival",
		Game:         adapter.GameMinecraftJava,
		Allocation:   adapter.Allocation{Host: "survival.games.bradfordly.com"},
		State:        adapter.StateAsleep,
		Backend:      backendLn.Addr().String(),
		StartTimeout: time.Second,
	}
	srv, addr := startTestServer(t, world)
	conn := loginAndMaybeProxy(t, addr, "survival.games.bradfordly.com", "alex")
	defer conn.Close()

	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("backend did not receive proxied login")
	}

	calls := srv.Scaler.(*RecordingScaler).Calls
	if len(calls) != 1 || calls[0].Replicas != 1 {
		t.Fatalf("scale calls = %#v, want one scale to 1", calls)
	}
	if got := srv.Catalog.Get("survival"); got.State != adapter.StateOnline || got.Replicas != 1 {
		t.Fatalf("world after login = %#v", got)
	}
}

func TestStatusStillDoesNotScaleAfterLoginSupport(t *testing.T) {
	world := &adapter.World{
		ID:           "survival",
		Game:         adapter.GameMinecraftJava,
		Allocation:   adapter.Allocation{Host: "survival.games.bradfordly.com"},
		State:        adapter.StateAsleep,
		AsleepMOTD:   "asleep",
		StartingMOTD: "starting",
	}
	srv, addr := startTestServer(t, world)
	_ = pingStatus(t, addr, "survival.games.bradfordly.com", mcproto.NextStateStatus)
	if srv.Scaler.(*RecordingScaler).Count() != 0 {
		t.Fatal("status ping must not scale")
	}
}

func TestLoginStartTimeoutScalesBackToZero(t *testing.T) {
	world := &adapter.World{
		ID:           "survival",
		Game:         adapter.GameMinecraftJava,
		Allocation:   adapter.Allocation{Host: "survival.games.bradfordly.com"},
		State:        adapter.StateAsleep,
		Backend:      "127.0.0.1:1",
		StartTimeout: 80 * time.Millisecond,
		OccupyMode:   adapter.OccupyHold,
	}
	srv, addr := startTestServer(t, world)
	conn := loginAndMaybeProxy(t, addr, "survival.games.bradfordly.com", "alex")
	defer conn.Close()
	time.Sleep(250 * time.Millisecond)

	calls := srv.Scaler.(*RecordingScaler).Calls
	if len(calls) != 2 || calls[0].Replicas != 1 || calls[1].Replicas != 0 {
		t.Fatalf("scale calls = %#v, want 1 then 0", calls)
	}
	if got := srv.Catalog.Get("survival"); got.State != adapter.StateFailed || got.Replicas != 0 {
		t.Fatalf("world after timeout = %#v", got)
	}
}

func TestWakeRateLimit(t *testing.T) {
	world := &adapter.World{
		ID:           "survival",
		Game:         adapter.GameMinecraftJava,
		Allocation:   adapter.Allocation{Host: "survival.games.bradfordly.com"},
		State:        adapter.StateAsleep,
		Backend:      "127.0.0.1:1",
		StartTimeout: 30 * time.Millisecond,
		OccupyMode:   adapter.OccupyHold,
	}
	srv := New("127.0.0.1:0", "127.0.0.1:0")
	srv.WakeInterval = time.Hour
	srv.Scaler = &RecordingScaler{}
	srv.Catalog.Put(world)
	_, addr, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)

	c1 := loginAndMaybeProxy(t, addr, "survival.games.bradfordly.com", "alex")
	defer c1.Close()
	time.Sleep(120 * time.Millisecond)
	if got := srv.Catalog.Get("survival"); got.Replicas != 0 {
		t.Fatalf("expected timeout to scale to 0, got %#v", got)
	}

	c2 := loginAndMaybeProxy(t, addr, "survival.games.bradfordly.com", "alex")
	defer c2.Close()
	time.Sleep(80 * time.Millisecond)

	if srv.Scaler.(*RecordingScaler).Count() != 2 {
		t.Fatalf("rate-limited second wake should not scale again, calls=%#v", srv.Scaler.(*RecordingScaler).Calls)
	}
}

func TestWakeCause(t *testing.T) {
	if WakeCause("alex") != "minecraft_login:alex" {
		t.Fatalf("WakeCause = %q", WakeCause("alex"))
	}
}
