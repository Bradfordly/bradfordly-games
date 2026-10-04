package world

import "testing"

func TestApplyDefaults(t *testing.T) {
	r := Record{Name: "Survival"}
	ApplyDefaults(&r)
	if r.ID != "survival" {
		t.Fatalf("id = %q", r.ID)
	}
	if r.Game != GameMinecraftJava || r.Image != DefaultImage {
		t.Fatalf("game/image = %q %q", r.Game, r.Image)
	}
	if r.IdleTimeout != "15m" || r.StartTimeout != "10m" || r.StopTimeout != "2m" {
		t.Fatalf("timeouts = %s %s %s", r.IdleTimeout, r.StartTimeout, r.StopTimeout)
	}
	if r.OccupyMode != DefaultOccupyMode {
		t.Fatalf("occupy = %q", r.OccupyMode)
	}
	if r.Allocation.Port != DefaultMCPort || r.Allocation.Protocol != DefaultProtocol {
		t.Fatalf("allocation = %+v", r.Allocation)
	}
	if r.Backend.Service != "survival" || r.Backend.Workload != "survival" {
		t.Fatalf("backend = %+v", r.Backend)
	}
	if r.Volume != "survival-data" {
		t.Fatalf("volume = %q", r.Volume)
	}
}

func TestValidateIdleTimeoutMin(t *testing.T) {
	r := Record{Name: "Tiny"}
	ApplyDefaults(&r)
	r.IdleTimeout = "30s"
	if err := Validate(r); err == nil {
		t.Fatal("expected idle_timeout min 1m")
	}
	r.IdleTimeout = "1m"
	if err := Validate(r); err != nil {
		t.Fatal(err)
	}
}
