package world

import "strconv"

// Runtime is gateway-owned state. The panel reads it and does not persist it on the CR.
type Runtime struct {
	State         string   `json:"state,omitempty"`
	PlayerCount   *int     `json:"player_count,omitempty"`
	IdleRemaining string   `json:"idle_remaining,omitempty"`
	LastWake      string   `json:"last_wake,omitempty"`
	LastError     string   `json:"last_error,omitempty"`
	Transitions   []string `json:"transitions,omitempty"`
}

// View is a stored world plus gateway runtime for the API and HTML pages.
type View struct {
	Record
	Runtime
}

// AsleepMessage is the list/detail copy when replicas are 0.
const AsleepMessage = "asleep — join the game to start"

// AllocationString is host:port for players.
func (r Record) AllocationString() string {
	if r.Allocation.Host == "" && r.Allocation.Port == 0 {
		return ""
	}
	if r.Allocation.Port == 0 {
		return r.Allocation.Host
	}
	return r.Allocation.Host + ":" + strconv.Itoa(r.Allocation.Port)
}
