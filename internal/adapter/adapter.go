package adapter

import (
	"errors"
	"net"
)

// ErrNotImplemented is returned when a follow-on UDP adapter is selected.
var ErrNotImplemented = errors.New("not implemented")

const (
	GameMinecraftJava = "minecraft-java"
	GameValheim       = "valheim"
	GamePalworld      = "palworld"
)

type Intent int

const (
	IntentOther Intent = iota
	IntentStatus
	IntentLogin
)

type OccupyAction int

const (
	OccupyKick OccupyAction = iota
	OccupyHold
	OccupyRetry
)

type WorldState string

const (
	StateAsleep   WorldState = "asleep"
	StateStarting WorldState = "starting"
	StateOnline   WorldState = "online"
	StateIdleWait WorldState = "idle_wait"
	StateStopping WorldState = "stopping"
	StateFailed   WorldState = "failed"
)

type OccupyMode string

const (
	OccupyModeKick  OccupyMode = "kick"
	OccupyModeHold  OccupyMode = "hold"
	OccupyModeRetry OccupyMode = "retry"
)

type Allocation struct {
	Host     string
	Port     int
	Protocol string
}

type Backend struct {
	Container string
	Address   string
}

type World struct {
	ID            string
	Name          string
	Game          string
	Allocation    Allocation
	Backend       Backend
	OccupyMode    OccupyMode
	AsleepMOTD    string
	StartingMOTD  string
	WakeWhitelist []string
}

type Event struct {
	Intent           Intent
	Player           string
	World            *World
	WhitelistChecked bool
}

// Adapter is the per-game interface from docs/specs/edge-gateway.md.
type Adapter interface {
	Game() string
	Match(worlds []*World, alloc Allocation, first []byte) *World
	Classify(first []byte) Intent
	ShouldWake(Event) bool
	ServeStatus(conn net.Conn, world *World, state WorldState) error
	Occupy(world *World, state WorldState) OccupyAction
	Activity(world *World) (players int, unknown bool)
	GracefulStop(world *World) error
}
