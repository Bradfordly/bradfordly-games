package world

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/bradfordly/bradfordly-games/internal/gameprofile"
)

const (
	DefaultIdleTimeout  = 15 * time.Minute
	DefaultStartTimeout = 10 * time.Minute
	DefaultStopTimeout  = 2 * time.Minute
	MinIdleTimeout      = time.Minute
	DefaultOccupyMode   = "kick"
	DefaultAllocPort    = 25565
	DefaultAllocProto   = "tcp"
	DefaultDomain       = "games.bradfordly.com"
)

var (
	ErrNotFound     = errors.New("world not found")
	ErrInvalidGame  = errors.New("game must be minecraft-java")
	ErrNameRequired = errors.New("name is required")
	ErrIdleTimeout  = errors.New("idle_timeout must be at least 1m")
	ErrConflict     = errors.New("world already exists")
	ErrInvalid      = errors.New("invalid world")
)

// Allocation is the public endpoint players type.
type Allocation struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// Backend names the Docker container for a world.
type Backend struct {
	Container string `json:"container"`
}

// World is the control-plane spec from docs/specs/control-plane.md.
type World struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	Game          string            `json:"game"`
	Image         string            `json:"image"`
	Allocation    Allocation        `json:"allocation"`
	Backend       Backend           `json:"backend"`
	IdleTimeout   string            `json:"idle_timeout"`
	StartTimeout  string            `json:"start_timeout"`
	StopTimeout   string            `json:"stop_timeout"`
	OccupyMode    string            `json:"occupy_mode"`
	AsleepMOTD    string            `json:"asleep_motd"`
	StartingMOTD  string            `json:"starting_motd"`
	WakeWhitelist []string          `json:"wake_whitelist"`
	Env           map[string]string `json:"env"`
	Volume        string            `json:"volume"`
}

// CreateRequest is the operator input for POST /api/worlds.
type CreateRequest struct {
	Name          string            `json:"name"`
	Game          string            `json:"game"`
	Image         string            `json:"image"`
	IdleTimeout   string            `json:"idle_timeout"`
	StartTimeout  string            `json:"start_timeout"`
	StopTimeout   string            `json:"stop_timeout"`
	OccupyMode    string            `json:"occupy_mode"`
	AsleepMOTD    string            `json:"asleep_motd"`
	StartingMOTD  string            `json:"starting_motd"`
	WakeWhitelist []string          `json:"wake_whitelist"`
	Env           map[string]string `json:"env"`
	Host          string            `json:"host"`
}

// UpdateRequest is the operator input for PATCH /api/worlds/:id.
type UpdateRequest struct {
	Name          *string           `json:"name"`
	Image         *string           `json:"image"`
	IdleTimeout   *string           `json:"idle_timeout"`
	StartTimeout  *string           `json:"start_timeout"`
	StopTimeout   *string           `json:"stop_timeout"`
	OccupyMode    *string           `json:"occupy_mode"`
	AsleepMOTD    *string           `json:"asleep_motd"`
	StartingMOTD  *string           `json:"starting_motd"`
	WakeWhitelist []string          `json:"wake_whitelist"`
	Env           map[string]string `json:"env"`
}

func (w World) IdleDuration() (time.Duration, error) {
	return parseTimeout(w.IdleTimeout, DefaultIdleTimeout)
}

func parseTimeout(raw string, fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	return d, nil
}

func slug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevHyphen = false
			continue
		}
		if !prevHyphen && b.Len() > 0 {
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "world"
	}
	return out
}

