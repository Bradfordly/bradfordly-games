package gateway

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

func TestLoadWorldsFile(t *testing.T) {
	worlds, err := LoadWorldsFile("")
	if err != nil || worlds != nil {
		t.Fatalf("empty path = %#v, %v", worlds, err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "worlds.json")
	data := `[
	  {"id":"survival","container":"mc-survival","idle_timeout":"30s","host":"survival.example","port":25565},
	  {"id":"creative","game":"minecraft-java"}
	]`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	worlds, err = LoadWorldsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(worlds) != 2 {
		t.Fatalf("len = %d", len(worlds))
	}
	if worlds[0].IdleTimeout != time.Minute {
		t.Fatalf("clamped idle = %s", worlds[0].IdleTimeout)
	}
	if worlds[0].Container != "mc-survival" {
		t.Fatalf("container = %s", worlds[0].Container)
	}
	if worlds[1].Game != adapter.GameMinecraftJava {
		t.Fatalf("default game = %s", worlds[1].Game)
	}
	if worlds[1].StopTimeout != DefaultStopTimeout {
		t.Fatalf("default stop = %s", worlds[1].StopTimeout)
	}
}

func TestLoadWorldsFileErrors(t *testing.T) {
	if _, err := LoadWorldsFile("/no/such/worlds.json"); err == nil {
		t.Fatal("want missing file error")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorldsFile(path); err == nil {
		t.Fatal("want JSON error")
	}
	badIdle := filepath.Join(dir, "idle.json")
	if err := os.WriteFile(badIdle, []byte(`[{"id":"w","idle_timeout":"nope"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorldsFile(badIdle); err == nil {
		t.Fatal("want idle parse error")
	}
	badStop := filepath.Join(dir, "stop.json")
	if err := os.WriteFile(badStop, []byte(`[{"id":"w","stop_timeout":"nope"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorldsFile(badStop); err == nil {
		t.Fatal("want stop parse error")
	}
}
