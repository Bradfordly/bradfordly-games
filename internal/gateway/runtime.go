package gateway

import (
	"context"
	"sync"
	"time"
)

// Runtime starts and stops world containers. This issue only stops.
// It must never stop the EC2 host.
type Runtime interface {
	Running(ctx context.Context, container string) (bool, error)
	Stop(ctx context.Context, container string, timeout time.Duration) (forced bool, err error)
}

// Signal records a stop signal sent to a container.
type Signal struct {
	Container string
	Name      string
}

// FakeRuntime is an in-process Docker stand-in for tests and BDD.
type FakeRuntime struct {
	mu         sync.Mutex
	running    map[string]bool
	ignoreTERM map[string]bool
	signals    []Signal
	hostStops  int
	stopErr    error
	runningErr error
}

// NewFakeRuntime returns a runtime with no running containers.
func NewFakeRuntime() *FakeRuntime {
	return &FakeRuntime{
		running:    map[string]bool{},
		ignoreTERM: map[string]bool{},
	}
}

// SetRunning marks a container as running.
func (f *FakeRuntime) SetRunning(container string, running bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.running[container] = running
}

// IgnoreTERM makes Stop report a forced SIGKILL after SIGTERM.
func (f *FakeRuntime) IgnoreTERM(container string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ignoreTERM[container] = true
	f.running[container] = true
}

// Signals returns a copy of signals sent to containers.
func (f *FakeRuntime) Signals() []Signal {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Signal, len(f.signals))
	copy(out, f.signals)
	return out
}

// HostStops is how many times a caller asked to stop the EC2 instance.
func (f *FakeRuntime) HostStops() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hostStops
}

// StopHost records a forbidden instance stop. Tests assert this stays zero.
func (f *FakeRuntime) StopHost() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hostStops++
}

func (f *FakeRuntime) Running(_ context.Context, container string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.runningErr != nil {
		return false, f.runningErr
	}
	return f.running[container], nil
}

func (f *FakeRuntime) Stop(_ context.Context, container string, _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopErr != nil {
		return false, f.stopErr
	}
	f.signals = append(f.signals, Signal{Container: container, Name: "SIGTERM"})
	if f.ignoreTERM[container] {
		f.signals = append(f.signals, Signal{Container: container, Name: "SIGKILL"})
		f.running[container] = false
		return true, nil
	}
	f.running[container] = false
	return false, nil
}
