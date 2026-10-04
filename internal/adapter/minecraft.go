package adapter

import (
	"bytes"
	"net"
	"strings"

	"github.com/bradfordly/bradfordly-games/internal/mcproto"
)

type minecraftJava struct{}

// MinecraftJava returns the v1 Minecraft Java adapter.
func MinecraftJava() Adapter {
	return minecraftJava{}
}

func (minecraftJava) Game() string { return GameMinecraftJava }

func (minecraftJava) Match(worlds []*World, alloc Allocation, first []byte) *World {
	host := mcproto.NormalizeHost(alloc.Host)
	if host == "" && len(first) > 0 {
		if hs, err := mcproto.ReadHandshake(bytes.NewReader(first)); err == nil {
			host = hs.ServerAddress
		}
	}
	for _, w := range worlds {
		if w == nil || w.Game != GameMinecraftJava {
			continue
		}
		if host != "" && mcproto.NormalizeHost(w.Allocation.Host) == host {
			return w
		}
	}
	return nil
}

func (minecraftJava) Classify(first []byte) Intent {
	hs, err := mcproto.ReadHandshake(bytes.NewReader(first))
	if err != nil {
		return IntentOther
	}
	return IntentFromNextState(hs.NextState)
}

func IntentFromNextState(next int) Intent {
	switch next {
	case mcproto.NextStateStatus:
		return IntentStatus
	case mcproto.NextStateLogin:
		return IntentLogin
	default:
		return IntentOther
	}
}

func (minecraftJava) ShouldWake(e Event) bool {
	if e.Intent != IntentLogin {
		return false
	}
	if e.World != nil && len(e.World.WakeWhitelist) > 0 {
		return onWhitelist(e.World.WakeWhitelist, e.Player)
	}
	return true
}

func onWhitelist(names []string, player string) bool {
	for _, n := range names {
		if strings.EqualFold(n, player) {
			return true
		}
	}
	return false
}

func (minecraftJava) ServeStatus(conn net.Conn, world *World, state WorldState, protocol int) error {
	motd := world.AsleepMOTD
	if state == StateStarting || state == StateStopping {
		motd = world.StartingMOTD
	}
	return mcproto.WriteStatus(conn, motd, protocol)
}

func MOTD(world *World, state WorldState) string {
	if world == nil {
		return ""
	}
	if state == StateStarting || state == StateStopping {
		return world.StartingMOTD
	}
	return world.AsleepMOTD
}

func (minecraftJava) Occupy(world *World, _ WorldState) OccupyAction {
	if world == nil {
		return OccupyKick
	}
	return world.OccupyMode
}

func (minecraftJava) Activity(*World) (int, bool) { return 0, false }

func (minecraftJava) GracefulStop(*World) error { return nil }
