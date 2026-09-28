package apiclient

import (
	"context"
	"net/http"
)

// ControlPlaneDRWarning mirrors internal/cpbackup.Warning.
type ControlPlaneDRWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ControlPlaneDRDrill mirrors internal/cpbackup.DrillStatus.
type ControlPlaneDRDrill struct {
	At         string `json:"at,omitempty"`
	OK         bool   `json:"ok"`
	Partial    bool   `json:"partial"`
	Detail     string `json:"detail"`
	DurationMs int64  `json:"duration_ms"`
}

// ControlPlaneDRChecklist mirrors internal/cpbackup.Checklist.
type ControlPlaneDRChecklist struct {
	DestinationChosen  bool `json:"destination_chosen"`
	RecipientSet       bool `json:"recipient_set"`
	EscrowAcknowledged bool `json:"escrow_acknowledged"`
	DrillPassed        bool `json:"drill_passed"`
}

// ControlPlaneDR mirrors internal/cpbackup.Status: off-box backup
// configuration and health. It carries public recipients only.
type ControlPlaneDR struct {
	Enabled                 bool                    `json:"enabled"`
	Configured              bool                    `json:"configured"`
	TargetID                string                  `json:"target_id"`
	TargetName              string                  `json:"target_name,omitempty"`
	InstallID               string                  `json:"install_id"`
	Recipients              []string                `json:"recipients"`
	Schedule                string                  `json:"schedule"`
	DrillSchedule           string                  `json:"drill_schedule"`
	RetainDaily             int                     `json:"retain_daily"`
	RetainWeekly            int                     `json:"retain_weekly"`
	RetainMonthly           int                     `json:"retain_monthly"`
	EscrowTargetID          string                  `json:"escrow_target_id"`
	EscrowGeneratedAt       string                  `json:"escrow_generated_at,omitempty"`
	EscrowAckedAt           string                  `json:"escrow_acked_at,omitempty"`
	NextBackupAt            string                  `json:"next_backup_at,omitempty"`
	NextDrillAt             string                  `json:"next_drill_at,omitempty"`
	LastBackupAt            string                  `json:"last_backup_at,omitempty"`
	LastBackupKey           string                  `json:"last_backup_key,omitempty"`
	LastBackupError         string                  `json:"last_backup_error,omitempty"`
	LastAttemptAt           string                  `json:"last_attempt_at,omitempty"`
	LastDrill               ControlPlaneDRDrill     `json:"last_drill"`
	DrillIdentityConfigured bool                    `json:"drill_identity_configured"`
	BackupRunning           bool                    `json:"backup_running"`
	DrillRunning            bool                    `json:"drill_running"`
	Checklist               ControlPlaneDRChecklist `json:"checklist"`
	Warnings                []ControlPlaneDRWarning `json:"warnings"`
}

// ControlPlaneDRSettings is the body of PUT /system/control-plane-dr/settings.
type ControlPlaneDRSettings struct {
	Enabled        bool     `json:"enabled"`
	TargetID       string   `json:"target_id"`
	Recipients     []string `json:"recipients"`
	Schedule       string   `json:"schedule"`
	DrillSchedule  string   `json:"drill_schedule"`
	RetainDaily    int      `json:"retain_daily"`
	RetainWeekly   int      `json:"retain_weekly"`
	RetainMonthly  int      `json:"retain_monthly"`
	EscrowTargetID string   `json:"escrow_target_id"`
}

// ControlPlaneOffboxBackup mirrors internal/cpbackup.Remote.
type ControlPlaneOffboxBackup struct {
	Key         string `json:"key"`
	ManifestKey string `json:"manifest_key"`
	CreatedAt   string `json:"created_at"`
	SizeBytes   int64  `json:"size_bytes"`
	Complete    bool   `json:"complete"`
}

// ControlPlaneEscrowBundle mirrors internal/cpbackup.EscrowBundle.
type ControlPlaneEscrowBundle struct {
	Armored        string `json:"armored"`
	Instructions   string `json:"instructions"`
	Fingerprint    string `json:"fingerprint"`
	RecipientCount int    `json:"recipient_count"`
	CreatedAt      string `json:"created_at"`
	UploadedKey    string `json:"uploaded_key,omitempty"`
}

const controlPlaneDRPath = "/api/v1/system/control-plane-dr"

// GetControlPlaneDR calls GET /api/v1/system/control-plane-dr.
func (c *Client) GetControlPlaneDR(ctx context.Context) (ControlPlaneDR, error) {
	var out ControlPlaneDR
	err := c.do(ctx, http.MethodGet, controlPlaneDRPath, nil, &out)
	return out, err
}

// UpdateControlPlaneDR calls PUT /api/v1/system/control-plane-dr/settings.
func (c *Client) UpdateControlPlaneDR(ctx context.Context, s ControlPlaneDRSettings) (ControlPlaneDR, error) {
	var out ControlPlaneDR
	err := c.do(ctx, http.MethodPut, controlPlaneDRPath+"/settings", s, &out)
	return out, err
}

// ListControlPlaneOffboxBackups calls GET /api/v1/system/control-plane-dr/backups.
func (c *Client) ListControlPlaneOffboxBackups(ctx context.Context) ([]ControlPlaneOffboxBackup, error) {
	var out []ControlPlaneOffboxBackup
	err := c.do(ctx, http.MethodGet, controlPlaneDRPath+"/backups", nil, &out)
	return out, err
}

// RunControlPlaneOffboxBackup calls POST /api/v1/system/control-plane-dr/run.
// The backup runs in the background; poll GetControlPlaneDR for the result.
func (c *Client) RunControlPlaneOffboxBackup(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, controlPlaneDRPath+"/run", nil, nil)
}

// RunControlPlaneDrill calls POST /api/v1/system/control-plane-dr/drill.
func (c *Client) RunControlPlaneDrill(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, controlPlaneDRPath+"/drill", nil, nil)
}

// BuildControlPlaneEscrow calls POST /api/v1/system/control-plane-dr/escrow.
// recipients, when non-empty, replace the configured ones for this bundle.
func (c *Client) BuildControlPlaneEscrow(ctx context.Context, recipients []string, upload bool) (ControlPlaneEscrowBundle, error) {
	var out ControlPlaneEscrowBundle
	body := struct {
		Recipients []string `json:"recipients"`
		Upload     bool     `json:"upload"`
	}{Recipients: recipients, Upload: upload}
	err := c.do(ctx, http.MethodPost, controlPlaneDRPath+"/escrow", body, &out)
	return out, err
}

// AckControlPlaneEscrow calls POST /api/v1/system/control-plane-dr/escrow/ack.
func (c *Client) AckControlPlaneEscrow(ctx context.Context) (ControlPlaneDR, error) {
	var out ControlPlaneDR
	err := c.do(ctx, http.MethodPost, controlPlaneDRPath+"/escrow/ack", nil, &out)
	return out, err
}
