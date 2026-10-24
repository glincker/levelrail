package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DataCopySource locates the database to copy from. Password is sent in the
// request body only and is never stored by the control plane.
type DataCopySource struct {
	Host         string `json:"host"`
	Port         int    `json:"port,omitempty"`
	User         string `json:"user,omitempty"`
	Password     string `json:"password,omitempty"`
	Database     string `json:"database,omitempty"`
	AuthDatabase string `json:"auth_database,omitempty"`
	TLS          bool   `json:"tls,omitempty"`
}

// DataCopyTable is one table or collection compared across both sides.
type DataCopyTable struct {
	Name   string `json:"name"`
	Source int64  `json:"source"`
	Target int64  `json:"target"`
	OK     bool   `json:"ok"`
}

// DataCopyFailure is a failed copy in plain words, with the redacted raw
// error under Technical.
type DataCopyFailure struct {
	Code      string `json:"code"`
	Summary   string `json:"summary"`
	Hint      string `json:"hint"`
	Technical string `json:"technical,omitempty"`
}

// DataCopyStatus is one database's copy state.
type DataCopyStatus struct {
	Database       string           `json:"database"`
	Engine         string           `json:"engine"`
	Status         string           `json:"status"`
	Reason         string           `json:"reason,omitempty"`
	Failure        *DataCopyFailure `json:"failure,omitempty"`
	NextAction     string           `json:"next_action,omitempty"`
	SourceHost     string           `json:"source_host,omitempty"`
	SourcePort     int              `json:"source_port,omitempty"`
	SourceDatabase string           `json:"source_database,omitempty"`
	Checked        int              `json:"checked"`
	Mismatched     int              `json:"mismatched"`
	Tables         []DataCopyTable  `json:"tables,omitempty"`
	StartedAt      string           `json:"started_at,omitempty"`
	FinishedAt     string           `json:"finished_at,omitempty"`
}

// CutoverCheck is one named check with what to do about it.
type CutoverCheck struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// CutoverChange is the exact DNS edit to make.
type CutoverChange struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Value    string `json:"value"`
	TTL      uint32 `json:"ttl"`
	Replaces string `json:"replaces,omitempty"`
}

// CutoverDomain is the go or no-go for one domain.
type CutoverDomain struct {
	App     string         `json:"app"`
	Domain  string         `json:"domain"`
	Verdict string         `json:"verdict"`
	Checks  []CutoverCheck `json:"checks"`
	Change  *CutoverChange `json:"change,omitempty"`
}

// CutoverReport is the pre-switch or post-switch result.
type CutoverReport struct {
	Verdict   string          `json:"verdict"`
	Phase     string          `json:"phase"`
	TargetIPs []string        `json:"target_ips"`
	Domains   []CutoverDomain `json:"domains"`
	Guidance  []string        `json:"guidance"`
}

// VolumeGuide is a command to run on the target node to copy one volume.
type VolumeGuide struct {
	App           string `json:"app"`
	Kind          string `json:"kind"`
	Source        string `json:"source"`
	Target        string `json:"target"`
	ContainerPath string `json:"container_path"`
	Command       string `json:"command"`
}

// VolumeGuideResponse is the volume copy guide.
type VolumeGuideResponse struct {
	Source  string        `json:"source"`
	Guides  []VolumeGuide `json:"guides"`
	Warning string        `json:"warning"`
}

// StartDatabaseDataCopy calls POST /api/v1/imports/platform/databases/{name}/copy.
func (c *Client) StartDatabaseDataCopy(ctx context.Context, name string, src DataCopySource) (DataCopyStatus, error) {
	var out DataCopyStatus
	err := c.do(ctx, http.MethodPost, "/api/v1/imports/platform/databases/"+url.PathEscape(name)+"/copy", src, &out)
	return out, err
}

// HostedImportRequest is the body of the hosted Postgres routes. URL is a
// credential, send it only over the API and never print it.
type HostedImportRequest struct {
	URL          string `json:"url"`
	WritesFrozen bool   `json:"writes_frozen"`
	AllowLarge   bool   `json:"allow_large"`
}

// HostedCheck is one preflight finding with its fix.
type HostedCheck struct {
	ID         string `json:"id"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	NextAction string `json:"next_action,omitempty"`
}

// HostedPreflight is the findings for a hosted Postgres move. Source is redacted.
type HostedPreflight struct {
	Database        string        `json:"database"`
	Source          string        `json:"source"`
	Blocked         bool          `json:"blocked"`
	TargetVersion   string        `json:"target_version"`
	RequiredBytes   int64         `json:"required_bytes"`
	EstimateSeconds int           `json:"estimate_seconds"`
	Checks          []HostedCheck `json:"checks"`
}

// PreflightHostedImport calls POST /api/v1/imports/platform/databases/{name}/hosted/preflight.
func (c *Client) PreflightHostedImport(ctx context.Context, name string, req HostedImportRequest) (HostedPreflight, error) {
	var out HostedPreflight
	err := c.do(ctx, http.MethodPost, "/api/v1/imports/platform/databases/"+url.PathEscape(name)+"/hosted/preflight", req, &out)
	return out, err
}

// StartHostedImport calls POST /api/v1/imports/platform/databases/{name}/hosted/run.
// Read progress with GetDatabaseDataCopy.
func (c *Client) StartHostedImport(ctx context.Context, name string, req HostedImportRequest) (DataCopyStatus, error) {
	var out DataCopyStatus
	err := c.do(ctx, http.MethodPost, "/api/v1/imports/platform/databases/"+url.PathEscape(name)+"/hosted/run", req, &out)
	return out, err
}

// GetDatabaseDataCopy calls GET /api/v1/imports/platform/databases/{name}.
func (c *Client) GetDatabaseDataCopy(ctx context.Context, name string) (DataCopyStatus, error) {
	var out DataCopyStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/imports/platform/databases/"+url.PathEscape(name), nil, &out)
	return out, err
}

// ListDatabaseDataCopies calls GET /api/v1/imports/platform/databases.
func (c *Client) ListDatabaseDataCopies(ctx context.Context) ([]DataCopyStatus, error) {
	var out []DataCopyStatus
	err := c.do(ctx, http.MethodGet, "/api/v1/imports/platform/databases", nil, &out)
	return out, err
}

// CutoverReport calls GET /api/v1/migration/cutover, or its /verify form.
func (c *Client) CutoverReport(ctx context.Context, app, targetIP string, verify bool) (CutoverReport, error) {
	q := url.Values{}
	if app != "" {
		q.Set("app", app)
	}
	if targetIP != "" {
		q.Set("target_ip", targetIP)
	}
	path := "/api/v1/migration/cutover"
	if verify {
		path += "/verify"
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out CutoverReport
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// MigrationVolumeGuide calls GET /api/v1/migration/volumes.
func (c *Client) MigrationVolumeGuide(ctx context.Context, source, app string) (VolumeGuideResponse, error) {
	q := url.Values{"source": {source}}
	if app != "" {
		q.Set("app", app)
	}
	var out VolumeGuideResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/migration/volumes?"+q.Encode(), nil, &out)
	return out, err
}
