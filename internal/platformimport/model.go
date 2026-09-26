// Package platformimport reads apps from another self-hosted platform
// (read-only) into a neutral model and maps it to this platform's
// resources. The source credential is used in memory only.
package platformimport

import "context"

// Platform identifies a supported source.
type Platform string

// Supported source platforms.
const (
	Coolify  Platform = "coolify"
	Dokploy  Platform = "dokploy"
	CapRover Platform = "caprover"
	Compose  Platform = "compose"
	Dokku    Platform = "dokku"
)

// Source discovers the neutral model from one source platform.
type Source interface {
	Discover(ctx context.Context) (*Discovery, error)
}

// SourceKind says where an app's code comes from.
type SourceKind string

// Where an app comes from.
const (
	SourceImage   SourceKind = "image"
	SourceGit     SourceKind = "git"
	SourceUnknown SourceKind = "unknown"
)

// Env is one environment variable. Secret is true when the source marked
// it secret or the key looks like one.
type Env struct {
	Key    string
	Value  string
	Secret bool
}

// Volume is a persistent mount.
type Volume struct {
	Name          string
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// HealthCheck is an HTTP probe.
type HealthCheck struct {
	Path            string
	IntervalSeconds int
	TimeoutSeconds  int
	Retries         int
}

// Cron is a scheduled command.
type Cron struct {
	Name     string
	Schedule string
	Command  string
}

// App is one application in the neutral model.
type App struct {
	SourceID    string
	Name        string
	Project     string
	Kind        SourceKind
	Image       string
	GitURL      string
	GitBranch   string
	BuildMethod string
	BuildPath   string
	Port        int
	Env         []Env
	Domains     []string
	Volumes     []Volume
	Replicas    int
	Health      *HealthCheck
	Crons       []Cron
	MemoryBytes int64
	NanoCPUs    int64
	Notes       []Note
}

// Database is a managed database in the source platform.
type Database struct {
	SourceID string
	Name     string
	Project  string
	Engine   string
	Version  string
}

// Note is a per-item caveat found while reading the source.
type Note struct {
	Reason string
	Manual string
}

// Unsupported is something that cannot be imported.
type Unsupported struct {
	Kind     string
	SourceID string
	Name     string
	Reason   string
	Manual   string
}

// Discovery is everything read from a source.
type Discovery struct {
	Platform    Platform
	Projects    []string
	Apps        []App
	Databases   []Database
	Unsupported []Unsupported
}
