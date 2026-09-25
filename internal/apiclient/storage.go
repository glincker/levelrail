package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// StorageProvider is one provider preset from GET /api/v1/storage/providers.
type StorageProvider struct {
	ID               string `json:"id"`
	Label            string `json:"label"`
	EndpointTemplate string `json:"endpoint_template,omitempty"`
	DefaultRegion    string `json:"default_region,omitempty"`
	PathStyle        bool   `json:"path_style"`
	NeedsAccountID   bool   `json:"needs_account_id"`
	NeedsRegion      bool   `json:"needs_region"`
	NeedsEndpoint    bool   `json:"needs_endpoint"`
}

// StorageDestination is a connected S3-compatible bucket. It never carries credentials.
type StorageDestination struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Preset          string `json:"preset"`
	Provider        string `json:"provider"`
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	Bucket          string `json:"bucket"`
	PathStyle       bool   `json:"path_style"`
	AccountID       string `json:"account_id,omitempty"`
	CreatedAt       string `json:"created_at"`
	ArchivePolicies int    `json:"archive_policies"`
}

// StorageDestinationRequest creates or updates a destination.
type StorageDestinationRequest struct {
	Name            string `json:"name"`
	Preset          string `json:"preset"`
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	Bucket          string `json:"bucket"`
	AccountID       string `json:"account_id,omitempty"`
	PathStyle       *bool  `json:"path_style,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"`
	Verify          bool   `json:"verify,omitempty"`
}

// StorageProbeStep is one step of a connection probe.
type StorageProbeStep struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// StorageProbeResult is the outcome of a put/get/delete probe.
type StorageProbeResult struct {
	OK      bool               `json:"ok"`
	Reason  string             `json:"reason,omitempty"`
	Message string             `json:"message,omitempty"`
	Steps   []StorageProbeStep `json:"steps"`
}

// LogArchivePolicy is a log archive schedule for one app, or all apps when AppName is empty.
type LogArchivePolicy struct {
	ID              string `json:"id"`
	AppName         string `json:"app_name"`
	TargetID        string `json:"target_id"`
	Enabled         bool   `json:"enabled"`
	Interval        string `json:"interval"`
	IntervalSeconds int64  `json:"interval_seconds"`
	RetentionDays   int    `json:"retention_days"`
	LastRunAt       string `json:"last_run_at,omitempty"`
	LastSuccessAt   string `json:"last_success_at,omitempty"`
	LastError       string `json:"last_error,omitempty"`
	CreatedAt       string `json:"created_at"`
}

// LogArchivePolicyRequest sets a policy.
type LogArchivePolicyRequest struct {
	AppName       string `json:"app_name"`
	TargetID      string `json:"target_id"`
	Interval      string `json:"interval,omitempty"`
	RetentionDays int    `json:"retention_days"`
	Enabled       *bool  `json:"enabled,omitempty"`
}

