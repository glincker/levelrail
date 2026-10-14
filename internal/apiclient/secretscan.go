package apiclient

import (
	"context"
	"net/http"
	"time"
)

// SecretFinding is one suspected credential. It never carries the value.
type SecretFinding struct {
	RuleID      string `json:"rule_id"`
	Source      string `json:"source"`
	Path        string `json:"path,omitempty"`
	Line        int    `json:"line,omitempty"`
	Var         string `json:"var,omitempty"`
	Fingerprint string `json:"fingerprint"`
	Allowed     bool   `json:"allowed,omitempty"`
}

// SecretScanAllowEntry is one allow-listed fingerprint with its audit trail.
type SecretScanAllowEntry struct {
	Fingerprint string    `json:"fingerprint"`
	Reason      string    `json:"reason"`
	Actor       string    `json:"actor"`
	CreatedAt   time.Time `json:"created_at"`
}

// SecretScanLimits are the server's scan caps.
type SecretScanLimits struct {
	MaxFiles       int   `json:"max_files"`
	MaxFileBytes   int64 `json:"max_file_bytes"`
	MaxTotalBytes  int64 `json:"max_total_bytes"`
	MaxFindings    int   `json:"max_findings"`
	TimeoutSeconds int   `json:"timeout_seconds"`
}

// SecretScanSettings is GET .../secret-scan's body.
type SecretScanSettings struct {
	App       string                 `json:"app"`
	Policy    string                 `json:"policy"`
	AllowList []SecretScanAllowEntry `json:"allow_list"`
	Limits    SecretScanLimits       `json:"limits"`
}

// SecretScanRun is the scan recorded for one deploy.
type SecretScanRun struct {
	DeploymentID string          `json:"deployment_id"`
	App          string          `json:"app"`
	Policy       string          `json:"policy"`
	Action       string          `json:"action"`
	Reason       string          `json:"reason,omitempty"`
	Status       string          `json:"status"`
	Error        string          `json:"error,omitempty"`
	FilesScanned int             `json:"files_scanned"`
	Truncated    bool            `json:"truncated"`
	Findings     []SecretFinding `json:"findings"`
	ScannedAt    time.Time       `json:"scanned_at"`
}

// SecretScanEnvResult is POST .../secret-scan/run's body.
type SecretScanEnvResult struct {
	Scope     string          `json:"scope"`
	Truncated bool            `json:"truncated"`
	Findings  []SecretFinding `json:"findings"`
	ScannedAt time.Time       `json:"scanned_at"`
}

func secretScanPath(name string) string { return "/api/v1/apps/" + PathEscape(name) + "/secret-scan" }

// GetSecretScan calls GET /api/v1/apps/{name}/secret-scan.
func (c *Client) GetSecretScan(ctx context.Context, name string) (SecretScanSettings, error) {
	var out SecretScanSettings
	err := c.do(ctx, http.MethodGet, secretScanPath(name), nil, &out)
	return out, err
}

// SetSecretScanPolicy calls PUT /api/v1/apps/{name}/secret-scan.
func (c *Client) SetSecretScanPolicy(ctx context.Context, name, policy string) (SecretScanSettings, error) {
	var out SecretScanSettings
	err := c.do(ctx, http.MethodPut, secretScanPath(name), map[string]string{"policy": policy}, &out)
	return out, err
}

// RunSecretScan calls POST /api/v1/apps/{name}/secret-scan/run.
func (c *Client) RunSecretScan(ctx context.Context, name string) (SecretScanEnvResult, error) {
	var out SecretScanEnvResult
	err := c.do(ctx, http.MethodPost, secretScanPath(name)+"/run", nil, &out)
	return out, err
}

// AllowSecretFingerprint calls POST /api/v1/apps/{name}/secret-scan/allow.
func (c *Client) AllowSecretFingerprint(ctx context.Context, name, fingerprint, reason string) (SecretScanAllowEntry, error) {
	var out SecretScanAllowEntry
	err := c.do(ctx, http.MethodPost, secretScanPath(name)+"/allow", map[string]string{"fingerprint": fingerprint, "reason": reason}, &out)
	return out, err
}

// UnallowSecretFingerprint calls DELETE /api/v1/apps/{name}/secret-scan/allow/{fingerprint}.
func (c *Client) UnallowSecretFingerprint(ctx context.Context, name, fingerprint string) error {
	return c.do(ctx, http.MethodDelete, secretScanPath(name)+"/allow/"+PathEscape(fingerprint), nil, nil)
}

// GetDeploymentSecretScan calls GET /api/v1/apps/{name}/deployments/{id}/secret-scan.
func (c *Client) GetDeploymentSecretScan(ctx context.Context, name, id string) (SecretScanRun, error) {
	var out SecretScanRun
	err := c.do(ctx, http.MethodGet, deploymentPath(name, id, "secret-scan"), nil, &out)
	return out, err
}
