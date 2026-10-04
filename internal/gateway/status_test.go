package gateway

import (
	"bufio"
	"net"
	"testing"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
	"github.com/bradfordly/bradfordly-games/internal/mcproto"
)

func startTestServer(t *testing.T, world *adapter.World) (*Server, string) {
	t.Helper()
	srv := New("127.0.0.1:0", "127.0.0.1:0")
	scaler := &RecordingScaler{}
	srv.Scaler = scaler
	srv.Catalog.Put(world)
	_, gameAddr, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	return srv, gameAddr
}

func pingStatus(t *testing.T, gameAddr, host string, next int) map[string]any {
	t.Helper()
	conn, err := net.DialTimeout("tcp", gameAddr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	if err := mcproto.WriteHandshake(conn, mcproto.Handshake{
		ProtocolVersion: 767,
		ServerAddress:   host,
		ServerPort:      25565,
		NextState:       next,
	}); err != nil {
		t.Fatal(err)
	}
	if next != mcproto.NextStateStatus {
		return nil
	}
	if err := mcproto.WriteStatusRequest(conn); err != nil {
		t.Fatal(err)
	}
	status, err := mcproto.ReadStatusJSON(bufio.NewReader(conn))
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func motdText(t *testing.T, status map[string]any) string {
	t.Helper()
	desc, ok := status["description"].(map[string]any)
	if !ok {
		t.Fatalf("description = %#v", status["description"])
	}
	text, _ := desc["text"].(string)
	return text
}

func TestStatusAsleepMOTDDoesNotScale(t *testing.T) {
	world := &adapter.World{
		ID:           "survival",
		Game:         adapter.GameMinecraftJava,
		Allocation:   adapter.Allocation{Host: "survival.games.bradfordly.com", Port: 25565, Protocol: "tcp"},
		State:        adapter.StateAsleep,
		AsleepMOTD:   "World is asleep",
		StartingMOTD: "World is starting",
	}
	srv, addr := startTestServer(t, world)

	status := pingStatus(t, addr, "survival.games.bradfordly.com", mcproto.NextStateStatus)
	if got := motdText(t, status); got != "World is asleep" {
		t.Fatalf("motd = %q, want asleep", got)
	}
	if srv.Scaler.(*RecordingScaler).Count() != 0 {
		t.Fatal("status ping must not scale replicas")
	}
	mc, _ := srv.Adapters.ForGame(adapter.GameMinecraftJava)
	if players, unknown := mc.Activity(world); players != 0 || unknown {
		t.Fatalf("status is not activity: players=%d unknown=%v", players, unknown)
	}
}

func TestStatusStartingMOTDDoesNotScale(t *testing.T) {
	world := &adapter.World{
		ID:           "survival",
		Game:         adapter.GameMinecraftJava,
		Allocation:   adapter.Allocation{Host: "survival.games.bradfordly.com", Port: 25565, Protocol: "tcp"},
		State:        adapter.StateStarting,
		AsleepMOTD:   "World is asleep",
		StartingMOTD: "World is starting",
	}
	srv, addr := startTestServer(t, world)

	status := pingStatus(t, addr, "Survival.Games.Bradfordly.com.", mcproto.NextStateStatus)
	if got := motdText(t, status); got != "World is starting" {
		t.Fatalf("motd = %q, want starting", got)
	}
	if srv.Scaler.(*RecordingScaler).Count() != 0 {
		t.Fatal("status ping must not scale replicas")
	}
}

func TestLoginDoesNotScaleYet(t *testing.T) {
	world := &adapter.World{
		ID:         "survival",
		Game:       adapter.GameMinecraftJava,
		Allocation: adapter.Allocation{Host: "survival.games.bradfordly.com"},
		State:      adapter.StateAsleep,
	}
	srv, addr := startTestServer(t, world)
	_ = pingStatus(t, addr, "survival.games.bradfordly.com", mcproto.NextStateLogin)
	time.Sleep(50 * time.Millisecond)
	if srv.Scaler.(*RecordingScaler).Count() != 0 {
		t.Fatal("login must not scale until #28")
	}
}
