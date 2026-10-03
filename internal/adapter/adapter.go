package adapter

import (
	"errors"
	"net"
	"time"
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

type Allocation struct {
	Host     string
	Port     int
	Protocol string
}

type World struct {
	ID           string
	Game         string
	Allocation   Allocation
	State        WorldState
	AsleepMOTD   string
	StartingMOTD string
	Replicas     int
	Backend      string
	StartTimeout time.Duration
	IdleTimeout  time.Duration
	StopTimeout  time.Duration
}

type Event struct {
	Intent Intent
	Player string
	World  *World
}

// Adapter is the per-game interface from docs/specs/edge-gateway.md.
type Adapter interface {
	Game() string
	Match(worlds []*World, alloc Allocation, first []byte) *World
	Classify(first []byte) Intent
	ShouldWake(Event) bool
	ServeStatus(rw net.Conn, world *World, state WorldState, protocol int) error
	Occupy(world *World, state WorldState) OccupyAction
	Activity(world *World) (players int, unknown bool)
	GracefulStop(world *World) error
}
