package adapter

import "sync"

// PlayCounts is the smallest hook for proxied play connections.
// Status pings must not be recorded here. Wake-on-login (#61) updates the count.
type PlayCounts struct {
	mu      sync.RWMutex
	byWorld map[string]int
}

// NewPlayCounts returns an empty play-connection table.
func NewPlayCounts() *PlayCounts {
	return &PlayCounts{byWorld: map[string]int{}}
}

// Set records the number of play connections for a world.
func (p *PlayCounts) Set(worldID string, n int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.byWorld == nil {
		p.byWorld = map[string]int{}
	}
	p.byWorld[worldID] = n
}

// Get returns the recorded play count. A missing world is a known zero.
func (p *PlayCounts) Get(worldID string) (int, bool) {
	if p == nil {
		return 0, false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.byWorld[worldID], false
}
