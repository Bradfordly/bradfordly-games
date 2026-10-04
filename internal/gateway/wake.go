package gateway

import (
	"io"
	"log"
	"net"
	"sync"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
	"github.com/bradfordly/bradfordly-games/internal/mcproto"
)

const defaultWakeInterval = 30 * time.Second
const defaultStartTimeout = 10 * time.Minute
const kickHold = 100 * time.Millisecond

func WakeCause(player string) string {
	return "minecraft_login:" + player
}

type wakeGate struct {
	mu       sync.Mutex
	locks    map[string]*sync.Mutex
	lastWake map[string]time.Time
	interval time.Duration
}

func newWakeGate(interval time.Duration) *wakeGate {
	if interval <= 0 {
		interval = defaultWakeInterval
	}
	return &wakeGate{
		locks:    map[string]*sync.Mutex{},
		lastWake: map[string]time.Time{},
		interval: interval,
	}
}

func (g *wakeGate) lock(id string) *sync.Mutex {
	g.mu.Lock()
	defer g.mu.Unlock()
	m, ok := g.locks[id]
	if !ok {
		m = &sync.Mutex{}
		g.locks[id] = m
	}
	return m
}

func (g *wakeGate) allow(id string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	last, ok := g.lastWake[id]
	if ok && time.Since(last) < g.interval {
		return false
	}
	g.lastWake[id] = time.Now()
	return true
}

func (s *Server) handleLogin(conn net.Conn, world *adapter.World, hs mcproto.Handshake, loginFrame []byte) {
	player, err := mcproto.LoginName(loginFrame)
	if err != nil {
		player = "unknown"
	}

	mc, ok := s.Adapters.ForGame(adapter.GameMinecraftJava)
	if !ok || !mc.ShouldWake(adapter.Event{Intent: adapter.IntentLogin, Player: player, World: world}) {
		log.Printf("false-wake reject world=%s player=%s", world.ID, player)
		return
	}
	cause := WakeCause(player)
	log.Printf("wake world=%s cause=%s", world.ID, cause)

	lock := s.wakes.lock(world.ID)
	lock.Lock()
	ready := false
	current := s.Catalog.Get(world.ID)
	if current == nil {
		lock.Unlock()
		return
	}
	if current.Replicas == 0 {
		if !s.wakes.allow(world.ID) {
			log.Printf("wake rate-limited world=%s", world.ID)
			lock.Unlock()
			return
		}
		if err := s.Scaler.SetReplicas(world.ID, 1); err != nil {
			log.Printf("scale_up failed world=%s: %v", world.ID, err)
			s.Catalog.SetState(world.ID, adapter.StateFailed)
			lock.Unlock()
			return
		}
		s.Catalog.SetReplicas(world.ID, 1)
		s.Catalog.SetState(world.ID, adapter.StateStarting)
		current = s.Catalog.Get(world.ID)
	}

	timeout := current.StartTimeout
	if timeout <= 0 {
		timeout = defaultStartTimeout
	}
	action := mc.Occupy(current, current.State)
	hold := timeout
	if action == adapter.OccupyKick {
		hold = kickHold
	}
	if action == adapter.OccupyRetry {
		hold = 0
	}
	ready = waitReady(current.Backend, hold)
	if ready {
		s.Catalog.SetState(world.ID, adapter.StateOnline)
	} else if action == adapter.OccupyHold {
		s.Catalog.SetState(world.ID, adapter.StateFailed)
		_ = s.Scaler.SetReplicas(world.ID, 0)
		s.Catalog.SetReplicas(world.ID, 0)
	}
	backend := current.Backend
	lock.Unlock()

	if ready {
		s.proxyLogin(conn, world.ID, backend, hs, loginFrame)
		return
	}
	if action == adapter.OccupyKick {
		_ = mcproto.WriteLoginDisconnect(conn, mcproto.StartingKickMessage)
	}
}

func waitReady(addr string, timeout time.Duration) bool {
	if addr == "" {
		return false
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func (s *Server) proxyLogin(client net.Conn, worldID, backendAddr string, hs mcproto.Handshake, loginFrame []byte) {
	backend, err := net.DialTimeout("tcp", backendAddr, 2*time.Second)
	if err != nil {
		return
	}
	defer backend.Close()
	s.noteJoin(worldID)
	defer s.noteLeave(worldID)
	_ = client.SetDeadline(time.Time{})
	if err := mcproto.WriteHandshake(backend, hs); err != nil {
		return
	}
	if _, err := backend.Write(loginFrame); err != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(backend, client)
		_ = backend.Close()
		close(done)
	}()
	_, _ = io.Copy(client, backend)
	_ = client.Close()
	<-done
}
