package apiclient

import (
	"context"
	"net/http"
)

// BackupTargetDrill is the result of POST /api/v1/backup-targets/{id}/drill.
type BackupTargetDrill struct {
	OK         bool   `json:"ok"`
	Stage      string `json:"stage,omitempty"`
	Bytes      int    `json:"bytes"`
	UploadMs   int64  `json:"upload_ms"`
	DownloadMs int64  `json:"download_ms"`
	CleanedUp  bool   `json:"cleaned_up"`
	ChecksumOK bool   `json:"checksum_ok"`
	Error      string `json:"error,omitempty"`
}

// DrillBackupTarget calls POST /api/v1/backup-targets/{id}/drill.
func (c *Client) DrillBackupTarget(ctx context.Context, id string) (BackupTargetDrill, error) {
	var out BackupTargetDrill
	err := c.do(ctx, http.MethodPost, backupTargetPath(id)+"/drill", nil, &out)
	return out, err
}
