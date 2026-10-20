package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// BackupSummaryResource mirrors internal/api's backupSummaryResource.
type BackupSummaryResource struct {
	ID        string `json:"id"`
	At        string `json:"at"`
	Status    string `json:"status"`
	SizeBytes int64  `json:"size_bytes"`
	Codec     string `json:"codec,omitempty"`
	Error     string `json:"error,omitempty"`
}

// DrillSummaryResource mirrors internal/api's drillSummaryResource.
type DrillSummaryResource struct {
	ID      string `json:"id"`
	At      string `json:"at"`
	Status  string `json:"status"`
	Stage   string `json:"stage,omitempty"`
	Error   string `json:"error,omitempty"`
	Trigger string `json:"trigger,omitempty"`
	Files   int64  `json:"files,omitempty"`
}

// ResourceHealthResource mirrors internal/api's resourceHealthResource.
type ResourceHealthResource struct {
	Kind                string                 `json:"kind"`
	AppName             string                 `json:"app_name,omitempty"`
	ResourceName        string                 `json:"resource_name"`
	TargetID            string                 `json:"target_id,omitempty"`
	Schedule            string                 `json:"schedule,omitempty"`
	NextRun             string                 `json:"next_run,omitempty"`
	BackupCount         int                    `json:"backup_count"`
	TotalBytes          int64                  `json:"total_bytes"`
	LastAttempt         *BackupSummaryResource `json:"last_attempt,omitempty"`
	LastBackup          *BackupSummaryResource `json:"last_backup,omitempty"`
	LastVerifiedRestore *DrillSummaryResource  `json:"last_verified_restore,omitempty"`
	LastDrill           *DrillSummaryResource  `json:"last_drill,omitempty"`
	Encrypted           bool                   `json:"encrypted"`
	State               string                 `json:"state"`
	StateReason         string                 `json:"state_reason,omitempty"`
	ProtectionLevel     string                 `json:"protection_level,omitempty"`
	Warning             string                 `json:"warning,omitempty"`
}

// TargetProtectionResource mirrors internal/api's targetProtectionResource.
type TargetProtectionResource struct {
	TargetID   string `json:"target_id"`
	Level      string `json:"level"`
	ObjectLock bool   `json:"object_lock"`
	LockMode   string `json:"lock_mode,omitempty"`
	Versioning string `json:"versioning,omitempty"`
	CanDelete  bool   `json:"can_delete"`
	Warning    string `json:"warning,omitempty"`
	ProbeError string `json:"probe_error,omitempty"`
	CheckedAt  string `json:"checked_at"`
}

// BackupHealthResponse mirrors internal/api's backupHealthResponse.
type BackupHealthResponse struct {
	Resources  []ResourceHealthResource   `json:"resources"`
	Targets    []TargetProtectionResource `json:"targets"`
	Encryption struct {
		Enabled   bool   `json:"enabled"`
		Recipient string `json:"recipient,omitempty"`
		Codec     string `json:"codec"`
	} `json:"encryption"`
}

// BackupDrillResource mirrors internal/api's drillResource.
type BackupDrillResource struct {
	ID           string `json:"id"`
	BackupID     string `json:"backup_id"`
	ResourceKind string `json:"resource_kind"`
	DatabaseName string `json:"database_name,omitempty"`
	ServiceName  string `json:"service_name,omitempty"`
	VolumeName   string `json:"volume_name,omitempty"`
	Trigger      string `json:"trigger"`
	Status       string `json:"status"`
	Stage        string `json:"stage"`
	ObjectOK     bool   `json:"object_ok"`
	ChecksumOK   bool   `json:"checksum_ok"`
	RestoreOK    bool   `json:"restore_ok"`
	ContentOK    bool   `json:"content_ok"`
	Files        int64  `json:"files"`
	Bytes        int64  `json:"bytes"`
	DurationMS   int64  `json:"duration_ms"`
	Error        string `json:"error,omitempty"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at,omitempty"`
}

