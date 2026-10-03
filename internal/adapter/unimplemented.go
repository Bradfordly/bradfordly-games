package adapter

import "net"

type unimplemented struct {
	game string
}

// Unimplemented is the Valheim/Palworld placeholder. UDP adapters are follow-on.
func Unimplemented(game string) Adapter {
	return unimplemented{game: game}
}

func (u unimplemented) Game() string { return u.game }

func (unimplemented) Match([]*World, Allocation, []byte) *World { return nil }

func (unimplemented) Classify([]byte) Intent { return IntentOther }

func (unimplemented) ShouldWake(Event) bool { return false }

func (unimplemented) ServeStatus(net.Conn, *World, WorldState) error {
	return ErrNotImplemented
}

func (unimplemented) Occupy(*World, WorldState) OccupyAction { return OccupyRetry }

func (unimplemented) Activity(*World) (int, bool) { return 0, true }

func (unimplemented) GracefulStop(*World) error { return ErrNotImplemented }
