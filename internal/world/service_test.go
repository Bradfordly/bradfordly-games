package world

import (
	"context"
	"errors"
	"testing"
)

func TestServiceCreateWritesRowDirAndStoppedContainer(t *testing.T) {
	svc, rt, files, gw := testService(t)
	w, err := svc.Create(context.Background(), CreateRequest{Name: "Survival"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Survival" || got.Game != "minecraft-java" {
		t.Fatalf("stored = %#v", got)
	}
	if !files.Has(w.Volume) {
		t.Fatalf("missing save dir %s", w.Volume)
	}
	c, err := rt.Inspect(context.Background(), w.Backend.Container)
	if err != nil {
		t.Fatal(err)
	}
	if c.Running {
		t.Fatal("container should be stopped")
	}
	if c.Image != "itzg/minecraft-server" {
		t.Fatalf("image = %q", c.Image)
	}
	if envMap(c.Env)["EULA"] != "TRUE" {
		t.Fatalf("env = %#v", c.Env)
	}
	if publishedRCON(c) {
		t.Fatal("RCON published")
	}
	if gw.Reloads != 1 {
		t.Fatalf("reloads = %d", gw.Reloads)
	}
	list, err := svc.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %#v err=%v", list, err)
	}
}

func TestServiceCreateContainerError(t *testing.T) {
	svc, rt, _, _ := testService(t)
	rt.FailEnsure = true
	if _, err := svc.Create(context.Background(), CreateRequest{Name: "Survival"}); err == nil {
		t.Fatal("expected container error")
	}
}

func TestServiceCreateRejectsOtherGames(t *testing.T) {
	svc, _, _, _ := testService(t)
	_, err := svc.Create(context.Background(), CreateRequest{Name: "Mead", Game: "valheim"})
	if err != ErrInvalidGame {
		t.Fatalf("err = %v", err)
	}
}

