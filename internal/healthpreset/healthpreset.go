// Package healthpreset is the one list of health check shortcuts. The API serves it,
// the CLI reads it and the dashboard renders its buttons from the API, so a new
// framework default is one entry here.
package healthpreset

import "strings"

// Preset is one named HTTP health path.
type Preset struct {
	ID   string `json:"id"`
	Path string `json:"path"`
	// Hint is the English one line note on which stacks serve this path.
	Hint string `json:"hint"`
}

// Timing is the probe timing a preset fills for one probe kind. Zero means leave unset.
type Timing struct {
	IntervalSeconds int `json:"interval_seconds"`
	TimeoutSeconds  int `json:"timeout_seconds"`
	Failures        int `json:"failures"`
}

// Catalog is the payload of GET /api/v1/health-presets.
type Catalog struct {
	Presets   []Preset `json:"presets"`
	Readiness Timing   `json:"readiness"`
	Liveness  Timing   `json:"liveness"`
}

var presets = []Preset{
	{ID: "healthz", Path: "/healthz", Hint: "common Go and container default"},
	{ID: "health", Path: "/health", Hint: "common default across frameworks"},
	{ID: "api-health", Path: "/api/health", Hint: "API services behind an /api prefix"},
	{ID: "ping", Path: "/ping", Hint: "lightweight liveness endpoint"},
	{ID: "status", Path: "/status", Hint: "status endpoint"},
	{ID: "up", Path: "/up", Hint: "Rails default health route"},
	{ID: "actuator", Path: "/actuator/health", Hint: "Spring Boot actuator"},
	{ID: "readyz", Path: "/readyz", Hint: "readiness route in the container ecosystem"},
}

// Get returns a preset by id.
func Get(id string) (Preset, bool) {
	for _, p := range presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}

// Names lists the preset ids for help text.
func Names() string {
	ids := make([]string, len(presets))
	for i, p := range presets {
		ids[i] = p.ID
	}
	return strings.Join(ids, ", ")
}

// TimingFor returns the timing a preset fills for "readiness" or "liveness".
// Readiness gates a cutover (short, no failure count); liveness restarts a hung container.
func TimingFor(which string) Timing {
	if which == "liveness" {
		return Timing{IntervalSeconds: 30, Failures: 3}
	}
	return Timing{IntervalSeconds: 5, TimeoutSeconds: 2}
}

// Current builds the API payload.
func Current() Catalog {
	out := make([]Preset, len(presets))
	copy(out, presets)
	return Catalog{Presets: out, Readiness: TimingFor("readiness"), Liveness: TimingFor("liveness")}
}
