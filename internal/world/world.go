// Package world is the World custom resource: the control-plane record
// for one game world. Gateway state is not stored here.
package world

const (
	APIGroup     = "games.bradfordly.com"
	APIVersion   = "v1"
	APIVersionV1 = APIGroup + "/" + APIVersion
	Kind         = "World"
	Plural       = "worlds"
)

// World is a namespaced custom resource. metadata.name is the stable id.
type World struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
	Spec       Spec     `json:"spec"`
}

// Metadata is the Kubernetes object identity.
type Metadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// Spec matches the control-plane world model. It does not include replica counts.
type Spec struct {
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

// Allocation is the public player endpoint.
type Allocation struct {
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Protocol string `json:"protocol,omitempty"`
}

// Backend names the in-cluster Service and workload.
type Backend struct {
	Service  string `json:"service,omitempty"`
	Workload string `json:"workload,omitempty"`
}

// ID is the stable identifier (metadata.name).
func (w World) ID() string {
	return w.Metadata.Name
}
