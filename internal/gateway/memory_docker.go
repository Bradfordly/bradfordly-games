package gateway

import (
	"context"
	"sync"
)

// MemoryEngine is an in-process stand-in for Docker used by tests and BDD.
type MemoryEngine struct {
	mu      sync.Mutex
	running map[string]bool
	starts  []string
	startFn func(string) error
}

func NewMemoryEngine(running ...string) *MemoryEngine {
	m := &MemoryEngine{running: map[string]bool{}}
	for _, name := range running {
		m.running[name] = true
	}
	return m
}

func (m *MemoryEngine) Start(_ context.Context, container string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.startFn != nil {
		if err := m.startFn(container); err != nil {
			return err
		}
	}
	m.starts = append(m.starts, container)
	m.running[container] = true
	return nil
}

func (m *MemoryEngine) Running(_ context.Context, container string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[container], nil
}

func (m *MemoryEngine) ListRunning(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for name, on := range m.running {
		if on {
			out = append(out, name)
		}
	}
	return out, nil
}

func (m *MemoryEngine) Starts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.starts...)
}
