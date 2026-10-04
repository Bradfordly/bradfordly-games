package world

import (
	"context"
	"os"
	"sync"
)

// FakeRuntime records stopped containers in memory.
type FakeRuntime struct {
	mu         sync.Mutex
	Containers map[string]Container
	Removed    []string
	FailEnsure bool
	FailRemove bool
}

func NewFakeRuntime() *FakeRuntime {
	return &FakeRuntime{Containers: map[string]Container{}}
}

func (f *FakeRuntime) EnsureStopped(_ context.Context, spec ContainerSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailEnsure {
		return errRunning(spec.Name)
	}
	if have, ok := f.Containers[spec.Name]; ok && have.Running {
		return errRunning(spec.Name)
	}
	f.Containers[spec.Name] = Container{
		Name:         spec.Name,
		Image:        spec.Image,
		Env:          append([]string(nil), spec.Env...),
		Binds:        append([]string(nil), spec.Binds...),
		Memory:       spec.Memory,
		NanoCPUs:     spec.NanoCPUs,
		Running:      false,
		PortBindings: map[string][]string{},
	}
	return nil
}

func (f *FakeRuntime) Remove(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.FailRemove {
		return errRunning(name)
	}
	delete(f.Containers, name)
	f.Removed = append(f.Removed, name)
	return nil
}

func (f *FakeRuntime) Inspect(_ context.Context, name string) (Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.Containers[name]
	if !ok {
		return Container{}, ErrNotExist
	}
	return c, nil
}

func (f *FakeRuntime) RunningCount(context.Context) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.Containers {
		if c.Running {
			n++
		}
	}
	return n, nil
}

func errRunning(name string) error {
	return &runningError{name: name}
}

type runningError struct{ name string }

func (e *runningError) Error() string {
	return "container " + e.name + " is running; drain before replacing"
}

// FakeFiles records save-directory operations.
type FakeFiles struct {
	mu      sync.Mutex
	Dirs    map[string]bool
	Removed []string
}

func NewFakeFiles() *FakeFiles {
	return &FakeFiles{Dirs: map[string]bool{}}
}

func (f *FakeFiles) MkdirAll(path string, _ os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Dirs[path] = true
	return nil
}

func (f *FakeFiles) RemoveAll(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Dirs, path)
	f.Removed = append(f.Removed, path)
	return nil
}

func (f *FakeFiles) Has(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Dirs[path]
}

// FakeGateway records drain and reload calls.
type FakeGateway struct {
	mu       sync.Mutex
	Drains   []string
	Reloads  int
	DrainErr error
}

func (g *FakeGateway) Drain(_ context.Context, worldID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Drains = append(g.Drains, worldID)
	return g.DrainErr
}

func (g *FakeGateway) Reload(context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.Reloads++
	return nil
}
