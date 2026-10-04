package world

import (
	"context"
	"sort"

	"github.com/bradfordly/bradfordly-games/internal/gameprofile"
)

// ContainerSpec is the Docker create body for a stopped world.
type ContainerSpec struct {
	Name     string
	Image    string
	Env      []string
	Binds    []string
	Memory   int64
	NanoCPUs int64
	Labels   map[string]string
}

// Container is a subset of docker inspect used by reconcile.
type Container struct {
	Name         string
	Image        string
	Env          []string
	Binds        []string
	Memory       int64
	NanoCPUs     int64
	Running      bool
	PortBindings map[string][]string
}

// Runtime creates, updates, and removes world containers. It never starts them.
type Runtime interface {
	EnsureStopped(ctx context.Context, spec ContainerSpec) error
	Remove(ctx context.Context, name string) error
	Inspect(ctx context.Context, name string) (Container, error)
	RunningCount(ctx context.Context) (int, error)
}

// containerSpec builds the vanilla Minecraft container from a world record.
func containerSpec(w World) (ContainerSpec, error) {
	profile, ok := gameprofile.Lookup(w.Game)
	if !ok {
		return ContainerSpec{}, ErrInvalidGame
	}
	memory, err := profile.MemoryBytes()
	if err != nil {
		return ContainerSpec{}, err
	}
	cpus, err := profile.NanoCPUs()
	if err != nil {
		return ContainerSpec{}, err
	}
	keys := make([]string, 0, len(w.Env))
	for k := range w.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, k+"="+w.Env[k])
	}
	return ContainerSpec{
		Name:     w.Backend.Container,
		Image:    w.Image,
		Env:      env,
		Binds:    []string{w.Volume + ":" + profile.MountPath},
		Memory:   memory,
		NanoCPUs: cpus,
		Labels: map[string]string{
			"bradfordly.world.id":   w.ID,
			"bradfordly.world.game": w.Game,
			"bradfordly.world.host": w.Allocation.Host,
		},
	}, nil
}

func envMap(list []string) map[string]string {
	out := map[string]string{}
	for _, item := range list {
		k, v, ok := splitEnv(item)
		if ok {
			out[k] = v
		}
	}
	return out
}

func splitEnv(item string) (string, string, bool) {
	for i := 0; i < len(item); i++ {
		if item[i] == '=' {
			return item[:i], item[i+1:], true
		}
	}
	return "", "", false
}

func specNeedsRecreate(have Container, want ContainerSpec) bool {
	if have.Image != want.Image || have.Memory != want.Memory || have.NanoCPUs != want.NanoCPUs {
		return true
	}
	if !stringSetsEqual(have.Binds, want.Binds) {
		return true
	}
	return !mapsEqual(envMap(have.Env), envMap(want.Env))
}

func stringSetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	as := append([]string(nil), a...)
	bs := append([]string(nil), b...)
	sort.Strings(as)
	sort.Strings(bs)
	for i := range as {
		if as[i] != bs[i] {
			return false
		}
	}
	return true
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func publishedRCON(c Container) bool {
	for port := range c.PortBindings {
		if port == "25575/tcp" || port == "25575" {
			if len(c.PortBindings[port]) > 0 {
				return true
			}
		}
	}
	return false
}
