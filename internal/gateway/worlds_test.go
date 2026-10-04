package gateway

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

func TestLoadWorlds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "worlds.json")
	raw := `{
  "worlds": [
    {
      "id": "survival",
      "name": "Survival",
      "allocation": {"host": "survival.games.bradfordly.com"},
      "backend": {"container": "survival", "address": "survival:25565"},
      "occupy_mode": "kick",
      "asleep_motd": "asleep",
      "starting_motd": "starting",
      "wake_whitelist": ["Steve"]
    }
  ]
}`
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	worlds, err := LoadWorlds(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 1 {
		t.Fatalf("len = %d", len(worlds))
	}
	w := worlds[0]
	if w.Game != adapter.GameMinecraftJava || w.Allocation.Port != 25565 {
		t.Fatalf("defaults: %#v", w)
	}
	if w.WakeWhitelist[0] != "Steve" || w.OccupyMode != adapter.OccupyModeKick {
		t.Fatalf("settings: %#v", w)
	}
}

func TestLoadWorldsEmptyPath(t *testing.T) {
	worlds, err := LoadWorlds("")
	if err != nil || worlds != nil {
		t.Fatalf("empty path: %v %#v", err, worlds)
	}
}

func TestLoadWorldsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorlds(path); err == nil {
		t.Fatal("expected parse error")
	}
	if _, err := LoadWorlds(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("expected read error")
	}
}
