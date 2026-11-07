package apiclient

import (
	"context"
	"net/http"
)

// ProjectCloneInclude picks what a clone copies. Secret values are never
// copied whatever is set here.
type ProjectCloneInclude struct {
	EnvVars  bool   `json:"env_vars"`
	Domains  string `json:"domains"`
	Settings bool   `json:"settings"`
}

// ProjectCloneRequest is the body of POST /api/v1/projects/{id}/clone.
type ProjectCloneRequest struct {
	Name        string              `json:"name"`
	Environment string              `json:"environment,omitempty"`
	Include     ProjectCloneInclude `json:"include"`
	DryRun      bool                `json:"dry_run"`
}

// ProjectCloneApp is one app the clone creates.
type ProjectCloneApp struct {
	Source  string   `json:"source"`
	Name    string   `json:"name"`
	Domains []string `json:"domains"`
	EnvVars int      `json:"env_vars"`
}

// ProjectCloneSecretKey is a key created without a value.
type ProjectCloneSecretKey struct {
	Scope string `json:"scope"`
	Key   string `json:"key"`
}

// ProjectCloneSkip is something left behind, and why.
type ProjectCloneSkip struct {
	Scope  string `json:"scope"`
	Item   string `json:"item"`
	Reason string `json:"reason"`
}

// ProjectCloneRef names a project in a clone plan.
type ProjectCloneRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

// ProjectClonePlan is the clone answer: the plan on a dry run, the receipt otherwise.
type ProjectClonePlan struct {
	DryRun      bool                    `json:"dry_run"`
	Applied     bool                    `json:"applied"`
	Source      ProjectCloneRef         `json:"source"`
	Project     ProjectCloneRef         `json:"project"`
	Environment string                  `json:"environment,omitempty"`
	Apps        []ProjectCloneApp       `json:"apps"`
	EnvVarCount int                     `json:"env_var_count"`
	SecretKeys  []ProjectCloneSecretKey `json:"secret_keys_needing_values"`
	Skipped     []ProjectCloneSkip      `json:"skipped"`
}

// CloneProject calls POST /api/v1/projects/{id}/clone.
func (c *Client) CloneProject(ctx context.Context, id string, req ProjectCloneRequest) (ProjectClonePlan, error) {
	var out ProjectClonePlan
	err := c.do(ctx, http.MethodPost, projectPath(id)+"/clone", req, &out)
	return out, err
}
