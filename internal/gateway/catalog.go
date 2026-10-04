package gateway

import (
	"sync"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

type Catalog struct {
	mu     sync.Mutex
	worlds map[string]*adapter.World
}

func NewCatalog(worlds ...*adapter.World) *Catalog {
	c := &Catalog{worlds: map[string]*adapter.World{}}
	for _, w := range worlds {
		c.Put(w)
	}
	return c
}

func (c *Catalog) Put(w *adapter.World) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if w.State == "" {
		w.State = adapter.StateAsleep
	}
	c.worlds[w.ID] = w
}

func (c *Catalog) List() []*adapter.World {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*adapter.World, 0, len(c.worlds))
	for _, w := range c.worlds {
		cp := *w
		out = append(out, &cp)
	}
	return out
}

func (c *Catalog) Get(id string) *adapter.World {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, ok := c.worlds[id]
	if !ok {
		return nil
	}
	cp := *w
	return &cp
}

func (c *Catalog) SetState(id string, state adapter.WorldState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if w, ok := c.worlds[id]; ok {
		w.State = state
	}
}

func (c *Catalog) SetReplicas(id string, n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if w, ok := c.worlds[id]; ok {
		w.Replicas = n
	}
}
