package adapter

// Registry maps a world game value to an Adapter.
type Registry struct {
	byGame map[string]Adapter
}

// NewRegistry registers the Minecraft Java stub and not-implemented UDP games.
func NewRegistry() *Registry {
	r := &Registry{byGame: map[string]Adapter{}}
	r.Register(MinecraftJava())
	r.Register(Unimplemented(GameValheim))
	r.Register(Unimplemented(GamePalworld))
	return r
}

func (r *Registry) Register(a Adapter) {
	r.byGame[a.Game()] = a
}

func (r *Registry) ForGame(game string) (Adapter, bool) {
	a, ok := r.byGame[game]
	return a, ok
}
