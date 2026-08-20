package apiclient

import (
	"context"
	"net/http"
)

// AppStatusSummary mirrors internal/api's appStatusSummary
// (internal/api/app_status.go): a compact category rollup, not the raw
// conditions list ConditionResource carries.
type AppStatusSummary struct {
	Label   string `json:"label"`
	Variant string `json:"variant"`
}

// AppGroupResource mirrors internal/api's appGroupResource
// (internal/api/apps_group.go): GET /api/v1/apps/{name}/group's response,
// name's sibling services under the same store.App plus one rollup
// status across all of them.
type AppGroupResource struct {
	AppID    string           `json:"app_id,omitempty"`
	Services []AppResource    `json:"services"`
	Status   AppStatusSummary `json:"status"`
}

// DeploySpecServiceBuild mirrors one service's build: block
// (internal/spec.Build's JSON wire shape) as POST
// /api/v1/apps/{name}/deploy-spec expects it: human-written field names,
// not the bytes/nanoCPUs encoding AppResource's own Resources/Health use
// for the general create/update endpoints. handleDeploySpec decodes this
// straight into internal/spec.Service, which has no json struct tags of
// its own; encoding/json matches these lowercase keys to that struct's
// exported Go field names case-insensitively.
type DeploySpecServiceBuild struct {
	Type               string `json:"type"`
	Path               string `json:"path,omitempty"`
	BaseDirectory      string `json:"baseDirectory,omitempty"`
	Image              string `json:"image,omitempty"`
	RegistryCredential string `json:"registryCredential,omitempty"`
}

// DeploySpecServiceEnv mirrors one entry in a deploy-spec service's env:
// map (internal/spec.EnvVar's JSON wire shape). Unlike app.yaml's own
// YAML parsing, a plain string shorthand is not accepted over JSON
// (EnvVar has no UnmarshalJSON): every entry must be this object shape,
// Value set for a literal value.
type DeploySpecServiceEnv struct {
	Value    string `json:"value,omitempty"`
	From     string `json:"from,omitempty"`
	Secret   bool   `json:"secret,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// DeploySpecServiceProbe mirrors internal/spec.Probe: human-readable
// duration strings ("5s"), not ServiceProbe's nanosecond encoding.
type DeploySpecServiceProbe struct {
	Path     string `json:"path"`
	Interval string `json:"interval,omitempty"`
	Timeout  string `json:"timeout,omitempty"`
	Failures int    `json:"failures,omitempty"`
}

// DeploySpecServiceHealth mirrors internal/spec.Health.
type DeploySpecServiceHealth struct {
	Readiness *DeploySpecServiceProbe `json:"readiness,omitempty"`
	Liveness  *DeploySpecServiceProbe `json:"liveness,omitempty"`
}

// DeploySpecServiceResources mirrors internal/spec.Resources:
// human-readable "512Mi"/0.5 values, not ServiceResources' bytes/
// nanoCPUs encoding.
type DeploySpecServiceResources struct {
	Memory string  `json:"memory,omitempty"`
	CPU    float64 `json:"cpu,omitempty"`
}

// DeploySpecService mirrors one entry in app.yaml's services: map
// (internal/spec.Service) as POST /api/v1/apps/{name}/deploy-spec
// expects to receive it directly, field for field.
type DeploySpecService struct {
	Build     DeploySpecServiceBuild          `json:"build"`
	Domains   []string                        `json:"domains,omitempty"`
	Port      int                             `json:"port,omitempty"`
	Health    *DeploySpecServiceHealth        `json:"health,omitempty"`
	Resources *DeploySpecServiceResources     `json:"resources,omitempty"`
	Env       map[string]DeploySpecServiceEnv `json:"env,omitempty"`
	Replicas  int                             `json:"replicas,omitempty"`
	Strategy  string                          `json:"strategy,omitempty"`
}

// DeploySpecRequest mirrors internal/api's deploySpecRequest
// (internal/api/apps_multi.go): POST /api/v1/apps/{name}/deploy-spec's
// body, one app.yaml services: map fanned out under app name.
type DeploySpecRequest struct {
	RepoURL       string                       `json:"repo_url"`
	Ref           string                       `json:"ref"`
	ImageRepoBase string                       `json:"image_repo_base,omitempty"`
	Services      map[string]DeploySpecService `json:"services"`
}

// DeploySpecServiceResult mirrors internal/api's deploySpecServiceResult:
// one service key's own build+deploy outcome. Error is set on failure,
// Image on success, matching deploy.ServiceOutcome's own "never both"
// contract.
type DeploySpecServiceResult struct {
	ServiceKey  string `json:"service_key"`
	ServiceName string `json:"service_name"`
	Image       string `json:"image,omitempty"`
	Error       string `json:"error,omitempty"`
}

// DeploySpecResult mirrors internal/api's deploySpecResponse.
// AllSucceeded false means at least one ServiceKey's own Error is set;
// the request as a whole still succeeded at fanning out.
type DeploySpecResult struct {
	AppID        string                    `json:"app_id"`
	Services     []DeploySpecServiceResult `json:"services"`
	AllSucceeded bool                      `json:"all_succeeded"`
}

// GetAppGroup calls GET /api/v1/apps/{name}/group
// (internal/api/apps_group.go's handleGetAppGroup): name's sibling
// services under the same store.App, one call whether name already has
// siblings or is its own one-service group.
func (c *Client) GetAppGroup(ctx context.Context, name string) (AppGroupResource, error) {
	var out AppGroupResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/group", nil, &out)
	return out, err
}

// DeploySpec calls POST /api/v1/apps/{name}/deploy-spec
// (internal/api/apps_multi.go's handleDeploySpec): fans req.Services out
// into N independent builds+deploys under one store.App named name.
// Synchronous and blocking, same as TriggerBuild, for the identical
// reason (internal/api/apps_multi.go's own doc comment).
func (c *Client) DeploySpec(ctx context.Context, name string, req DeploySpecRequest) (DeploySpecResult, error) {
	var out DeploySpecResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/deploy-spec", req, &out)
	return out, err
}
