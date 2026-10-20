package apiclient

import (
	"context"
	"net/http"
)

// ImportPlanRequest is POST /api/v1/imports/plan's body.
type ImportPlanRequest struct {
	Text string            `json:"text"`
	Kind string            `json:"kind,omitempty"`
	Ref  string            `json:"ref,omitempty"`
	Name string            `json:"name,omitempty"`
	Port int               `json:"port,omitempty"`
	Env  map[string]string `json:"env,omitempty"`
}

// ImportEnvVar mirrors internal/importplan's EnvVar.
type ImportEnvVar struct {
	Key        string `json:"key"`
	Value      string `json:"value,omitempty"`
	Required   bool   `json:"required"`
	HasDefault bool   `json:"has_default"`
	Secret     bool   `json:"secret"`
	Source     string `json:"source,omitempty"`
}

// ImportPort mirrors internal/importplan's PortMapping.
type ImportPort struct {
	Host      int `json:"host,omitempty"`
	Container int `json:"container"`
}

// ImportVolume mirrors internal/importplan's VolumeMount.
type ImportVolume struct {
	Name          string `json:"name,omitempty"`
	HostPath      string `json:"host_path,omitempty"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only,omitempty"`
	NeedsApproval bool   `json:"needs_approval,omitempty"`
}

// ImportWarning mirrors internal/importplan's Warning.
type ImportWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ImportService mirrors internal/importplan's ServicePlan.
type ImportService struct {
	Name        string         `json:"name"`
	Image       string         `json:"image,omitempty"`
	Build       string         `json:"build"`
	BuildReason string         `json:"build_reason,omitempty"`
	Port        int            `json:"port,omitempty"`
	Ports       []ImportPort   `json:"ports,omitempty"`
	Command     []string       `json:"command,omitempty"`
	Entrypoint  []string       `json:"entrypoint,omitempty"`
	HealthPath  string         `json:"health_path,omitempty"`
	Env         []ImportEnvVar `json:"env,omitempty"`
	Volumes     []ImportVolume `json:"volumes,omitempty"`
	Restart     string         `json:"restart,omitempty"`
	MemoryBytes int64          `json:"memory_bytes,omitempty"`
	NanoCPUs    int64          `json:"nano_cpus,omitempty"`
}

// ImportPlan mirrors internal/importplan's DeploymentPlan.
type ImportPlan struct {
	Source             string          `json:"source"`
	SuggestedName      string          `json:"suggested_name"`
	Deploy             string          `json:"deploy"`
	RepoURL            string          `json:"repo_url,omitempty"`
	Ref                string          `json:"ref,omitempty"`
	ComposeYAML        string          `json:"compose_yaml,omitempty"`
	Services           []ImportService `json:"services"`
	DomainSuggestion   string          `json:"domain_suggestion,omitempty"`
	Warnings           []ImportWarning `json:"warnings"`
	MissingRequiredEnv []string        `json:"missing_required_env"`
}

// PlanImport calls POST /api/v1/imports/plan. It creates nothing.
func (c *Client) PlanImport(ctx context.Context, req ImportPlanRequest) (ImportPlan, error) {
	var out ImportPlan
	err := c.do(ctx, http.MethodPost, "/api/v1/imports/plan", req, &out)
	return out, err
}
