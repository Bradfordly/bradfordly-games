package world

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	GameMinecraftJava   = "minecraft-java"
	DefaultImage        = "itzg/minecraft-server"
	DefaultIdleTimeout  = 15 * time.Minute
	MinIdleTimeout      = time.Minute
	DefaultStartTimeout = 10 * time.Minute
	DefaultStopTimeout  = 2 * time.Minute
	DefaultOccupyMode   = "kick"
	DefaultMCPort       = 25565
	DefaultProtocol     = "tcp"
)

var idPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Record is the API view of a World: stable id plus spec fields.
type Record struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Game          string            `json:"game"`
	Image         string            `json:"image,omitempty"`
	Allocation    Allocation        `json:"allocation"`
	Backend       Backend           `json:"backend"`
	IdleTimeout   string            `json:"idle_timeout,omitempty"`
	StartTimeout  string            `json:"start_timeout,omitempty"`
	StopTimeout   string            `json:"stop_timeout,omitempty"`
	OccupyMode    string            `json:"occupy_mode,omitempty"`
	AsleepMOTD    string            `json:"asleep_motd,omitempty"`
	StartingMOTD  string            `json:"starting_motd,omitempty"`
	WakeWhitelist []string          `json:"wake_whitelist,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	Volume        string            `json:"volume,omitempty"`
}

// Record returns the API view. Replica counts are not part of the model.
func (w World) Record() Record {
	return Record{
		ID:            w.ID(),
		Name:          w.Spec.Name,
		Game:          w.Spec.Game,
		Image:         w.Spec.Image,
		Allocation:    w.Spec.Allocation,
		Backend:       w.Spec.Backend,
		IdleTimeout:   w.Spec.IdleTimeout,
		StartTimeout:  w.Spec.StartTimeout,
		StopTimeout:   w.Spec.StopTimeout,
		OccupyMode:    w.Spec.OccupyMode,
		AsleepMOTD:    w.Spec.AsleepMOTD,
		StartingMOTD:  w.Spec.StartingMOTD,
		WakeWhitelist: w.Spec.WakeWhitelist,
		Env:           w.Spec.Env,
		Volume:        w.Spec.Volume,
	}
}

// World converts an API record into the custom resource.
func (r Record) World() World {
	return World{
		APIVersion: APIVersionV1,
		Kind:       Kind,
		Metadata:   Metadata{Name: r.ID},
		Spec: Spec{
			Name:          r.Name,
			Game:          r.Game,
			Image:         r.Image,
			Allocation:    r.Allocation,
			Backend:       r.Backend,
			IdleTimeout:   r.IdleTimeout,
			StartTimeout:  r.StartTimeout,
			StopTimeout:   r.StopTimeout,
			OccupyMode:    r.OccupyMode,
			AsleepMOTD:    r.AsleepMOTD,
			StartingMOTD:  r.StartingMOTD,
			WakeWhitelist: r.WakeWhitelist,
			Env:           r.Env,
			Volume:        r.Volume,
		},
	}
}

// ApplyDefaults fills omitted settings. It does not set replica counts.
func ApplyDefaults(r *Record) {
	if r.ID == "" {
		r.ID = slug(r.Name)
	}
	if r.Game == "" {
		r.Game = GameMinecraftJava
	}
	if r.IdleTimeout == "" {
		r.IdleTimeout = "15m"
	}
	if r.StartTimeout == "" {
		r.StartTimeout = "10m"
	}
	if r.StopTimeout == "" {
		r.StopTimeout = "2m"
	}
	if r.OccupyMode == "" {
		r.OccupyMode = DefaultOccupyMode
	}
	if r.Game == GameMinecraftJava {
		if r.Image == "" {
			r.Image = DefaultImage
		}
		if r.Allocation.Port == 0 {
			r.Allocation.Port = DefaultMCPort
		}
		if r.Allocation.Protocol == "" {
			r.Allocation.Protocol = DefaultProtocol
		}
	}
	if r.Backend.Service == "" {
		r.Backend.Service = r.ID
	}
	if r.Backend.Workload == "" {
		r.Backend.Workload = r.Backend.Service
	}
	if r.Volume == "" && r.ID != "" {
		r.Volume = r.ID + "-data"
	}
}

// Validate checks required fields and timeout bounds.
func Validate(r Record) error {
	if r.ID == "" || !idPattern.MatchString(r.ID) {
		return fmt.Errorf("id must be a DNS label")
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if r.Game != GameMinecraftJava {
		return fmt.Errorf("unsupported game %q", r.Game)
	}
	idle, err := time.ParseDuration(r.IdleTimeout)
	if err != nil {
		return fmt.Errorf("idle_timeout: %w", err)
	}
	if idle < MinIdleTimeout {
		return fmt.Errorf("idle_timeout must be at least %s", MinIdleTimeout)
	}
	if _, err := time.ParseDuration(r.StartTimeout); err != nil {
		return fmt.Errorf("start_timeout: %w", err)
	}
	if _, err := time.ParseDuration(r.StopTimeout); err != nil {
		return fmt.Errorf("stop_timeout: %w", err)
	}
	switch r.OccupyMode {
	case "hold", "kick", "retry":
	default:
		return fmt.Errorf("occupy_mode must be hold, kick, or retry")
	}
	return nil
}

// Merge copies non-empty fields from src onto dst. ID is never changed.
func Merge(dst *Record, src Record) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Game != "" {
		dst.Game = src.Game
	}
	if src.Image != "" {
		dst.Image = src.Image
	}
	if src.Allocation.Host != "" {
		dst.Allocation.Host = src.Allocation.Host
	}
	if src.Allocation.Port != 0 {
		dst.Allocation.Port = src.Allocation.Port
	}
	if src.Allocation.Protocol != "" {
		dst.Allocation.Protocol = src.Allocation.Protocol
	}
	if src.Backend.Service != "" {
		dst.Backend.Service = src.Backend.Service
	}
	if src.Backend.Workload != "" {
		dst.Backend.Workload = src.Backend.Workload
	}
	if src.IdleTimeout != "" {
		dst.IdleTimeout = src.IdleTimeout
	}
	if src.StartTimeout != "" {
		dst.StartTimeout = src.StartTimeout
	}
	if src.StopTimeout != "" {
		dst.StopTimeout = src.StopTimeout
	}
	if src.OccupyMode != "" {
		dst.OccupyMode = src.OccupyMode
	}
	if src.AsleepMOTD != "" {
		dst.AsleepMOTD = src.AsleepMOTD
	}
	if src.StartingMOTD != "" {
		dst.StartingMOTD = src.StartingMOTD
	}
	if src.WakeWhitelist != nil {
		dst.WakeWhitelist = src.WakeWhitelist
	}
	if src.Env != nil {
		dst.Env = src.Env
	}
	if src.Volume != "" {
		dst.Volume = src.Volume
	}
}

func slug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			return r
		case r == ' ' || r == '_' || r == '.':
			return '-'
		default:
			return -1
		}
	}, s)
	return strings.Trim(s, "-")
}
