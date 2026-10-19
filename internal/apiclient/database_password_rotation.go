package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// DatabasePasswordRotation mirrors internal/api's databasePasswordRotationResource.
type DatabasePasswordRotation struct {
	Database        string   `json:"database"`
	Engine          string   `json:"engine"`
	Supported       bool     `json:"supported"`
	Reason          string   `json:"reason,omitempty"`
	RestartRequired bool     `json:"restart_required"`
	Apps            []string `json:"apps"`
	SuspendedApps   []string `json:"suspended_apps"`
	State           string   `json:"state"`
	LastError       string   `json:"last_error,omitempty"`
	LastChangedAt   string   `json:"last_changed_at,omitempty"`
	AgeDays         *int     `json:"age_days,omitempty"`
	IntervalDays    int      `json:"interval_days"`
	NextDue         string   `json:"next_due,omitempty"`
	RestartedApps   []string `json:"restarted_apps,omitempty"`
	RotatedAt       string   `json:"rotated_at,omitempty"`
}

type rotateDatabasePasswordRequest struct {
	DryRun bool         `json:"dry_run,omitempty"`
	StepUp *StepUpProof `json:"step_up,omitempty"`
}

func databasePasswordRotationPath(name string) string {
	return "/api/v1/databases/" + url.PathEscape(name) + "/password-rotation"
}

// GetDatabasePasswordRotation calls GET /api/v1/databases/{name}/password-rotation.
func (c *Client) GetDatabasePasswordRotation(ctx context.Context, name string) (DatabasePasswordRotation, error) {
	var out DatabasePasswordRotation
	err := c.do(ctx, http.MethodGet, databasePasswordRotationPath(name), nil, &out)
	return out, err
}

// PreviewDatabasePasswordRotation calls the rotation endpoint with dry_run,
// which changes nothing and needs no step-up.
func (c *Client) PreviewDatabasePasswordRotation(ctx context.Context, name string) (DatabasePasswordRotation, error) {
	var out DatabasePasswordRotation
	err := c.do(ctx, http.MethodPost, databasePasswordRotationPath(name), rotateDatabasePasswordRequest{DryRun: true}, &out)
	return out, err
}

// RotateDatabasePassword calls POST /api/v1/databases/{name}/password-rotation.
// It needs the token owner's step-up proof.
func (c *Client) RotateDatabasePassword(ctx context.Context, name string, proof *StepUpProof) (DatabasePasswordRotation, error) {
	var out DatabasePasswordRotation
	err := c.do(ctx, http.MethodPost, databasePasswordRotationPath(name), rotateDatabasePasswordRequest{StepUp: proof}, &out)
	return out, err
}

// SetDatabasePasswordRotationInterval calls PUT .../password-rotation/policy; 0 turns the schedule off.
func (c *Client) SetDatabasePasswordRotationInterval(ctx context.Context, name string, days int) (DatabasePasswordRotation, error) {
	var out DatabasePasswordRotation
	err := c.do(ctx, http.MethodPut, databasePasswordRotationPath(name)+"/policy", map[string]int{"interval_days": days}, &out)
	return out, err
}