// VolumeBackupPolicyResource mirrors internal/api's volumeBackupPolicyResource.
type VolumeBackupPolicyResource struct {
	ServiceName   string `json:"service_name"`
	VolumeName    string `json:"volume_name"`
	RetainDaily   int    `json:"retain_daily"`
	RetainWeekly  int    `json:"retain_weekly"`
	RetainMonthly int    `json:"retain_monthly"`
	PreHook       string `json:"pre_hook"`
	PostHook      string `json:"post_hook"`
	Quiesce       string `json:"quiesce"`
}

// VolumeRestoreToRequest is POST .../volumes/{volume}/restore-to's body.
type VolumeRestoreToRequest struct {
	BackupID      string `json:"backup_id"`
	NewVolumeName string `json:"new_volume_name,omitempty"`
	TargetApp     string `json:"target_app,omitempty"`
	NodeID        string `json:"node_id,omitempty"`
}

// VolumeRestoreToResponse is the 202 body of restore-to.
type VolumeRestoreToResponse struct {
	ID            string `json:"id"`
	NewVolumeName string `json:"new_volume_name"`
	NodeID        string `json:"node_id"`
}

// BackupHealth calls GET /api/v1/backups/health.
func (c *Client) BackupHealth(ctx context.Context) (BackupHealthResponse, error) {
	var out BackupHealthResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/backups/health", nil, &out)
	return out, err
}

// ListBackupDrills calls GET /api/v1/backups/drills.
func (c *Client) ListBackupDrills(ctx context.Context, service, volume, database string, limit int) ([]BackupDrillResource, error) {
	q := url.Values{}
	if service != "" {
		q.Set("service", service)
	}
	if volume != "" {
		q.Set("volume", volume)
	}
	if database != "" {
		q.Set("database", database)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/v1/backups/drills"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out []BackupDrillResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetBackupDrill calls GET /api/v1/backups/drills/{id}.
func (c *Client) GetBackupDrill(ctx context.Context, id string) (BackupDrillResource, error) {
	var out BackupDrillResource
	err := c.do(ctx, http.MethodGet, "/api/v1/backups/drills/"+PathEscape(id), nil, &out)
	return out, err
}

// StartBackupDrill calls POST /api/v1/backups/drills and returns the drill ID.
func (c *Client) StartBackupDrill(ctx context.Context, backupID string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	err := c.do(ctx, http.MethodPost, "/api/v1/backups/drills", map[string]string{"backup_id": backupID}, &out)
	return out.ID, err
}

// RefreshBackupProtection calls POST /api/v1/backups/protection/refresh.
func (c *Client) RefreshBackupProtection(ctx context.Context, targetIDs []string) ([]TargetProtectionResource, error) {
	var out []TargetProtectionResource
	err := c.do(ctx, http.MethodPost, "/api/v1/backups/protection/refresh", map[string][]string{"target_ids": targetIDs}, &out)
	return out, err
}

// GetVolumeBackupPolicy calls GET /api/v1/apps/{name}/volumes/{volume}/backup-policy.
func (c *Client) GetVolumeBackupPolicy(ctx context.Context, name, volume string) (VolumeBackupPolicyResource, error) {
	var out VolumeBackupPolicyResource
	err := c.do(ctx, http.MethodGet, volumePolicyPath(name, volume), nil, &out)
	return out, err
}

// SetVolumeBackupPolicy calls PUT /api/v1/apps/{name}/volumes/{volume}/backup-policy.
func (c *Client) SetVolumeBackupPolicy(ctx context.Context, name, volume string, p VolumeBackupPolicyResource) (VolumeBackupPolicyResource, error) {
	var out VolumeBackupPolicyResource
	err := c.do(ctx, http.MethodPut, volumePolicyPath(name, volume), p, &out)
	return out, err
}

// RestoreVolumeTo calls POST /api/v1/apps/{name}/volumes/{volume}/restore-to.
func (c *Client) RestoreVolumeTo(ctx context.Context, name, volume string, req VolumeRestoreToRequest) (VolumeRestoreToResponse, error) {
	var out VolumeRestoreToResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/volumes/"+PathEscape(volume)+"/restore-to", req, &out)
	return out, err
}

func volumePolicyPath(name, volume string) string {
	return "/api/v1/apps/" + PathEscape(name) + "/volumes/" + PathEscape(volume) + "/backup-policy"
}
