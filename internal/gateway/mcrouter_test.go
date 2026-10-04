package gateway

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

func TestWriteRouterFiles(t *testing.T) {
	dir := t.TempDir()
	world := survival()
	world.WakeWhitelist = []string{"Steve"}
	routesPath, allowPath, err := WriteRouterFiles(dir, []*adapter.World{world})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(routesPath)
	if err != nil {
		t.Fatal(err)
	}
	var routes RouterConfig
	if err := json.Unmarshal(raw, &routes); err != nil {
		t.Fatal(err)
	}
	if routes.Mappings["survival.games.bradfordly.com"] != "survival:25565" {
		t.Fatalf("mappings = %#v", routes.Mappings)
	}
	allowRaw, err := os.ReadFile(allowPath)
	if err != nil {
		t.Fatal(err)
	}
	var allow allowDenyFile
	if err := json.Unmarshal(allowRaw, &allow); err != nil {
		t.Fatal(err)
	}
	if len(allow.Servers["survival.games.bradfordly.com"].Allowlist) != 1 {
		t.Fatalf("allow = %#v", allow)
	}
	containerOnly := &adapter.World{
		ID:         "creative",
		Allocation: adapter.Allocation{Host: "creative.example"},
		Backend:    adapter.Backend{Container: "creative"},
	}
	if _, _, err := WriteRouterFiles(dir, []*adapter.World{nil, containerOnly, {ID: "nohost"}}); err != nil {
		t.Fatal(err)
	}
}

func TestRouterCommand(t *testing.T) {
	defaults := RouterCommand("", "", "/tmp/routes.json", "", "http://127.0.0.1:8080/internal/scale", "asleep", "starting", 0)
	if defaults.Path != "mc-router" && defaults.Args[0] != "mc-router" {
		t.Fatalf("default bin = %v", defaults.Args)
	}
	cmd := RouterCommand("/bin/mc-router", "25565", "/tmp/routes.json", "/tmp/allow.json", "http://127.0.0.1:8080/internal/scale", "asleep", "starting", 0)
	if cmd.Path != "/bin/mc-router" {
		t.Fatalf("path = %s", cmd.Path)
	}
	joined := cmd.Args
	has := func(flag, value string) {
		t.Helper()
		for i, a := range joined {
			if a == flag && i+1 < len(joined) && joined[i+1] == value {
				return
			}
		}
		t.Fatalf("missing %s %s in %v", flag, value, joined)
	}
	has("-port", "25565")
	has("-auto-scale-webhook-url", "http://127.0.0.1:8080/internal/scale")
	has("-auto-scale-asleep-motd", "asleep")
	has("-auto-scale-loading-motd", "starting")
	for _, a := range joined {
		if a == "-auto-scale-down" || a == "-in-docker" {
			t.Fatalf("must not pass %s: docker start is ours, idle stop is #62", a)
		}
	}
}

func TestWebhookURL(t *testing.T) {
	got, err := WebhookURL("127.0.0.1:9090")
	if err != nil {
		t.Fatal(err)
	}
	if got != "http://127.0.0.1:9090/internal/scale" {
		t.Fatalf("got %s", got)
	}
	if _, err := WebhookURL(":8080"); err == nil {
		t.Fatal("wildcard admin must fail")
	}
}

func TestDefaultMOTDs(t *testing.T) {
	asleep, starting := DefaultMOTDs([]*adapter.World{{AsleepMOTD: "zzz", StartingMOTD: "boot"}})
	if asleep != "zzz" || starting != "boot" {
		t.Fatalf("%q %q", asleep, starting)
	}
	asleep, starting = DefaultMOTDs(nil)
	if starting != adapter.DefaultStartingMOTD {
		t.Fatalf("starting default = %q", starting)
	}
	if asleep == "" {
		t.Fatal("asleep default")
	}
}
