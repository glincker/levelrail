package apiclient

import (
	"context"
	"errors"
	"net/http"
)

// EnvVaultRef is one vault reference in an apply plan.
type EnvVaultRef struct {
	Path    string `json:"path,omitempty"`
	Key     string `json:"key,omitempty"`
	Conn    string `json:"conn,omitempty"`
	Version int    `json:"version,omitempty"`
	Ref     string `json:"ref,omitempty"`
}

// EnvApplyRequest is the body of POST /api/v1/apps/{name}/env/apply.
type EnvApplyRequest struct {
	ExpectedVersion *int                   `json:"expected_version,omitempty"`
	Set             map[string]string      `json:"set,omitempty"`
	Unset           []string               `json:"unset,omitempty"`
	SecretSet       map[string]string      `json:"secret_set,omitempty"`
	SecretDelete    []string               `json:"secret_delete,omitempty"`
	Convert         []string               `json:"convert,omitempty"`
	VaultSet        map[string]EnvVaultRef `json:"vault_set,omitempty"`
	VaultClear      []string               `json:"vault_clear,omitempty"`
	OverwriteLocked bool                   `json:"overwrite_locked,omitempty"`
	DryRun          bool                   `json:"dry_run,omitempty"`
}

// EnvPlanChange is one key and what the plan does to it; never a value.
type EnvPlanChange struct {
	Key    string `json:"key"`
	Action string `json:"action"`
}

// EnvApplyResult carries names and the new version only.
type EnvApplyResult struct {
	App        string          `json:"app"`
	DryRun     bool            `json:"dry_run"`
	Changed    bool            `json:"changed"`
	EnvVersion int             `json:"env_version"`
	Plan       []EnvPlanChange `json:"plan"`
}

// EnvConflictCode is the error code of a 409 caused by a stale expected_version.
const EnvConflictCode = "env_version_conflict"

// EnvVersionConflictError reports a stale expected_version, so callers can branch on it.
type EnvVersionConflictError struct {
	Message string
}

func (e *EnvVersionConflictError) Error() string { return e.Message }

// SetSecretReplacing is PUT /apps/{name}/secrets/{key} and reports whether the server replaced
// a plain env copy of the key in the same request. buildTime nil leaves build access unchanged.
func (c *Client) SetSecretReplacing(ctx context.Context, name, key, value string, overwriteLocked bool, buildTime *bool) (bool, error) {
	if err := PrecheckEnv(map[string]string{key: value}, true); err != nil {
		return false, err
	}
	var out struct {
		ReplacedPlainEnv bool `json:"replaced_plain_env"`
	}
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/secrets/"+PathEscape(key), SetSecretRequest{Value: value, OverwriteLocked: overwriteLocked, BuildTime: buildTime}, &out)
	return out.ReplacedPlainEnv, err
}

// ApplyAppEnv calls POST /api/v1/apps/{name}/env/apply: plain, secret and vault changes in one transaction.
func (c *Client) ApplyAppEnv(ctx context.Context, name string, req EnvApplyRequest) (EnvApplyResult, error) {
	var out EnvApplyResult
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/env/apply", req, &out)
	var apiErr *APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict && apiErr.Code == EnvConflictCode {
		return out, &EnvVersionConflictError{Message: apiErr.Message}
	}
	return out, err
}