func normalizeCreate(req CreateRequest, id, dataDir, domain string) (World, error) {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return World{}, ErrNameRequired
	}
	if req.Game == "" {
		req.Game = gameprofile.MinecraftJava
	}
	profile, ok := gameprofile.Lookup(req.Game)
	if !ok || req.Game != gameprofile.MinecraftJava {
		return World{}, ErrInvalidGame
	}

	idle, err := parseTimeout(req.IdleTimeout, DefaultIdleTimeout)
	if err != nil {
		return World{}, err
	}
	if idle < MinIdleTimeout {
		return World{}, ErrIdleTimeout
	}
	start, err := parseTimeout(req.StartTimeout, DefaultStartTimeout)
	if err != nil {
		return World{}, err
	}
	stop, err := parseTimeout(req.StopTimeout, DefaultStopTimeout)
	if err != nil {
		return World{}, err
	}

	occupy := req.OccupyMode
	if occupy == "" {
		occupy = DefaultOccupyMode
	}
	switch occupy {
	case "kick", "hold", "retry":
	default:
		return World{}, fmt.Errorf("%w: occupy_mode", ErrInvalid)
	}

	image := req.Image
	if image == "" {
		image = profile.Image
	}

	host := strings.TrimSpace(req.Host)
	if host == "" {
		if domain == "" {
			domain = DefaultDomain
		}
		host = slug(req.Name) + "." + domain
	}

	env := map[string]string{}
	for _, item := range profile.Env {
		env[item.Name] = item.Value
	}
	for k, v := range req.Env {
		env[k] = v
	}
	env["EULA"] = "TRUE"

	if req.WakeWhitelist == nil {
		req.WakeWhitelist = []string{}
	}

	w := World{
		ID:            id,
		Name:          req.Name,
		Game:          req.Game,
		Image:         image,
		Allocation:    Allocation{Host: host, Port: DefaultAllocPort, Protocol: DefaultAllocProto},
		Backend:       Backend{Container: "world-" + id},
		IdleTimeout:   idle.String(),
		StartTimeout:  start.String(),
		StopTimeout:   stop.String(),
		OccupyMode:    occupy,
		AsleepMOTD:    req.AsleepMOTD,
		StartingMOTD:  req.StartingMOTD,
		WakeWhitelist: req.WakeWhitelist,
		Env:           env,
		Volume:        saveDir(dataDir, id),
	}
	return w, nil
}

func applyUpdate(current World, req UpdateRequest) (World, error) {
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return World{}, ErrNameRequired
		}
		current.Name = name
	}
	if req.Image != nil {
		if strings.TrimSpace(*req.Image) == "" {
			return World{}, fmt.Errorf("%w: image", ErrInvalid)
		}
		current.Image = strings.TrimSpace(*req.Image)
	}
	if req.IdleTimeout != nil {
		idle, err := parseTimeout(*req.IdleTimeout, DefaultIdleTimeout)
		if err != nil {
			return World{}, err
		}
		if idle < MinIdleTimeout {
			return World{}, ErrIdleTimeout
		}
		current.IdleTimeout = idle.String()
	}
	if req.StartTimeout != nil {
		start, err := parseTimeout(*req.StartTimeout, DefaultStartTimeout)
		if err != nil {
			return World{}, err
		}
		current.StartTimeout = start.String()
	}
	if req.StopTimeout != nil {
		stop, err := parseTimeout(*req.StopTimeout, DefaultStopTimeout)
		if err != nil {
			return World{}, err
		}
		current.StopTimeout = stop.String()
	}
	if req.OccupyMode != nil {
		switch *req.OccupyMode {
		case "kick", "hold", "retry":
			current.OccupyMode = *req.OccupyMode
		default:
			return World{}, fmt.Errorf("%w: occupy_mode", ErrInvalid)
		}
	}
	if req.AsleepMOTD != nil {
		current.AsleepMOTD = *req.AsleepMOTD
	}
	if req.StartingMOTD != nil {
		current.StartingMOTD = *req.StartingMOTD
	}
	if req.WakeWhitelist != nil {
		current.WakeWhitelist = req.WakeWhitelist
	}
	if req.Env != nil {
		if current.Env == nil {
			current.Env = map[string]string{}
		}
		for k, v := range req.Env {
			current.Env[k] = v
		}
		current.Env["EULA"] = "TRUE"
	}
	return current, nil
}

func saveDir(dataDir, id string) string {
	return strings.TrimRight(dataDir, "/") + "/worlds/" + id
}
