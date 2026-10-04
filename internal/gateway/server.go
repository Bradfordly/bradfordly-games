package gateway

import (
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
	"github.com/bradfordly/bradfordly-games/internal/mcproto"
)

type Server struct {
	AdminAddr    string
	GameAddr     string
	Adapters     *adapter.Registry
	Catalog      *Catalog
	Scaler       Scaler
	WakeInterval time.Duration

	wakes    *wakeGate
	activity *activity
	adminLn  net.Listener
	gameLn   net.Listener
}

func New(adminAddr, gameAddr string) *Server {
	return &Server{
		AdminAddr:    adminAddr,
		GameAddr:     gameAddr,
		Adapters:     adapter.NewRegistry(),
		Catalog:      NewCatalog(),
		Scaler:       &RecordingScaler{},
		WakeInterval: defaultWakeInterval,
	}
}

func (s *Server) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /worlds", s.handleWorlds)
	return mux
}

func (s *Server) handleWorlds(w http.ResponseWriter, _ *http.Request) {
	type row struct {
		ID       string `json:"id"`
		Game     string `json:"game"`
		State    string `json:"state"`
		Replicas int    `json:"replicas"`
	}
	worlds := s.Catalog.List()
	out := make([]row, 0, len(worlds))
	for _, world := range worlds {
		out = append(out, row{
			ID:       world.ID,
			Game:     world.Game,
			State:    string(world.State),
			Replicas: world.Replicas,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Server) Start() (adminAddr, gameAddr string, err error) {
	if s.wakes == nil {
		s.wakes = newWakeGate(s.WakeInterval)
	}
	if s.activity == nil {
		s.activity = newActivity()
	}
	s.adminLn, err = net.Listen("tcp", s.AdminAddr)
	if err != nil {
		return "", "", err
	}
	s.gameLn, err = net.Listen("tcp", s.GameAddr)
	if err != nil {
		_ = s.adminLn.Close()
		return "", "", err
	}
	go func() {
		_ = http.Serve(s.adminLn, s.AdminHandler())
	}()
	go s.serveGame()
	return s.adminLn.Addr().String(), s.gameLn.Addr().String(), nil
}

func (s *Server) Close() {
	if s.adminLn != nil {
		_ = s.adminLn.Close()
	}
	if s.gameLn != nil {
		_ = s.gameLn.Close()
	}
}

func (s *Server) serveGame() {
	for {
		conn, err := s.gameLn.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	hs, err := mcproto.ReadHandshake(conn)
	if err != nil {
		return
	}

	mc, ok := s.Adapters.ForGame(adapter.GameMinecraftJava)
	if !ok {
		return
	}
	world := mc.Match(s.Catalog.List(), adapter.Allocation{Host: hs.ServerAddress}, nil)
	if world == nil {
		return
	}

	intent := adapter.IntentFromNextState(hs.NextState)
	if intent == adapter.IntentStatus {
		_ = mc.ServeStatus(conn, world, world.State, hs.ProtocolVersion)
		return
	}

	if intent != adapter.IntentLogin {
		return
	}
	loginFrame, err := mcproto.ReadFrameBytes(conn)
	if err != nil {
		return
	}
	s.handleLogin(conn, world, hs, loginFrame)
}
