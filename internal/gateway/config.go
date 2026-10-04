package gateway

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/bradfordly/bradfordly-games/internal/adapter"
)

type worldFile struct {
	ID          string `json:"id"`
	Game        string `json:"game"`
	Container   string `json:"container"`
	IdleTimeout string `json:"idle_timeout"`
	StopTimeout string `json:"stop_timeout"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
}

// LoadWorldsFile reads world specs for the gateway. Missing path is empty.
func LoadWorldsFile(path string) ([]adapter.World, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("worlds file: %w", err)
	}
	var rows []worldFile
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("worlds file: %w", err)
	}
	out := make([]adapter.World, 0, len(rows))
	for _, row := range rows {
		idle, err := ParseIdleTimeout(row.IdleTimeout)
		if err != nil {
			return nil, fmt.Errorf("world %s idle_timeout: %w", row.ID, err)
		}
		stop, err := ParseStopTimeout(row.StopTimeout)
		if err != nil {
			return nil, fmt.Errorf("world %s stop_timeout: %w", row.ID, err)
		}
		game := row.Game
		if game == "" {
			game = adapter.GameMinecraftJava
		}
		out = append(out, adapter.World{
			ID:          row.ID,
			Game:        game,
			Container:   row.Container,
			IdleTimeout: idle,
			StopTimeout: stop,
			Allocation: adapter.Allocation{
				Host:     row.Host,
				Port:     row.Port,
				Protocol: "tcp",
			},
		})
	}
	return out, nil
}
