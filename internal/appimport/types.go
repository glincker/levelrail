// Package appimport turns a source platform's application inventory into a
// reviewed, staged import: per-app verdicts, a hostname mapping with a
// visible diff, preflight checks and a secret-free receipt. It reads from
// internal/platformimport and never talks to the source itself.
package appimport

// Verdicts for one source application or service.
const (
	VerdictReady       = "ready"
	VerdictNotes       = "ready-with-notes"
	VerdictAttention   = "needs-attention"
	VerdictUnsupported = "unsupported"
)

// Kinds of inventory entries.
const (
	KindApp     = "app"
	KindService = "service"
)

// Where an app's code comes from, for display.
const (
	SourceGit     = "git"
	SourceImage   = "image"
	SourceCompose = "compose"
	SourceService = "service"
	SourceUnknown = "unknown"
)

// Build methods an app maps to here.
const (
	MapDockerfile = "dockerfile"
	MapRailpack   = "railpack"
	MapStatic     = "static"
	MapImage      = "image"
)

// Finding is one reason behind a verdict and what to do about it.
type Finding struct {
	Reason string `json:"reason"`
	Next   string `json:"next,omitempty"`
}

// EnvSummary counts variables by sensitivity. Values are never included.
type EnvSummary struct {
	Plain  int `json:"plain"`
	Secret int `json:"secret"`
	// Empty counts secrets that came back without a readable value.
	Empty int `json:"empty"`
}

// VolumeEntry is one persistent mount.
type VolumeEntry struct {
	Name          string `json:"name"`
	ContainerPath string `json:"container_path"`
	HostPath      string `json:"host_path,omitempty"`
	SizeBytes     int64  `json:"size_bytes"`
	SizeKnown     bool   `json:"size_known"`
}

// HealthEntry is the source's HTTP probe.
type HealthEntry struct {
	Path            string `json:"path"`
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty"`
	Retries         int    `json:"retries,omitempty"`
}

// DBRef is a database one app's configuration points at.
type DBRef struct {
	SourceID string `json:"source_id"`
	Name     string `json:"name"`
	// Host is the in-network hostname apps on the source use.
	Host string `json:"host"`
	// Target is the database here the data was moved to, empty when none.
	Target string `json:"target,omitempty"`
	// TargetHost is the connection host of Target here.
	TargetHost string `json:"target_host,omitempty"`
}

// Entry is one row of the inventory.
type Entry struct {
	SourceID    string        `json:"source_id"`
	Name        string        `json:"name"`
	Kind        string        `json:"kind"`
	Project     string        `json:"project,omitempty"`
	Environment string        `json:"environment,omitempty"`
	Server      string        `json:"server,omitempty"`
	Source      string        `json:"source"`
	Repo        string        `json:"repo,omitempty"`
	Branch      string        `json:"branch,omitempty"`
	Image       string        `json:"image,omitempty"`
	ImageID     string        `json:"image_id,omitempty"`
	HostBuilt   bool          `json:"host_built,omitempty"`
	BuildPack   string        `json:"build_pack,omitempty"`
	MapsTo      string        `json:"maps_to,omitempty"`
	Port        int           `json:"port,omitempty"`
	Domains     []string      `json:"domains,omitempty"`
	Env         EnvSummary    `json:"env"`
	Volumes     []VolumeEntry `json:"volumes,omitempty"`
	Health      *HealthEntry  `json:"health,omitempty"`
	MemoryBytes int64         `json:"memory_bytes,omitempty"`
	NanoCPUs    int64         `json:"nano_cpus,omitempty"`
	Databases   []DBRef       `json:"databases,omitempty"`
	Verdict     string        `json:"verdict"`
	Findings    []Finding     `json:"findings,omitempty"`
}

// Group is the entries of one project environment.
type Group struct {
	Project     string  `json:"project"`
	Environment string  `json:"environment"`
	Entries     []Entry `json:"entries"`
}

// Inventory is everything read from the source, grouped and judged.
type Inventory struct {
	Platform  string         `json:"platform"`
	Groups    []Group        `json:"groups"`
	Databases []DBRef        `json:"databases"`
	Counts    map[string]int `json:"counts"`
}

// Entries returns every entry across groups.
func (i Inventory) Entries() []Entry {
	var out []Entry
	for _, g := range i.Groups {
		out = append(out, g.Entries...)
	}
	return out
}

// Mapping rewrites one source hostname to a new one in env values.
type Mapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}