func TestServicePatchUpdatesEnvWithoutRecreatingSaveDir(t *testing.T) {
	svc, rt, files, _ := testService(t)
	w, err := svc.Create(context.Background(), CreateRequest{Name: "Survival"})
	if err != nil {
		t.Fatal(err)
	}
	idle := "30m"
	got, err := svc.Update(context.Background(), w.ID, UpdateRequest{
		IdleTimeout: &idle,
		Env:         map[string]string{"DIFFICULTY": "hard"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Volume != w.Volume {
		t.Fatalf("volume %q -> %q", w.Volume, got.Volume)
	}
	if !files.Has(w.Volume) {
		t.Fatal("save dir missing after patch")
	}
	if len(files.Removed) != 0 {
		t.Fatalf("save dir recreated: %#v", files.Removed)
	}
	if got.IdleTimeout != "30m0s" {
		t.Fatalf("idle = %q", got.IdleTimeout)
	}
	c, err := rt.Inspect(context.Background(), w.Backend.Container)
	if err != nil {
		t.Fatal(err)
	}
	if envMap(c.Env)["DIFFICULTY"] != "hard" || envMap(c.Env)["EULA"] != "TRUE" {
		t.Fatalf("container env = %#v", c.Env)
	}
}

func TestServiceDeleteDrainsRemovesContainerKeepsSave(t *testing.T) {
	svc, rt, files, gw := testService(t)
	w, err := svc.Create(context.Background(), CreateRequest{Name: "Survival"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(context.Background(), w.ID, false); err != nil {
		t.Fatal(err)
	}
	if len(gw.Drains) != 1 || gw.Drains[0] != w.ID {
		t.Fatalf("drains = %#v", gw.Drains)
	}
	if _, err := rt.Inspect(context.Background(), w.Backend.Container); !errors.Is(err, ErrNotExist) {
		t.Fatalf("container still present: %v", err)
	}
	if !files.Has(w.Volume) {
		t.Fatal("save dir should remain")
	}
	if _, err := svc.Get(w.ID); err != ErrNotFound {
		t.Fatalf("row still present: %v", err)
	}
}

func TestServiceDeleteDestroyRemovesSave(t *testing.T) {
	svc, _, files, _ := testService(t)
	w, err := svc.Create(context.Background(), CreateRequest{Name: "Survival"})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(context.Background(), w.ID, true); err != nil {
		t.Fatal(err)
	}
	if files.Has(w.Volume) {
		t.Fatal("save dir should be gone")
	}
}

func TestServiceDeleteFailsWhenDrainFails(t *testing.T) {
	svc, rt, _, gw := testService(t)
	w, err := svc.Create(context.Background(), CreateRequest{Name: "Survival"})
	if err != nil {
		t.Fatal(err)
	}
	gw.DrainErr = errors.New("busy")
	if err := svc.Delete(context.Background(), w.ID, false); err == nil {
		t.Fatal("expected drain error")
	}
	if _, err := rt.Inspect(context.Background(), w.Backend.Container); err != nil {
		t.Fatal("container should remain when drain fails")
	}
}

func TestServiceCreateRollsBackContainerOnConflict(t *testing.T) {
	svc, rt, _, _ := testService(t)
	n := 0
	svc.newID = func() (string, error) {
		n++
		if n == 1 {
			return "one", nil
		}
		return "two", nil
	}
	first, err := svc.Create(context.Background(), CreateRequest{Name: "Survival"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Create(context.Background(), CreateRequest{Name: "Survival"})
	if err != ErrConflict {
		t.Fatalf("second create = %v, want name conflict", err)
	}
	if _, err := rt.Inspect(context.Background(), first.Backend.Container); err != nil {
		t.Fatal("first container should remain")
	}
	if _, err := rt.Inspect(context.Background(), "world-two"); !errors.Is(err, ErrNotExist) {
		t.Fatalf("rolled-back container still present: %v", err)
	}
}

func TestRandomID(t *testing.T) {
	a, err := randomID()
	if err != nil || len(a) != 16 {
		t.Fatalf("id = %q err=%v", a, err)
	}
	b, err := randomID()
	if err != nil || a == b {
		t.Fatalf("ids should differ: %q %q err=%v", a, b, err)
	}
}

func TestFakeRuntimeRunningCount(t *testing.T) {
	rt := NewFakeRuntime()
	if n, err := rt.RunningCount(context.Background()); err != nil || n != 0 {
		t.Fatalf("empty running = %d %v", n, err)
	}
	rt.Containers["w"] = Container{Running: true}
	if n, err := rt.RunningCount(context.Background()); err != nil || n != 1 {
		t.Fatalf("running = %d %v", n, err)
	}
}

func TestServiceCreateIDError(t *testing.T) {
	svc, _, _, _ := testService(t)
	svc.newID = func() (string, error) { return "", errors.New("rng") }
	if _, err := svc.Create(context.Background(), CreateRequest{Name: "Survival"}); err == nil {
		t.Fatal("expected id error")
	}
}

func TestServiceUpdateMissing(t *testing.T) {
	svc, _, _, _ := testService(t)
	if _, err := svc.Update(context.Background(), "missing", UpdateRequest{}); err != ErrNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestServiceUpdateAndDeleteRuntimeErrors(t *testing.T) {
	svc, rt, _, _ := testService(t)
	w, err := svc.Create(context.Background(), CreateRequest{Name: "Survival"})
	if err != nil {
		t.Fatal(err)
	}
	rt.FailEnsure = true
	idle := "20m"
	if _, err := svc.Update(context.Background(), w.ID, UpdateRequest{IdleTimeout: &idle}); err == nil {
		t.Fatal("expected update container error")
	}
	rt.FailEnsure = false
	rt.FailRemove = true
	if err := svc.Delete(context.Background(), w.ID, false); err == nil {
		t.Fatal("expected delete container error")
	}
	if err := svc.Delete(context.Background(), "missing", false); err != ErrNotFound {
		t.Fatalf("missing delete = %v", err)
	}
}

func testService(t *testing.T) (*Service, *FakeRuntime, *FakeFiles, *FakeGateway) {
	t.Helper()
	store := openTestStore(t)
	rt := NewFakeRuntime()
	files := NewFakeFiles()
	gw := &FakeGateway{}
	svc := NewService(store, rt, files, gw, t.TempDir(), DefaultDomain)
	svc.newID = func() (string, error) { return "fixedid1", nil }
	return svc, rt, files, gw
}
