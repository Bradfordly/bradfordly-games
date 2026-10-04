// Package gameprofile holds reusable defaults the control plane applies
// when it reconciles a world's workload.
package gameprofile

// MinecraftJava is the v1 game value from docs/specs/game-adapters.md.
const MinecraftJava = "minecraft-java"

// EnvVar is a container environment variable.
type EnvVar struct {
	Name  string
	Value string
}

// Profile is the starting point for a world's container spec.
// Operators can raise CPU and memory later.
type Profile struct {
	Game      string
	Image     string
	CPU       string
	Memory    string
	Env       []EnvVar
	Port      int32
	MountPath string
}

// MinecraftJavaProfile is the vanilla EKS Fargate starting point.
// CPU and memory are Kubernetes quantities (1 vCPU / 2 GiB).
var MinecraftJavaProfile = Profile{
	Game:      MinecraftJava,
	Image:     "itzg/minecraft-server",
	CPU:       "1",
	Memory:    "2Gi",
	Env:       []EnvVar{{Name: "EULA", Value: "TRUE"}},
	Port:      25565,
	MountPath: "/data",
}

var byGame = map[string]Profile{
	MinecraftJava: MinecraftJavaProfile,
}

// Lookup returns the shipped profile for a game value.
func Lookup(game string) (Profile, bool) {
	p, ok := byGame[game]
	return p, ok
}
