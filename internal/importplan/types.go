// Package importplan classifies an import input (repo URL, docker run
// command, image reference, compose file or Dockerfile) and builds a
// side-effect free deployment plan preview from it.
package importplan

// SourceKind is what Classify decided the input is.
type SourceKind string

// Source kinds Classify can return.
const (
	SourceRepo       SourceKind = "repo"
	SourceDockerRun  SourceKind = "docker_run"
	SourceImage      SourceKind = "image"
	SourceCompose    SourceKind = "compose"
	SourceDockerfile SourceKind = "dockerfile"
)

// BuildMethod is how the plan's app gets its image.
type BuildMethod string

// Build methods a plan can use.
const (
	BuildImage      BuildMethod = "image"
	BuildDockerfile BuildMethod = "dockerfile"
	BuildRailpack   BuildMethod = "railpack"
	BuildStatic     BuildMethod = "static"
	BuildCompose    BuildMethod = "compose"
	BuildUnknown    BuildMethod = "unknown"
)

// DeployKind names which existing create/deploy endpoint a plan feeds.
type DeployKind string

const (
	// DeployApp is POST /api/v1/apps with a prebuilt image.
	DeployApp DeployKind = "app"
	// DeployBuild is POST /api/v1/apps then POST /api/v1/apps/{name}/builds.
	DeployBuild DeployKind = "build"
	// DeployCompose is POST /api/v1/apps/{name}/compose.
	DeployCompose DeployKind = "compose"
	// DeployNone means the input alone cannot be deployed (for example a bare Dockerfile snippet).
	DeployNone DeployKind = "none"
)

// EnvVar is one discovered environment variable. Value is empty for a
// secret-looking variable; the plan never echoes such defaults back.
type EnvVar struct {
	Key        string `json:"key"`
	Value      string `json:"value,omitempty"`
	Required   bool   `json:"required"`
	HasDefault bool   `json:"has_default"`
	Secret     bool   `json:"secret"`
	Source     string `json:"source,omitempty"`
}

// PortMapping is one published or exposed port.
type PortMapping struct {
	Host      int `json:"host,omitempty"`
	Container int `json:"container"`
}

// VolumeMount is one volume or bind mount.
type VolumeMount struct {
	Name          string `json:"name,omitempty"`
	HostPath      string `json:"host_path,omitempty"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only,omitempty"`
	// NeedsApproval flags a host bind mount, which only root may create.
	NeedsApproval bool `json:"needs_approval,omitempty"`
}

// Warning is a note the operator should read before deploying.
type Warning struct {
	// Code is stable and machine readable, for example "unsupported_flag".
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ServicePlan is the plan for one service (compose files carry several).
type ServicePlan struct {
	Name        string        `json:"name"`
	Image       string        `json:"image,omitempty"`
	Build       BuildMethod   `json:"build"`
	BuildReason string        `json:"build_reason,omitempty"`
	Port        int           `json:"port,omitempty"`
	Ports       []PortMapping `json:"ports,omitempty"`
	Command     []string      `json:"command,omitempty"`
	Entrypoint  []string      `json:"entrypoint,omitempty"`
	HealthPath  string        `json:"health_path,omitempty"`
	Env         []EnvVar      `json:"env,omitempty"`
	Volumes     []VolumeMount `json:"volumes,omitempty"`
	Restart     string        `json:"restart,omitempty"`
	MemoryBytes int64         `json:"memory_bytes,omitempty"`
	NanoCPUs    int64         `json:"nano_cpus,omitempty"`
}

// DeploymentPlan is the full preview returned before anything is created.
type DeploymentPlan struct {
	Source        SourceKind `json:"source"`
	SuggestedName string     `json:"suggested_name"`
	Deploy        DeployKind `json:"deploy"`
	// RepoURL and Ref are set for repo sources.
	RepoURL string `json:"repo_url,omitempty"`
	Ref     string `json:"ref,omitempty"`
	// ComposeYAML is the compose document to POST for DeployCompose,
	// generated from a docker run command or supplied by the caller.
	ComposeYAML string        `json:"compose_yaml,omitempty"`
	Services    []ServicePlan `json:"services"`
	// DomainSuggestion is a hint only; nothing is configured from it.
	DomainSuggestion string    `json:"domain_suggestion,omitempty"`
	Warnings         []Warning `json:"warnings"`
	// MissingRequiredEnv lists required variables with no value; deploy is blocked until each has one.
	MissingRequiredEnv []string `json:"missing_required_env"`
}

// Input is the raw request to Classify and Plan.
type Input struct {
	// Text is a URL, image reference, docker run command, compose YAML or Dockerfile.
	Text string `json:"text"`
	// Kind overrides classification when set.
	Kind SourceKind `json:"kind,omitempty"`
	// Ref selects a branch for repo sources.
	Ref string `json:"ref,omitempty"`
	// Name overrides the suggested app name.
	Name string `json:"name,omitempty"`
	// Port overrides the detected container port.
	Port int `json:"port,omitempty"`
	// Env carries operator-supplied values, keyed by variable name. They
	// satisfy required variables and are injected into a generated compose file.
	Env map[string]string `json:"env,omitempty"`
}