// LogArchiveRun is one archive attempt.
type LogArchiveRun struct {
	ID         string `json:"id"`
	AppName    string `json:"app_name"`
	TargetID   string `json:"target_id"`
	Kind       string `json:"kind"`
	From       string `json:"from"`
	To         string `json:"to"`
	Status     string `json:"status"`
	Objects    int    `json:"objects"`
	Lines      int64  `json:"lines"`
	Bytes      int64  `json:"bytes"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

// LogArchiveDumpRequest exports a time range now.
type LogArchiveDumpRequest struct {
	AppName  string `json:"app_name"`
	TargetID string `json:"target_id"`
	From     string `json:"from"`
	To       string `json:"to"`
}

// LogArchiveObject is one archived object.
type LogArchiveObject struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	LastModified string `json:"last_modified"`
}

// LogArchiveObjects is one page of archived objects.
type LogArchiveObjects struct {
	Objects []LogArchiveObject `json:"objects"`
	Next    string             `json:"next,omitempty"`
}

func storageDestinationPath(id string) string {
	return "/api/v1/storage/destinations/" + PathEscape(id)
}

// ListStorageProviders calls GET /api/v1/storage/providers.
func (c *Client) ListStorageProviders(ctx context.Context) ([]StorageProvider, error) {
	var out []StorageProvider
	err := c.do(ctx, http.MethodGet, "/api/v1/storage/providers", nil, &out)
	return out, err
}

// ListStorageDestinations calls GET /api/v1/storage/destinations.
func (c *Client) ListStorageDestinations(ctx context.Context) ([]StorageDestination, error) {
	var out []StorageDestination
	err := c.do(ctx, http.MethodGet, "/api/v1/storage/destinations", nil, &out)
	return out, err
}

// CreateStorageDestination calls POST /api/v1/storage/destinations.
func (c *Client) CreateStorageDestination(ctx context.Context, req StorageDestinationRequest) (StorageDestination, error) {
	var out StorageDestination
	err := c.do(ctx, http.MethodPost, "/api/v1/storage/destinations", req, &out)
	return out, err
}

// DeleteStorageDestination calls DELETE /api/v1/storage/destinations/{id}.
func (c *Client) DeleteStorageDestination(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, storageDestinationPath(id), nil, nil)
}

// TestStorageDestination calls POST /api/v1/storage/destinations/{id}/test.
func (c *Client) TestStorageDestination(ctx context.Context, id string) (StorageProbeResult, error) {
	var out StorageProbeResult
	err := c.do(ctx, http.MethodPost, storageDestinationPath(id)+"/test", nil, &out)
	return out, err
}

// ListLogArchivePolicies calls GET /api/v1/log-archive/policies.
func (c *Client) ListLogArchivePolicies(ctx context.Context) ([]LogArchivePolicy, error) {
	var out []LogArchivePolicy
	err := c.do(ctx, http.MethodGet, "/api/v1/log-archive/policies", nil, &out)
	return out, err
}

// SetLogArchivePolicy calls PUT /api/v1/log-archive/policy.
func (c *Client) SetLogArchivePolicy(ctx context.Context, req LogArchivePolicyRequest) (LogArchivePolicy, error) {
	var out LogArchivePolicy
	err := c.do(ctx, http.MethodPut, "/api/v1/log-archive/policy", req, &out)
	return out, err
}

// DeleteLogArchivePolicy calls DELETE /api/v1/log-archive/policy. An empty app is the global policy.
func (c *Client) DeleteLogArchivePolicy(ctx context.Context, app string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/log-archive/policy?app="+url.QueryEscape(app), nil, nil)
}

// StartLogArchiveDump calls POST /api/v1/log-archive/dump.
func (c *Client) StartLogArchiveDump(ctx context.Context, req LogArchiveDumpRequest) (LogArchiveRun, error) {
	var out LogArchiveRun
	err := c.do(ctx, http.MethodPost, "/api/v1/log-archive/dump", req, &out)
	return out, err
}

// ListLogArchiveRuns calls GET /api/v1/log-archive/runs. all ignores app.
func (c *Client) ListLogArchiveRuns(ctx context.Context, app string, all bool) ([]LogArchiveRun, error) {
	q := url.Values{"app": {app}}
	if all {
		q.Set("all", "true")
	}
	var out []LogArchiveRun
	err := c.do(ctx, http.MethodGet, "/api/v1/log-archive/runs?"+q.Encode(), nil, &out)
	return out, err
}

// ListLogArchiveObjects calls GET /api/v1/log-archive/objects.
func (c *Client) ListLogArchiveObjects(ctx context.Context, targetID, app, cursor string) (LogArchiveObjects, error) {
	q := url.Values{"target_id": {targetID}}
	if app != "" {
		q.Set("app", app)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	var out LogArchiveObjects
	err := c.do(ctx, http.MethodGet, "/api/v1/log-archive/objects?"+q.Encode(), nil, &out)
	return out, err
}

// DownloadLogArchiveObject calls GET /api/v1/log-archive/objects/download and returns the gzip bytes.
func (c *Client) DownloadLogArchiveObject(ctx context.Context, targetID, key string) ([]byte, error) {
	q := url.Values{"target_id": {targetID}, "key": {key}}
	return c.downloadRaw(ctx, "/api/v1/log-archive/objects/download?"+q.Encode())
}
