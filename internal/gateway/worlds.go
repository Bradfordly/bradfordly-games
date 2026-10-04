package gateway

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

type worldsFile struct {
	Worlds []worldFile `json:"worlds"`
}

type worldFile struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Game          string         `json:"game"`
	Allocation    allocationFile `json:"allocation"`
	Backend       backendFile    `json:"backend"`
	OccupyMode    string         `json:"occupy_mode"`
	AsleepMOTD    string         `json:"asleep_motd"`
	StartingMOTD  string         `json:"starting_motd"`
	WakeWhitelist []string       `json:"wake_whitelist"`
}

type allocationFile struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type backendFile struct {
	Container string `json:"container"`
	Address   string `json:"address"`
}

// LoadWorlds reads the on-disk world store the control plane will own later.
func LoadWorlds(path string) ([]*adapter.World, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read worlds: %w", err)
	}
	var file worldsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parse worlds: %w", err)
	}
	out := make([]*adapter.World, 0, len(file.Worlds))
	for _, w := range file.Worlds {
		game := w.Game
		if game == "" {
			game = adapter.GameMinecraftJava
		}
		port := w.Allocation.Port
		if port == 0 {
			port = 25565
		}
		proto := w.Allocation.Protocol
		if proto == "" {
			proto = "tcp"
		}
		out = append(out, &adapter.World{
			ID:   w.ID,
			Name: w.Name,
			Game: game,
			Allocation: adapter.Allocation{
				Host:     w.Allocation.Host,
				Port:     port,
				Protocol: proto,
			},
			Backend: adapter.Backend{
				Container: w.Backend.Container,
				Address:   w.Backend.Address,
			},
			OccupyMode:    adapter.OccupyMode(w.OccupyMode),
			AsleepMOTD:    w.AsleepMOTD,
			StartingMOTD:  w.StartingMOTD,
			WakeWhitelist: w.WakeWhitelist,
		})
	}
	return out, nil
}
