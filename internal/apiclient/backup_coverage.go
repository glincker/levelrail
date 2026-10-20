package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// CoverageRowResource mirrors internal/api's coverageRowResource.
type CoverageRowResource struct {
	Kind            string `json:"kind"`
	AppName         string `json:"app_name,omitempty"`
	Name            string `json:"name"`
	Scratch         bool   `json:"scratch,omitempty"`
	State           string `json:"state"`
	Reason          string `json:"reason,omitempty"`
	PrimaryAction   string `json:"primary_action"`
	Schedule        string `json:"schedule,omitempty"`
	TargetID        string `json:"target_id,omitempty"`
	NextRun         string `json:"next_run,omitempty"`
	LastBackupAt    string `json:"last_backup_at,omitempty"`
	LastVerifiedAt  string `json:"last_verified_at,omitempty"`
	TotalBytes      int64  `json:"total_bytes"`
	BackupCount     int    `json:"backup_count"`
	Encrypted       bool   `json:"encrypted"`
	ProtectionLevel string `json:"protection_level,omitempty"`
}

// CoverageSummaryResource mirrors internal/api's coverageSummaryResource.
type CoverageSummaryResource struct {
	Total               int    `json:"total"`
	Protected           int    `json:"protected"`
	Verified            int    `json:"verified"`
	Unverified          int    `json:"unverified"`
	NeverRan            int    `json:"never_ran"`
	Failing             int    `json:"failing"`
	Unprotected         int    `json:"unprotected"`
	Scratch             int    `json:"scratch"`
	StorageBytes        int64  `json:"storage_bytes"`
	LastVerifiedRestore string `json:"last_verified_restore,omitempty"`
	NextRun             string `json:"next_run,omitempty"`
}

// BackupDefaults mirrors internal/api's backupDefaults.
type BackupDefaults struct {
	Schedule   string `json:"schedule"`
	Retain     int    `json:"retain"`
	RetainDays int    `json:"retain_days"`
}

// BackupCoverageResponse mirrors internal/api's backupCoverageResponse.
type BackupCoverageResponse struct {
	Summary   CoverageSummaryResource `json:"summary"`
	Resources []CoverageRowResource   `json:"resources"`
	Targets   []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Provider string `json:"provider"`
	} `json:"targets"`
	Defaults BackupDefaults `json:"defaults"`
}

// ProtectItem names one resource in a protect-all plan.
type ProtectItem struct {
	Kind    string `json:"kind"`
	AppName string `json:"app_name,omitempty"`
	Name    string `json:"name"`
	Scratch bool   `json:"scratch,omitempty"`
}

// ProtectAllRequest is POST /api/v1/backups/protect-all's body.
type ProtectAllRequest struct {
	TargetID       string        `json:"target_id"`
	Schedule       string        `json:"schedule,omitempty"`
	Retain         *int          `json:"retain,omitempty"`
	RetainDays     *int          `json:"retain_days,omitempty"`
	IncludeScratch bool          `json:"include_scratch,omitempty"`
	Only           []ProtectItem `json:"only,omitempty"`
	Apply          bool          `json:"apply"`
}

// ProtectAllResponse mirrors internal/api's protectAllResponse.
type ProtectAllResponse struct {
	Applied    bool          `json:"applied"`
	TargetID   string        `json:"target_id"`
	Schedule   string        `json:"schedule"`
	Retain     int           `json:"retain"`
	RetainDays int           `json:"retain_days"`
	Plan       []ProtectItem `json:"plan"`
	Done       []ProtectItem `json:"done,omitempty"`
	Failed     []struct {
		ProtectItem
		Error string `json:"error"`
	} `json:"failed,omitempty"`
}

// StorageOrphanResource mirrors internal/api's storageOrphanResource.
type StorageOrphanResource struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SizeBytes  *int64 `json:"size_bytes,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	AgeDays    *int   `json:"age_days,omitempty"`
	BelongedTo struct {
		Kind    string `json:"kind"`
		AppName string `json:"app_name,omitempty"`
		Name    string `json:"name,omitempty"`
		Role    string `json:"role"`
	} `json:"belonged_to"`
	BackupCount int `json:"backup_count"`
	LastBackup  *struct {
		ID        string `json:"id"`
		At        string `json:"at"`
		SizeBytes int64  `json:"size_bytes"`
	} `json:"last_backup,omitempty"`
}

// StorageOrphansResponse mirrors internal/api's storageOrphansResponse.
type StorageOrphansResponse struct {
	Orphans        []StorageOrphanResource `json:"orphans"`
	TotalBytes     int64                   `json:"total_bytes"`
	ThresholdBytes int64                   `json:"threshold_bytes"`
}

// ReclaimOrphanResponse mirrors internal/api's reclaimOrphanResponse.
type ReclaimOrphanResponse struct {
	Removed        string `json:"removed"`
	ReclaimedBytes int64  `json:"reclaimed_bytes"`
}

// DeleteDatabaseAndData calls DELETE /api/v1/databases/{name} with
// delete_data=true: the data volumes are destroyed with the database. The
// server requires confirm to equal name.
func (c *Client) DeleteDatabaseAndData(ctx context.Context, name string, force bool) error {
	q := url.Values{"delete_data": {"true"}, "confirm": {name}}
	if force {
		q.Set("force", "true")
	}
	return c.do(ctx, http.MethodDelete, "/api/v1/databases/"+PathEscape(name)+"?"+q.Encode(), nil, nil)
}

// DeleteAppAndData is DeleteAppResult with delete_data=true: the app's
// volumes are destroyed once its containers are gone.
func (c *Client) DeleteAppAndData(ctx context.Context, name string) (DeleteAppResult, error) {
	var body struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	q := url.Values{"delete_data": {"true"}, "confirm": {name}}
	if err := c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"?"+q.Encode(), nil, &body); err != nil {
		return DeleteAppResult{}, err
	}
	return DeleteAppResult{Pending: body.Status == "teardown_pending", Message: body.Message, Error: body.Error}, nil
}

// BackupCoverage calls GET /api/v1/backups/coverage.
func (c *Client) BackupCoverage(ctx context.Context) (BackupCoverageResponse, error) {
	var out BackupCoverageResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/backups/coverage", nil, &out)
	return out, err
}

// ProtectAll calls POST /api/v1/backups/protect-all; Apply false only plans.
func (c *Client) ProtectAll(ctx context.Context, req ProtectAllRequest) (ProtectAllResponse, error) {
	var out ProtectAllResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/backups/protect-all", req, &out)
	return out, err
}

// ListStorageOrphans calls GET /api/v1/storage/orphans.
func (c *Client) ListStorageOrphans(ctx context.Context) (StorageOrphansResponse, error) {
	var out StorageOrphansResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/storage/orphans", nil, &out)
	return out, err
}

// ReclaimStorageOrphan calls DELETE /api/v1/storage/orphans/{id}; confirm
// must equal id.
func (c *Client) ReclaimStorageOrphan(ctx context.Context, id, confirm string) (ReclaimOrphanResponse, error) {
	var out ReclaimOrphanResponse
	body := map[string]string{"confirm": confirm}
	err := c.do(ctx, http.MethodDelete, "/api/v1/storage/orphans/"+url.PathEscape(id), body, &out)
	return out, err
}
