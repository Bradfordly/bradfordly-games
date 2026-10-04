package world

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorldJSONRoundTrip(t *testing.T) {
	in := World{
		APIVersion: APIVersionV1,
		Kind:       Kind,
		Metadata:   Metadata{Name: "survival", Namespace: "worlds"},
		Spec: Spec{
			Name:  "Survival",
			Game:  "minecraft-java",
			Image: "itzg/minecraft-server",
			Allocation: Allocation{
				Host:     "survival.games.bradfordly.com",
				Port:     25565,
				Protocol: "tcp",
			},
			Backend: Backend{
				Service:  "survival",
				Workload: "survival",
			},
			IdleTimeout:   "15m",
			StartTimeout:  "10m",
			StopTimeout:   "2m",
			OccupyMode:    "kick",
			AsleepMOTD:    "asleep — join the game to start",
			StartingMOTD:  "starting",
			WakeWhitelist: []string{"bradfordly"},
			Env:           map[string]string{"EULA": "TRUE"},
			Volume:        "survival-data",
		},
	}

	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out World
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.ID() != "survival" || out.Spec.Game != "minecraft-java" || out.Spec.Volume != "survival-data" {
		t.Fatalf("round trip = %+v", out)
	}
	if strings.Contains(string(raw), "replica") {
		t.Fatal("world JSON must not include replica counts")
	}
}

func TestCRDListsWorldModelFields(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	crd := filepath.Join(filepath.Dir(file), "..", "..", "config", "crd", "worlds.games.bradfordly.com.yaml")
	raw, err := os.ReadFile(crd)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, field := range []string{
		"name", "game", "image",
		"allocation", "host", "port", "protocol",
		"backend", "service", "workload",
		"idle_timeout", "start_timeout", "stop_timeout",
		"occupy_mode", "asleep_motd", "starting_motd",
		"wake_whitelist", "env", "volume",
	} {
		if !strings.Contains(text, field+":") {
			t.Fatalf("CRD missing world model field %q", field)
		}
	}
	if strings.Contains(text, "replicas") {
		t.Fatal("CRD must not define replica counts")
	}
}
