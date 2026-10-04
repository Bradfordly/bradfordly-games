package world

import (
	"path/filepath"
	"testing"
)

func TestStoreInsertGetListUpdateDelete(t *testing.T) {
	s := openTestStore(t)
	w := sampleWorld("alpha", "Survival")

	if err := s.Insert(w); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, err := s.Get(w.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	assertWorldEqual(t, got, w)

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != w.ID {
		t.Fatalf("List = %#v", list)
	}

	w.Name = "Survival Two"
	w.IdleTimeout = "20m0s"
	w.Env["DIFFICULTY"] = "hard"
	if err := s.Update(w); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = s.Get(w.ID)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Name != "Survival Two" || got.IdleTimeout != "20m0s" || got.Env["DIFFICULTY"] != "hard" {
		t.Fatalf("updated world = %#v", got)
	}

	if err := s.Delete(w.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(w.ID); err != ErrNotFound {
		t.Fatalf("Get after delete = %v, want ErrNotFound", err)
	}
	if err := s.Delete(w.ID); err != ErrNotFound {
		t.Fatalf("Delete missing = %v, want ErrNotFound", err)
	}
	if err := s.Update(w); err != ErrNotFound {
		t.Fatalf("Update missing = %v, want ErrNotFound", err)
	}
}

func TestStoreRejectsDuplicateName(t *testing.T) {
	s := openTestStore(t)
	a := sampleWorld("a", "Survival")
	b := sampleWorld("b", "Survival")
	b.Allocation.Host = "other.games.bradfordly.com"
	b.Backend.Container = "world-b"
	b.Volume = "/data/worlds/b"
	if err := s.Insert(a); err != nil {
		t.Fatal(err)
	}
	if err := s.Insert(b); err != ErrConflict {
		t.Fatalf("duplicate name = %v, want ErrConflict", err)
	}
}

func TestStoreListEmpty(t *testing.T) {
	s := openTestStore(t)
	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if list == nil || len(list) != 0 {
		t.Fatalf("empty list = %#v", list)
	}
}

func TestStoreCreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "panel.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
}

func TestStoreCloseNil(t *testing.T) {
	var s *Store
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRejectsCorruptJSON(t *testing.T) {
	s := openTestStore(t)
	w := sampleWorld("alpha", "Survival")
	if err := s.Insert(w); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE worlds SET env='not-json' WHERE id=?`, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(w.ID); err == nil {
		t.Fatal("expected corrupt env")
	}
	if _, err := s.db.Exec(`UPDATE worlds SET env='{}', wake_whitelist='nope' WHERE id=?`, w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(w.ID); err == nil {
		t.Fatal("expected corrupt whitelist")
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func sampleWorld(id, name string) World {
	return World{
		ID:            id,
		Name:          name,
		Game:          "minecraft-java",
		Image:         "itzg/minecraft-server",
		Allocation:    Allocation{Host: slug(name) + ".games.bradfordly.com", Port: 25565, Protocol: "tcp"},
		Backend:       Backend{Container: "world-" + id},
		IdleTimeout:   "15m0s",
		StartTimeout:  "10m0s",
		StopTimeout:   "2m0s",
		OccupyMode:    "kick",
		WakeWhitelist: []string{},
		Env:           map[string]string{"EULA": "TRUE"},
		Volume:        "/data/worlds/" + id,
	}
}

func assertWorldEqual(t *testing.T, got, want World) {
	t.Helper()
	if got.ID != want.ID || got.Name != want.Name || got.Game != want.Game || got.Image != want.Image {
		t.Fatalf("identity got %#v want %#v", got, want)
	}
	if got.Allocation != want.Allocation || got.Backend != want.Backend || got.Volume != want.Volume {
		t.Fatalf("alloc/backend/volume got %#v want %#v", got, want)
	}
	if got.IdleTimeout != want.IdleTimeout || got.OccupyMode != want.OccupyMode {
		t.Fatalf("settings got %#v want %#v", got, want)
	}
	if got.Env["EULA"] != "TRUE" {
		t.Fatalf("env = %#v", got.Env)
	}
	if got.WakeWhitelist == nil {
		t.Fatal("wake_whitelist is nil")
	}
}
