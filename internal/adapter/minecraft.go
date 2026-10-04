package adapter

import "net"

type minecraftJava struct {
	plays *PlayCounts
}

// MinecraftJava returns the v1 Minecraft Java adapter.
func MinecraftJava() Adapter {
	return NewMinecraftJava(nil)
}

// NewMinecraftJava returns the Minecraft Java adapter using plays for Activity.
func NewMinecraftJava(plays *PlayCounts) Adapter {
	return minecraftJava{plays: plays}
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

// Classify is a stub. Handshake parsing lands with the status listener.
func (minecraftJava) Classify(_ []byte) Intent { return IntentOther }

func (minecraftJava) ShouldWake(e Event) bool {
	return e.Intent == IntentLogin
}

func (minecraftJava) ServeStatus(net.Conn, *World, WorldState) error { return nil }

func (minecraftJava) Occupy(*World, WorldState) OccupyAction { return OccupyKick }

func (m minecraftJava) Activity(world *World) (int, bool) {
	if world == nil || m.plays == nil {
		return 0, false
	}
	return m.plays.Get(world.ID)
}

func (minecraftJava) GracefulStop(*World) error { return nil }
