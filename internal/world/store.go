package world

import (
	"context"
	"fmt"
	"sync"
)

// ErrNotFound is a missing world id.
var ErrNotFound = fmt.Errorf("world not found")

// Store persists World custom resources. Implementations must not write replica counts.
type Store interface {
	List(ctx context.Context) ([]World, error)
	Get(ctx context.Context, id string) (World, error)
	Create(ctx context.Context, w World) (World, error)
	Update(ctx context.Context, w World) (World, error)
}

// MemoryStore is an in-process map of World objects.
type MemoryStore struct {
	mu     sync.Mutex
	worlds map[string]World
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{worlds: map[string]World{}}
}

func (s *MemoryStore) List(_ context.Context) ([]World, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]World, 0, len(s.worlds))
	for _, w := range s.worlds {
		out = append(out, w)
	}
	return out, nil
}

func (s *MemoryStore) Get(_ context.Context, id string) (World, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.worlds[id]
	if !ok {
		return World{}, ErrNotFound
	}
	return w, nil
}

func (s *MemoryStore) Create(_ context.Context, w World) (World, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w.Metadata.Name == "" {
		return World{}, fmt.Errorf("world id is required")
	}
	if _, exists := s.worlds[w.Metadata.Name]; exists {
		return World{}, fmt.Errorf("world %s exists", w.Metadata.Name)
	}
	s.worlds[w.Metadata.Name] = w
	return w, nil
}

func (s *MemoryStore) Update(_ context.Context, w World) (World, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.worlds[w.Metadata.Name]; !ok {
		return World{}, ErrNotFound
	}
	s.worlds[w.Metadata.Name] = w
	return w, nil
}
