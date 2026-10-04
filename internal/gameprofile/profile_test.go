package gameprofile

import "testing"

func TestMinecraftJavaMatchesSpec(t *testing.T) {
	p, ok := Lookup(MinecraftJava)
	if !ok {
		t.Fatal("minecraft-java profile is missing")
	}

	if p.Game != "minecraft-java" {
		t.Errorf("Game = %q, want %q", p.Game, "minecraft-java")
	}
	if p.Image != "itzg/minecraft-server" {
		t.Errorf("Image = %q, want %q", p.Image, "itzg/minecraft-server")
	}
	if p.CPU != "1" {
		t.Errorf("CPU = %q, want %q", p.CPU, "1")
	}
	if p.Memory != "2Gi" {
		t.Errorf("Memory = %q, want %q", p.Memory, "2Gi")
	}
	if p.Port != 25565 {
		t.Errorf("Port = %d, want %d", p.Port, 25565)
	}
	if p.MountPath != "/data" {
		t.Errorf("MountPath = %q, want %q", p.MountPath, "/data")
	}
	if !hasEnv(p.Env, "EULA", "TRUE") {
		t.Errorf("Env = %#v, want EULA=TRUE", p.Env)
	}
}

func TestFollowOnProfilesAreNotShipped(t *testing.T) {
	for _, game := range []string{"valheim", "palworld"} {
		if _, ok := Lookup(game); ok {
			t.Errorf("unexpected follow-on profile %q", game)
		}
	}
}

func TestMinecraftJavaDockerResources(t *testing.T) {
	p := MinecraftJavaProfile
	mem, err := p.MemoryBytes()
	if err != nil {
		t.Fatal(err)
	}
	if mem != 2<<30 {
		t.Fatalf("MemoryBytes = %d, want 2Gi", mem)
	}
	cpus, err := p.NanoCPUs()
	if err != nil {
		t.Fatal(err)
	}
	if cpus != 1_000_000_000 {
		t.Fatalf("NanoCPUs = %d, want 1 vCPU", cpus)
	}

	bad := Profile{CPU: "x", Memory: "2Mi"}
	if _, err := bad.NanoCPUs(); err == nil {
		t.Fatal("bad cpu")
	}
	if _, err := bad.MemoryBytes(); err == nil {
		t.Fatal("bad memory")
	}
	if _, err := (Profile{Memory: "xGi"}).MemoryBytes(); err == nil {
		t.Fatal("bad Gi")
	}
}

func hasEnv(env []EnvVar, name, value string) bool {
	for _, item := range env {
		if item.Name == name && item.Value == value {
			return true
		}
	}
	return false
}
