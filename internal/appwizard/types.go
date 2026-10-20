// Package appwizard analyses a git repository through a provider file-tree
// API and returns pre-filled, evidence-backed suggestions for creating an app.
// Repository text is parsed as data only and env values are never returned.
package appwizard

import "github.com/GLINCKER/levelrail/internal/repolayout"

// Env vars bounding one analysis.
const (
	EnvMaxFiles     = "APP_WIZARD_MAX_FILES"
	EnvMaxFileBytes = "APP_WIZARD_MAX_FILE_BYTES"
	EnvMaxEnvKeys   = "APP_WIZARD_MAX_ENV_KEYS"
	envMemoryPrefix = "APP_WIZARD_MEMORY_MB_"
)

// Warning codes a dashboard can translate.
const (
	WarnNothingDetected = "nothing_detected"
	WarnReadBudget      = "read_budget"
	WarnTruncated       = "tree_truncated"
	WarnMonorepo        = "monorepo_choose_app"
	WarnNoPort          = "port_unknown"

	WarnDirectoryMissing = "directory_missing"
)

// Stack is the detected runtime and framework.
type Stack struct {
	Runtime   string `json:"runtime"`
	Framework string `json:"framework,omitempty"`
	Evidence  string `json:"evidence"`
}

// Build is the suggested build configuration, relative to the repository root.
type Build struct {
	Type           string                  `json:"type"`
	DockerfilePath string                  `json:"dockerfile_path,omitempty"`
	BaseDirectory  string                  `json:"base_directory,omitempty"`
	Evidence       string                  `json:"evidence"`
	Alternatives   []repolayout.Suggestion `json:"alternatives"`
}

// Text is a suggested string with the evidence behind it.
type Text struct {
	Value    string `json:"value"`
	Evidence string `json:"evidence"`
}

// Port is the suggested listening port.
type Port struct {
	Value    int    `json:"value"`
	Evidence string `json:"evidence"`
}

// Memory is the suggested memory limit.
type Memory struct {
	Bytes    int64  `json:"bytes"`
	Evidence string `json:"evidence"`
}

// EnvKey is one environment variable the repository declares. It never
// carries a value.
type EnvKey struct {
	Key        string `json:"key"`
	Required   bool   `json:"required"`
	Secret     bool   `json:"secret"`
	Source     string `json:"source"`
	ProvidedBy string `json:"provided_by,omitempty"`
	HasDefault bool   `json:"has_default"`
}

// DatabaseNeed is a managed database the app appears to need.
type DatabaseNeed struct {
	Engine        string `json:"engine"`
	Version       string `json:"version"`
	SuggestedName string `json:"suggested_name"`
	EnvVar        string `json:"env_var"`
	Field         string `json:"field"`
	Evidence      string `json:"evidence"`
}

// Warning is a note to show before deploying.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Analysis is the full set of suggestions for one repository and ref.
type Analysis struct {
	RepoURL       string               `json:"repo_url"`
	Branch        string               `json:"branch"`
	Monorepo      bool                 `json:"monorepo"`
	AppDirectory  string               `json:"app_directory,omitempty"`
	Name          Text                 `json:"name"`
	Stack         Stack                `json:"stack"`
	Build         Build                `json:"build"`
	Port          Port                 `json:"port"`
	Health        Text                 `json:"health"`
	Memory        Memory               `json:"memory"`
	Domain        Text                 `json:"domain"`
	Env           []EnvKey             `json:"env"`
	Databases     []DatabaseNeed       `json:"databases"`
	Warnings      []Warning            `json:"warnings"`
	Detection     Detection            `json:"detection"`
	Packages      []repolayout.Package `json:"packages"`
	FilesRead     int                  `json:"files_read"`
	TreeTruncated bool                 `json:"tree_truncated"`
}

// Options carries per-request inputs and environment hooks.
type Options struct {
	RepoURL string
	Branch  string
	// BaseDirectory, DockerfilePath and BuildType override the recommended
	// build suggestion when the operator picked an alternative.
	BaseDirectory  string
	DockerfilePath string
	BuildType      string
	Name           string
	// AppsBaseDomain, when set with DomainFor, yields a suggested domain.
	AppsBaseDomain string
	DomainFor      func(name, base string) string
	// DefaultVersion returns an engine's default version, "" when unknown.
	DefaultVersion func(engine string) string
}
