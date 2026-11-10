package apiclient

import (
	"context"
	"net/http"
)

// ComposeDBServiceResource is one detected database service and its backup schedule.
type ComposeDBServiceResource struct {
	Service    string `json:"service"`
	Engine     string `json:"engine"`
	Image      string `json:"image"`
	Scheduled  bool   `json:"scheduled"`
	Schedule   string `json:"schedule,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	Retain     int    `json:"retain,omitempty"`
	RetainDays int    `json:"retain_days,omitempty"`
	Enabled    bool   `json:"enabled"`
}

// SetComposeDBBackupScheduleRequest is the body of the schedule PUT route.
type SetComposeDBBackupScheduleRequest struct {
	TargetID   string `json:"target_id"`
	Schedule   string `json:"schedule"`
	Retain     int    `json:"retain,omitempty"`
	RetainDays int    `json:"retain_days,omitempty"`
	Enabled    *bool  `json:"enabled,omitempty"`
}

// ComposeDBBackupRunResource is one backup run of a compose database service.
type ComposeDBBackupRunResource struct {
	ID         string `json:"id"`
	Service    string `json:"service"`
	TargetID   string `json:"target_id"`
	Status     string `json:"status"`
	ObjectKey  string `json:"object_key,omitempty"`
	SizeBytes  int64  `json:"size_bytes"`
	Checksum   string `json:"checksum,omitempty"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

func composeDBServicesPath(app string) string {
	return "/api/v1/apps/" + PathEscape(app) + "/compose-db-services"
}

func composeDBServicePath(app, service string) string {
	return composeDBServicesPath(app) + "/" + PathEscape(service)
}

// ListComposeDBServices calls GET /api/v1/apps/{name}/compose-db-services.
func (c *Client) ListComposeDBServices(ctx context.Context, app string) ([]ComposeDBServiceResource, error) {
	var out []ComposeDBServiceResource
	err := c.do(ctx, http.MethodGet, composeDBServicesPath(app), nil, &out)
	return out, err
}

// SetComposeDBBackupSchedule calls PUT .../compose-db-services/{service}/backup-schedule.
func (c *Client) SetComposeDBBackupSchedule(ctx context.Context, app, service string, req SetComposeDBBackupScheduleRequest) (ComposeDBServiceResource, error) {
	var out ComposeDBServiceResource
	err := c.do(ctx, http.MethodPut, composeDBServicePath(app, service)+"/backup-schedule", req, &out)
	return out, err
}

// DeleteComposeDBBackupSchedule calls DELETE .../compose-db-services/{service}/backup-schedule.
func (c *Client) DeleteComposeDBBackupSchedule(ctx context.Context, app, service string) error {
	return c.do(ctx, http.MethodDelete, composeDBServicePath(app, service)+"/backup-schedule", nil, nil)
}

// PauseComposeDBBackupSchedule calls POST .../backup-schedule/pause or /resume.
func (c *Client) PauseComposeDBBackupSchedule(ctx context.Context, app, service string, paused bool) (ComposeDBServiceResource, error) {
	verb := "resume"
	if paused {
		verb = "pause"
	}
	var out ComposeDBServiceResource
	err := c.do(ctx, http.MethodPost, composeDBServicePath(app, service)+"/backup-schedule/"+verb, nil, &out)
	return out, err
}

// ListComposeDBBackupRuns calls GET .../compose-db-services/{service}/backups.
func (c *Client) ListComposeDBBackupRuns(ctx context.Context, app, service string) ([]ComposeDBBackupRunResource, error) {
	var out []ComposeDBBackupRunResource
	err := c.do(ctx, http.MethodGet, composeDBServicePath(app, service)+"/backups", nil, &out)
	return out, err
}

// TriggerComposeDBBackup calls POST .../compose-db-services/{service}/backups.
func (c *Client) TriggerComposeDBBackup(ctx context.Context, app, service, targetID string) (ComposeDBBackupRunResource, error) {
	var out ComposeDBBackupRunResource
	body := struct {
		TargetID string `json:"target_id"`
	}{TargetID: targetID}
	err := c.do(ctx, http.MethodPost, composeDBServicePath(app, service)+"/backups", body, &out)
	return out, err
}
