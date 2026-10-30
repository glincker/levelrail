package apiclient

import (
	"context"
	"net/http"
	"strconv"
)

// BackupCopySettings mirrors internal/api's backupCopySettingsResource.
type BackupCopySettings struct {
	Enabled    bool   `json:"enabled"`
	TargetID   string `json:"target_id"`
	TargetName string `json:"target_name,omitempty"`
}

// BackupCopy mirrors internal/api's backupCopyResource.
type BackupCopy struct {
	BackupID       string `json:"backup_id"`
	Resource       string `json:"resource"`
	ResourceKind   string `json:"resource_kind"`
	SourceTargetID string `json:"source_target_id"`
	TargetID       string `json:"target_id"`
	Status         string `json:"status"`
	Attempts       int    `json:"attempts"`
	Error          string `json:"error,omitempty"`
	SizeBytes      int64  `json:"size_bytes"`
	BackupFinished string `json:"backup_finished_at"`
	CopiedAt       string `json:"copied_at,omitempty"`
	LagSeconds     int64  `json:"lag_seconds"`
}

// GetBackupCopySettings calls GET /api/v1/backups/copy-settings.
func (c *Client) GetBackupCopySettings(ctx context.Context) (BackupCopySettings, error) {
	var out BackupCopySettings
	err := c.do(ctx, http.MethodGet, "/api/v1/backups/copy-settings", nil, &out)
	return out, err
}

// SetBackupCopySettings calls PUT /api/v1/backups/copy-settings.
func (c *Client) SetBackupCopySettings(ctx context.Context, enabled bool, targetID string) (BackupCopySettings, error) {
	var out BackupCopySettings
	body := map[string]any{"enabled": enabled, "target_id": targetID}
	err := c.do(ctx, http.MethodPut, "/api/v1/backups/copy-settings", body, &out)
	return out, err
}

// ListBackupCopies calls GET /api/v1/backups/copies; limit 0 uses the server default.
func (c *Client) ListBackupCopies(ctx context.Context, limit int) ([]BackupCopy, error) {
	path := "/api/v1/backups/copies"
	if limit > 0 {
		path += "?limit=" + strconv.Itoa(limit)
	}
	var out []BackupCopy
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
