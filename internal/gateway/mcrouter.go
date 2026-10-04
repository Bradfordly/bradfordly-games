package gateway

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

// RouterConfig is the static route file mc-router reloads for hostname mapping.
type RouterConfig struct {
	Mappings map[string]string `json:"mappings"`
}

type allowDenyFile struct {
	Servers map[string]allowDenyServer `json:"servers"`
}

type allowDenyServer struct {
	Allowlist []allowDenyPlayer `json:"allowlist,omitempty"`
}

type allowDenyPlayer struct {
	Name string `json:"name"`
}

// WriteRouterFiles writes mc-router routes and wake_whitelist allow lists.
func WriteRouterFiles(dir string, worlds []*adapter.World) (routesPath, allowPath string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	routes := RouterConfig{Mappings: map[string]string{}}
	allow := allowDenyFile{Servers: map[string]allowDenyServer{}}
	for _, w := range worlds {
		if w == nil || w.Allocation.Host == "" {
			continue
		}
		backend := w.Backend.Address
		if backend == "" && w.Backend.Container != "" {
			backend = w.Backend.Container + ":25565"
		}
		if backend != "" {
			routes.Mappings[w.Allocation.Host] = backend
		}
		if len(w.WakeWhitelist) > 0 {
			players := make([]allowDenyPlayer, 0, len(w.WakeWhitelist))
			for _, name := range w.WakeWhitelist {
				players = append(players, allowDenyPlayer{Name: name})
			}
			allow.Servers[w.Allocation.Host] = allowDenyServer{Allowlist: players}
		}
	}
	routesPath = filepath.Join(dir, "routes.json")
	allowPath = filepath.Join(dir, "allow-deny.json")
	if err := writeJSON(routesPath, routes); err != nil {
		return "", "", err
	}
	if err := writeJSON(allowPath, allow); err != nil {
		return "", "", err
	}
	return routesPath, allowPath, nil
}

func writeJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

// RouterCommand starts itzg/mc-router. Handshake parsing stays in that binary.
// Docker start stays in this process so the socket is never published on 25565.
func RouterCommand(bin, playerPort, routesPath, allowPath, webhookURL, asleepMOTD, startingMOTD string, wakeTimeout time.Duration) *exec.Cmd {
	if bin == "" {
		bin = "mc-router"
	}
	if playerPort == "" {
		playerPort = "25565"
	}
	if wakeTimeout <= 0 {
		wakeTimeout = DefaultHoldWindow
	}
	args := []string{
		"-port", playerPort,
		"-routes-config", routesPath,
		"-auto-scale-webhook-url", webhookURL,
		"-auto-scale-webhook-wake-timeout", wakeTimeout.String(),
		"-auto-scale-asleep-motd", asleepMOTD,
		"-auto-scale-loading-motd", startingMOTD,
	}
	if allowPath != "" {
		args = append(args, "-auto-scale-allow-deny", allowPath)
	}
	return exec.Command(bin, args...)
}

func firstMOTD(worlds []*adapter.World, starting bool) string {
	for _, w := range worlds {
		if w == nil {
			continue
		}
		if starting && w.StartingMOTD != "" {
			return w.StartingMOTD
		}
		if !starting && w.AsleepMOTD != "" {
			return w.AsleepMOTD
		}
	}
	if starting {
		return adapter.DefaultStartingMOTD
	}
	return "World is asleep. Join to start."
}

// WebhookURL is the localhost scaler mc-router calls on login.
func WebhookURL(adminAddr string) (string, error) {
	if !AdminIsLocalhost(adminAddr) {
		return "", fmt.Errorf("admin address %q must be localhost", adminAddr)
	}
	_, port, err := net.SplitHostPort(adminAddr)
	if err != nil {
		return "", err
	}
	return "http://127.0.0.1:" + port + "/internal/scale", nil
}

// DefaultMOTDs picks asleep and starting text from the first world that has them.
func DefaultMOTDs(worlds []*adapter.World) (asleep, starting string) {
	return firstMOTD(worlds, false), firstMOTD(worlds, true)
}
