package panel

import (
	"context"
	"testing"
	"time"
)

func TestMemoryCatalog(t *testing.T) {
	c := NewMemoryCatalog()
	if len(c.List()) != 0 {
		t.Fatal("empty catalog")
	}
	c.Reset(World{ID: "survival", Name: "Survival", Game: "minecraft-java", Allocation: Allocation{Host: "survival.games.bradfordly.com", Port: 25565, Protocol: "tcp"}})
	if got := c.List()[0].Allocation.String(); got != "survival.games.bradfordly.com:25565" {
		t.Fatalf("allocation = %s", got)
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("missing world")
	}
	if w, ok := c.Get("survival"); !ok || w.Name != "Survival" {
		t.Fatalf("get = %#v ok=%v", w, ok)
	}
}

func TestMergeView(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	idle := now.Add(90 * time.Second)
	wake := now.Add(-time.Hour)
	forced := now.Add(-2 * time.Hour)
	players := 3
	world := World{ID: "survival", Name: "Survival", Allocation: Allocation{Host: "h", Port: 25565}}
	v := mergeView(world, Snapshot{
		State:              "idle_wait",
		Players:            &players,
		IdleUntil:          &idle,
		LastWakeCause:      "manual",
		LastWakeAt:         &wake,
		LastForcedStopAt:   &forced,
		HoursOnlineLastDay: 4.5,
		LastError:          "",
		ContainerRunning:   true,
	}, now)
	if v.AllocationString != "h:25565" || v.IdleTimer != "1m30s" || !v.ConfirmStop || v.AsleepHint {
		t.Fatalf("view = %#v", v)
	}
	asleep := mergeView(world, Snapshot{}, now)
	if !asleep.AsleepHint || asleep.State != "asleep" || asleep.ConfirmStop {
		t.Fatalf("default asleep = %#v", asleep)
	}
	past := now.Add(-time.Second)
	idlePast := mergeView(world, Snapshot{State: "idle_wait", IdleUntil: &past}, now)
	if idlePast.IdleTimer != "0s" {
		t.Fatalf("past idle = %q", idlePast.IdleTimer)
	}
}

func TestServerViews(t *testing.T) {
	env := newTestEnv(t)
	env.catalog.Reset(World{ID: "survival", Name: "Survival", Game: "minecraft-java"})
	env.gateway.Set(Snapshot{ID: "survival", State: "failed", LastError: "wedged"})
	views, err := env.server.views(context.Background())
	if err != nil || len(views) != 1 || views[0].State != "failed" {
		t.Fatalf("views = %#v err=%v", views, err)
	}
	got, ok, err := env.server.view(context.Background(), "survival")
	if err != nil || !ok || got.LastError != "wedged" {
		t.Fatalf("view = %#v ok=%v err=%v", got, ok, err)
	}
	if _, ok, err := env.server.view(context.Background(), "nope"); err != nil || ok {
		t.Fatal("missing world")
	}
}

func TestAllocationEmptyString(t *testing.T) {
	if (Allocation{}).String() != "" {
		t.Fatal("empty allocation")
	}
}
