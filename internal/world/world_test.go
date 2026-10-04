package world

import (
	"testing"

	"github.com/bradfordly/bradfordly-games/internal/gameprofile"
)

func TestNormalizeCreateDefaults(t *testing.T) {
	w, err := normalizeCreate(CreateRequest{Name: "Survival"}, "abc", "/data", "")
	if err != nil {
		t.Fatal(err)
	}
	if w.Game != gameprofile.MinecraftJava {
		t.Fatalf("game = %q", w.Game)
	}
	if w.Image != "itzg/minecraft-server" {
		t.Fatalf("image = %q", w.Image)
	}
	if w.Allocation.Host != "survival.games.bradfordly.com" {
		t.Fatalf("host = %q", w.Allocation.Host)
	}
	if w.Allocation.Port != 25565 || w.Allocation.Protocol != "tcp" {
		t.Fatalf("allocation = %#v", w.Allocation)
	}
	if w.Backend.Container != "world-abc" {
		t.Fatalf("container = %q", w.Backend.Container)
	}
	if w.Volume != "/data/worlds/abc" {
		t.Fatalf("volume = %q", w.Volume)
	}
	if w.IdleTimeout != "15m0s" || w.StartTimeout != "10m0s" || w.StopTimeout != "2m0s" {
		t.Fatalf("timeouts = %#v", w)
	}
	if w.OccupyMode != "kick" {
		t.Fatalf("occupy = %q", w.OccupyMode)
	}
	if w.Env["EULA"] != "TRUE" {
		t.Fatalf("env = %#v", w.Env)
	}
}

func TestNormalizeCreateRejects(t *testing.T) {
	if _, err := normalizeCreate(CreateRequest{}, "id", "/data", ""); err != ErrNameRequired {
		t.Fatalf("empty name = %v", err)
	}
	if _, err := normalizeCreate(CreateRequest{Name: "x", Game: "valheim"}, "id", "/data", ""); err != ErrInvalidGame {
		t.Fatalf("valheim = %v", err)
	}
	if _, err := normalizeCreate(CreateRequest{Name: "x", IdleTimeout: "30s"}, "id", "/data", ""); err != ErrIdleTimeout {
		t.Fatalf("short idle = %v", err)
	}
	if _, err := normalizeCreate(CreateRequest{Name: "x", IdleTimeout: "nope"}, "id", "/data", ""); err == nil {
		t.Fatal("bad duration")
	}
	if _, err := normalizeCreate(CreateRequest{Name: "x", OccupyMode: "teleport"}, "id", "/data", ""); err == nil {
		t.Fatal("bad occupy")
	}
}

func TestNormalizeCreateMergesEnvAndCustomHost(t *testing.T) {
	w, err := normalizeCreate(CreateRequest{
		Name: "Creative",
		Env:  map[string]string{"DIFFICULTY": "hard", "EULA": "false"},
		Host: "play.example.com",
	}, "id", "/mnt/data", "ignored.example")
	if err != nil {
		t.Fatal(err)
	}
	if w.Env["EULA"] != "TRUE" || w.Env["DIFFICULTY"] != "hard" {
		t.Fatalf("env = %#v", w.Env)
	}
	if w.Allocation.Host != "play.example.com" {
		t.Fatalf("host = %q", w.Allocation.Host)
	}
	if w.Volume != "/mnt/data/worlds/id" {
		t.Fatalf("volume = %q", w.Volume)
	}
}

func TestSlug(t *testing.T) {
	if got := slug("My World!"); got != "my-world" {
		t.Fatalf("slug = %q", got)
	}
	if got := slug("   "); got != "world" {
		t.Fatalf("empty slug = %q", got)
	}
}

