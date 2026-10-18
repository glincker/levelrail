package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// ArchiveBundle mirrors internal/archive.Bundle.
type ArchiveBundle struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	CreatedAt  string         `json:"created_at"`
	Cutoff     string         `json:"cutoff"`
	Rows       map[string]int `json:"rows"`
	TotalRows  int            `json:"total_rows"`
	SizeBytes  int64          `json:"size_bytes"`
	SHA256     string         `json:"sha256"`
	VerifiedAt string         `json:"verified_at,omitempty"`
	VerifiedOK *bool          `json:"verified_ok,omitempty"`
	RestoredAt string         `json:"restored_at,omitempty"`
}

// ArchivePolicy mirrors internal/store.ArchiveSettings.
type ArchivePolicy struct {
	Enabled         bool   `json:"enabled"`
	AuditHotDays    int    `json:"audit_hot_days"`
	DeployHotDays   int    `json:"deploy_hot_days"`
	PipelineHotDays int    `json:"pipeline_hot_days"`
	KeepDays        int    `json:"keep_days"`
	LastRunAt       string `json:"last_run_at,omitempty"`
	LastRunError    string `json:"last_run_error,omitempty"`
}

// ArchiveList mirrors GET /api/v1/archives.
type ArchiveList struct {
	Bundles   []ArchiveBundle `json:"bundles"`
	Policy    ArchivePolicy   `json:"policy"`
	HotRows   map[string]int  `json:"hot_rows"`
	TotalSize int64           `json:"total_size_bytes"`
}

// ArchiveRunResult mirrors internal/archive.RunResult.
type ArchiveRunResult struct {
	Created []ArchiveBundle `json:"created"`
	Pruned  int             `json:"pruned"`
}

// ArchiveRestoreResult mirrors internal/archive.RestoreResult.
type ArchiveRestoreResult struct {
	Restored map[string]int `json:"restored"`
	Existing int            `json:"already_present"`
	Skipped  int            `json:"skipped_missing_parent"`
}

// ArchiveVerifyResult is the verify-by-restore outcome.
type ArchiveVerifyResult struct {
	Bundle ArchiveBundle `json:"bundle"`
	OK     bool          `json:"ok"`
	Error  string        `json:"error,omitempty"`
}

// ArchiveRows mirrors internal/archive.Rows.
type ArchiveRows struct {
	Table string           `json:"table"`
	Total int              `json:"total"`
	Rows  []map[string]any `json:"rows"`
}

// ListArchives calls GET /api/v1/archives.
func (c *Client) ListArchives(ctx context.Context) (ArchiveList, error) {
	var out ArchiveList
	err := c.do(ctx, http.MethodGet, "/api/v1/archives", nil, &out)
	return out, err
}

// CreateArchive calls POST /api/v1/archives. An empty kind runs the whole policy.
func (c *Client) CreateArchive(ctx context.Context, kind string, olderThanDays int) (ArchiveRunResult, error) {
	var out ArchiveRunResult
	body := map[string]any{"kind": kind, "older_than_days": olderThanDays}
	err := c.do(ctx, http.MethodPost, "/api/v1/archives", body, &out)
	return out, err
}

// RestoreArchive calls POST /api/v1/archives/{id}/restore.
func (c *Client) RestoreArchive(ctx context.Context, id string) (ArchiveRestoreResult, error) {
	var out ArchiveRestoreResult
	err := c.do(ctx, http.MethodPost, "/api/v1/archives/"+PathEscape(id)+"/restore", nil, &out)
	return out, err
}

// VerifyArchive calls POST /api/v1/archives/{id}/verify.
func (c *Client) VerifyArchive(ctx context.Context, id string) (ArchiveVerifyResult, error) {
	var out ArchiveVerifyResult
	err := c.do(ctx, http.MethodPost, "/api/v1/archives/"+PathEscape(id)+"/verify", nil, &out)
	return out, err
}

// DeleteArchive calls DELETE /api/v1/archives/{id}.
func (c *Client) DeleteArchive(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/archives/"+PathEscape(id), nil, nil)
}

// DownloadArchive calls GET /api/v1/archives/{id}/download and returns the zstd bundle.
func (c *Client) DownloadArchive(ctx context.Context, id string) ([]byte, error) {
	return c.downloadRaw(ctx, "/api/v1/archives/"+PathEscape(id)+"/download")
}

// BrowseArchive calls GET /api/v1/archives/{id}/rows.
func (c *Client) BrowseArchive(ctx context.Context, id, table string, offset, limit int) (ArchiveRows, error) {
	var out ArchiveRows
	q := url.Values{"table": {table}, "offset": {strconv.Itoa(offset)}, "limit": {strconv.Itoa(limit)}}
	err := c.do(ctx, http.MethodGet, "/api/v1/archives/"+PathEscape(id)+"/rows?"+q.Encode(), nil, &out)
	return out, err
}

// GetArchivePolicy calls GET /api/v1/archives/policy.
func (c *Client) GetArchivePolicy(ctx context.Context) (ArchivePolicy, error) {
	var out ArchivePolicy
	err := c.do(ctx, http.MethodGet, "/api/v1/archives/policy", nil, &out)
	return out, err
}

// SetArchivePolicy calls PUT /api/v1/archives/policy.
func (c *Client) SetArchivePolicy(ctx context.Context, p ArchivePolicy) (ArchivePolicy, error) {
	var out ArchivePolicy
	err := c.do(ctx, http.MethodPut, "/api/v1/archives/policy", p, &out)
	return out, err
}
