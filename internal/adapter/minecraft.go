package adapter

import (
	"net"
	"strings"
)

// DefaultStartingMOTD is the occupy-kick message from docs/specs/game-adapters.md.
const DefaultStartingMOTD = "Server is starting, join again."

type minecraftJava struct{}

// MinecraftJava returns the v1 Minecraft Java adapter.
// Handshake parsing is owned by itzg/mc-router, not this type.
func MinecraftJava() Adapter {
	return minecraftJava{}
}

func (minecraftJava) Game() string { return GameMinecraftJava }

func (minecraftJava) Match(worlds []*World, alloc Allocation, _ []byte) *World {
	for _, w := range worlds {
		if w == nil || w.Game != GameMinecraftJava {
			continue
		}
		if alloc.Host != "" && w.Allocation.Host == alloc.Host {
			return w
		}
	}
	return nil
}

// Classify does not parse a handshake. mc-router already classified the intent.
func (minecraftJava) Classify(_ []byte) Intent { return IntentOther }

func (minecraftJava) ShouldWake(e Event) bool {
	if e.Intent != IntentLogin {
		return false
	}
	if e.WhitelistChecked {
		return true
	}
	return WakeAllowed(e.World, e.Player)
}

func (minecraftJava) ServeStatus(net.Conn, *World, WorldState) error { return nil }

func (minecraftJava) Occupy(world *World, _ WorldState) OccupyAction {
	if world == nil {
		return OccupyKick
	}
	switch world.OccupyMode {
	case OccupyModeHold:
		return OccupyHold
	case OccupyModeRetry:
		return OccupyRetry
	default:
		return OccupyKick
	}
}

func (minecraftJava) Activity(*World) (int, bool) { return 0, false }

func (minecraftJava) GracefulStop(*World) error { return nil }

// WakeAllowed is false when wake_whitelist is set and the login name is absent.
func WakeAllowed(world *World, player string) bool {
	if world == nil || len(world.WakeWhitelist) == 0 {
		return true
	}
	player = strings.TrimSpace(player)
	if player == "" {
		return false
	}
	for _, name := range world.WakeWhitelist {
		if strings.EqualFold(strings.TrimSpace(name), player) {
			return true
		}
	}
	return false
}

// StartingMessage is the MOTD players see while a world is booting.
func StartingMessage(world *World) string {
	if world != nil && strings.TrimSpace(world.StartingMOTD) != "" {
		return world.StartingMOTD
	}
	return DefaultStartingMOTD
}