func TestApplyUpdateKeepsVolumeAndForcesEULA(t *testing.T) {
	cur := sampleWorld("id", "Survival")
	name := "Renamed"
	idle := "20m"
	image := "itzg/minecraft-server:java21"
	occupy := "hold"
	asleep := "Asleep"
	starting := "Starting"
	env := map[string]string{"DIFFICULTY": "peaceful", "EULA": "no"}
	got, err := applyUpdate(cur, UpdateRequest{
		Name:          &name,
		Image:         &image,
		IdleTimeout:   &idle,
		OccupyMode:    &occupy,
		AsleepMOTD:    &asleep,
		StartingMOTD:  &starting,
		WakeWhitelist: []string{"Steve"},
		Env:           env,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Volume != cur.Volume {
		t.Fatalf("volume changed to %q", got.Volume)
	}
	if got.Name != "Renamed" || got.IdleTimeout != "20m0s" || got.OccupyMode != "hold" {
		t.Fatalf("updated = %#v", got)
	}
	if got.Env["EULA"] != "TRUE" || got.Env["DIFFICULTY"] != "peaceful" {
		t.Fatalf("env = %#v", got.Env)
	}
	if got.AsleepMOTD != "Asleep" || got.StartingMOTD != "Starting" || got.WakeWhitelist[0] != "Steve" {
		t.Fatalf("motd/whitelist = %#v", got)
	}
}

func TestApplyUpdateRejects(t *testing.T) {
	cur := sampleWorld("id", "Survival")
	empty := "  "
	if _, err := applyUpdate(cur, UpdateRequest{Name: &empty}); err != ErrNameRequired {
		t.Fatalf("empty name = %v", err)
	}
	short := "10s"
	if _, err := applyUpdate(cur, UpdateRequest{IdleTimeout: &short}); err != ErrIdleTimeout {
		t.Fatalf("short idle = %v", err)
	}
	bad := "teleport"
	if _, err := applyUpdate(cur, UpdateRequest{OccupyMode: &bad}); err == nil {
		t.Fatal("bad occupy")
	}
	blankImage := ""
	if _, err := applyUpdate(cur, UpdateRequest{Image: &blankImage}); err == nil {
		t.Fatal("blank image")
	}
	badDur := "nope"
	if _, err := applyUpdate(cur, UpdateRequest{StartTimeout: &badDur}); err == nil {
		t.Fatal("bad start")
	}
	if _, err := applyUpdate(cur, UpdateRequest{StopTimeout: &badDur}); err == nil {
		t.Fatal("bad stop")
	}
}

func TestContainerSpecMinecraftJava(t *testing.T) {
	w := sampleWorld("id", "Survival")
	w.Env["DIFFICULTY"] = "hard"
	spec, err := containerSpec(w)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Image != "itzg/minecraft-server" {
		t.Fatalf("image = %q", spec.Image)
	}
	if spec.Memory != 2<<30 || spec.NanoCPUs != 1_000_000_000 {
		t.Fatalf("resources mem=%d cpus=%d", spec.Memory, spec.NanoCPUs)
	}
	if len(spec.Binds) != 1 || spec.Binds[0] != "/data/worlds/id:/data" {
		t.Fatalf("binds = %#v", spec.Binds)
	}
	env := envMap(spec.Env)
	if env["EULA"] != "TRUE" || env["DIFFICULTY"] != "hard" {
		t.Fatalf("env = %#v", env)
	}
	if spec.Labels["bradfordly.world.host"] != w.Allocation.Host {
		t.Fatalf("labels = %#v", spec.Labels)
	}
}

func TestIdleDurationAndUpdateTimeouts(t *testing.T) {
	w := sampleWorld("id", "Survival")
	d, err := w.IdleDuration()
	if err != nil || d != DefaultIdleTimeout {
		t.Fatalf("IdleDuration = %v %v", d, err)
	}
	start := "12m"
	stop := "3m"
	got, err := applyUpdate(w, UpdateRequest{StartTimeout: &start, StopTimeout: &stop})
	if err != nil {
		t.Fatal(err)
	}
	if got.StartTimeout != "12m0s" || got.StopTimeout != "3m0s" {
		t.Fatalf("timeouts = %#v", got)
	}
}

func TestContainerSpecRejectsUnknownGame(t *testing.T) {
	w := sampleWorld("id", "Survival")
	w.Game = "valheim"
	if _, err := containerSpec(w); err != ErrInvalidGame {
		t.Fatalf("err = %v", err)
	}
}

func TestSpecNeedsRecreate(t *testing.T) {
	spec, err := containerSpec(sampleWorld("id", "Survival"))
	if err != nil {
		t.Fatal(err)
	}
	have := Container{Image: spec.Image, Env: spec.Env, Binds: spec.Binds, Memory: spec.Memory, NanoCPUs: spec.NanoCPUs}
	if specNeedsRecreate(have, spec) {
		t.Fatal("identical spec")
	}
	have.Image = "other"
	if !specNeedsRecreate(have, spec) {
		t.Fatal("image change")
	}
}

func TestSplitEnvAndSets(t *testing.T) {
	if _, _, ok := splitEnv("noseparator"); ok {
		t.Fatal("expected false")
	}
	if stringSetsEqual([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("length")
	}
	if mapsEqual(map[string]string{"a": "1"}, map[string]string{"a": "2"}) {
		t.Fatal("maps")
	}
}

func TestNewServiceDefaults(t *testing.T) {
	s := NewService(nil, nil, nil, nil, "/data", "")
	if s.Domain != DefaultDomain {
		t.Fatalf("domain = %q", s.Domain)
	}
	if s.Files == nil {
		t.Fatal("files")
	}
}

func TestPublishedRCON(t *testing.T) {
	if publishedRCON(Container{PortBindings: map[string][]string{}}) {
		t.Fatal("empty bindings")
	}
	if !publishedRCON(Container{PortBindings: map[string][]string{"25575/tcp": {"25575"}}}) {
		t.Fatal("expected RCON published")
	}
}
