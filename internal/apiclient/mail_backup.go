package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// MailBackupJob is one running or finished mail backup or restore.
type MailBackupJob struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	State      string     `json:"state"`
	Step       string     `json:"step"`
	BackupID   string     `json:"backup_id,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Error      string     `json:"error,omitempty"`
}

// MailBackup is one stored mail backup.
type MailBackup struct {
	ID          string    `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	StoredBytes int64     `json:"stored_bytes"`
	DataBytes   int64     `json:"data_bytes"`
	Files       int       `json:"files"`
	MoxVersion  string    `json:"mox_version"`
	Verified    bool      `json:"verified"`
	Where       string    `json:"where"`
}

// MailBackups is the GET /api/v1/mail/backups response.
type MailBackups struct {
	App       string         `json:"app"`
	Encrypted bool           `json:"encrypted"`
	Folder    string         `json:"folder"`
	CapBytes  int64          `json:"cap_bytes"`
	UsedBytes int64          `json:"used_bytes"`
	Backups   []MailBackup   `json:"backups"`
	Job       *MailBackupJob `json:"job,omitempty"`
}

// MailRestoreResult is the restore response; DryRun means nothing was written.
type MailRestoreResult struct {
	App        string         `json:"app"`
	DryRun     bool           `json:"dry_run"`
	BackupID   string         `json:"backup_id"`
	MoxVersion string         `json:"mox_version"`
	Files      int            `json:"files"`
	Bytes      int64          `json:"bytes"`
	Job        *MailBackupJob `json:"job,omitempty"`
}

// MailStorageStep is one connection test step.
type MailStorageStep struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// MailStorage is the backup destination; it never carries the bucket secret.
type MailStorage struct {
	App    string `json:"app"`
	Kind   string `json:"kind"`
	Folder string `json:"folder"`
	Cap    int64  `json:"cap_bytes"`
	Bucket *struct {
		Endpoint  string `json:"endpoint,omitempty"`
		Region    string `json:"region,omitempty"`
		Bucket    string `json:"bucket"`
		PathStyle bool   `json:"path_style"`
	} `json:"bucket,omitempty"`
	Steps         []MailStorageStep `json:"steps"`
	OK            bool              `json:"ok"`
	Saved         bool              `json:"saved"`
	BucketUploads bool              `json:"bucket_uploads"`
}

// MailStorageInput sets the destination. SecretAccessKey is write-only.
type MailStorageInput struct {
	App             string `json:"app,omitempty"`
	Kind            string `json:"kind"`
	Endpoint        string `json:"endpoint,omitempty"`
	Region          string `json:"region,omitempty"`
	Bucket          string `json:"bucket,omitempty"`
	AccessKeyID     string `json:"access_key_id,omitempty"`
	SecretAccessKey string `json:"secret_access_key,omitempty"` //nolint:gosec // write-only request field
	PathStyle       bool   `json:"path_style,omitempty"`
}

func mailAppQuery(path, app string) string {
	if app != "" {
		return path + "?app=" + url.QueryEscape(app)
	}
	return path
}

// MailBackups calls GET /api/v1/mail/backups.
func (c *Client) MailBackups(ctx context.Context, app string) (MailBackups, error) {
	var out MailBackups
	err := c.do(ctx, http.MethodGet, mailAppQuery("/api/v1/mail/backups", app), nil, &out)
	return out, err
}

// RunMailBackup calls POST /api/v1/mail/backups and returns the started job.
func (c *Client) RunMailBackup(ctx context.Context, app string) (MailBackupJob, error) {
	var out MailBackupJob
	err := c.do(ctx, http.MethodPost, "/api/v1/mail/backups", map[string]string{"app": app}, &out)
	return out, err
}

// RestoreMailBackup calls POST /api/v1/mail/backups/{id}/restore. An empty
// confirm is a dry run; confirm equal to id restores for real.
func (c *Client) RestoreMailBackup(ctx context.Context, app, id, confirm string, proof *StepUpProof) (MailRestoreResult, error) {
	var out MailRestoreResult
	body := struct {
		App     string       `json:"app,omitempty"`
		Confirm string       `json:"confirm,omitempty"`
		StepUp  *StepUpProof `json:"step_up,omitempty"`
	}{app, confirm, proof}
	err := c.do(ctx, http.MethodPost, "/api/v1/mail/backups/"+url.PathEscape(id)+"/restore", body, &out)
	return out, err
}

// MailStorage calls GET /api/v1/mail/storage.
func (c *Client) MailStorage(ctx context.Context, app string) (MailStorage, error) {
	var out MailStorage
	err := c.do(ctx, http.MethodGet, mailAppQuery("/api/v1/mail/storage", app), nil, &out)
	return out, err
}

// SetMailStorage calls PUT /api/v1/mail/storage.
func (c *Client) SetMailStorage(ctx context.Context, in MailStorageInput) (MailStorage, error) {
	var out MailStorage
	err := c.do(ctx, http.MethodPut, "/api/v1/mail/storage", in, &out)
	return out, err
}

// TestMailStorage calls POST /api/v1/mail/storage/test.
func (c *Client) TestMailStorage(ctx context.Context, app string) (MailStorage, error) {
	var out MailStorage
	err := c.do(ctx, http.MethodPost, "/api/v1/mail/storage/test", map[string]string{"app": app}, &out)
	return out, err
}
