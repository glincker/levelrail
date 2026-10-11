package apiclient

import (
	"context"
	"net/http"
	"net/url"
)

// SelfUpgradeBreaking is one breaking change between two versions.
type SelfUpgradeBreaking struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Summary     string `json:"summary"`
	RequiresAck bool   `json:"requires_ack"`
}

// SelfUpgradePlan is GET /api/v1/updates/self-upgrade/plan's response.
type SelfUpgradePlan struct {
	CurrentVersion string                `json:"current_version"`
	TargetVersion  string                `json:"target_version"`
	Breaking       []SelfUpgradeBreaking `json:"breaking"`
	NotesAvailable bool                  `json:"notes_available"`
	CanApply       bool                  `json:"can_apply"`
	CannotApplyWhy string                `json:"cannot_apply_reason"`
	Command        string                `json:"command"`
	Steps          []string              `json:"steps"`
}

// SelfUpgradeStep is one step of an attempt's timeline.
type SelfUpgradeStep struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	At         string `json:"at"`
	DurationMS int64  `json:"duration_ms"`
}

// SelfUpgradeAttempt is one recorded upgrade attempt.
type SelfUpgradeAttempt struct {
	ID          string            `json:"id"`
	FromVersion string            `json:"from_version"`
	ToVersion   string            `json:"to_version"`
	FromSchema  int               `json:"from_schema"`
	ToSchema    int               `json:"to_schema"`
	Initiator   string            `json:"initiator"`
	Outcome     string            `json:"outcome"`
	FailedStep  string            `json:"failed_step"`
	Error       string            `json:"error"`
	BackupName  string            `json:"backup_name"`
	Acked       []string          `json:"acked"`
	Steps       []SelfUpgradeStep `json:"steps"`
	StartedAt   string            `json:"started_at"`
	FinishedAt  string            `json:"finished_at"`
}

// SelfUpgradeAttempts is GET /api/v1/updates/self-upgrade/attempts' response.
type SelfUpgradeAttempts struct {
	Attempts []SelfUpgradeAttempt `json:"attempts"`
}

// SelfUpgradeStarted is POST /api/v1/updates/self-upgrade's response.
type SelfUpgradeStarted struct {
	Status        string `json:"status"`
	TargetVersion string `json:"target_version"`
}

// GetSelfUpgradePlan calls GET /api/v1/updates/self-upgrade/plan. An empty
// target asks for the latest release on the configured channel.
func (c *Client) GetSelfUpgradePlan(ctx context.Context, target string) (SelfUpgradePlan, error) {
	var out SelfUpgradePlan
	path := "/api/v1/updates/self-upgrade/plan"
	if target != "" {
		path += "?target=" + url.QueryEscape(target)
	}
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// StartSelfUpgrade calls POST /api/v1/updates/self-upgrade.
func (c *Client) StartSelfUpgrade(ctx context.Context, target string, ack []string) (SelfUpgradeStarted, error) {
	var out SelfUpgradeStarted
	body := map[string]any{"target": target, "ack": ack}
	err := c.do(ctx, http.MethodPost, "/api/v1/updates/self-upgrade", body, &out)
	return out, err
}

// ListSelfUpgradeAttempts calls GET /api/v1/updates/self-upgrade/attempts.
func (c *Client) ListSelfUpgradeAttempts(ctx context.Context) (SelfUpgradeAttempts, error) {
	var out SelfUpgradeAttempts
	err := c.do(ctx, http.MethodGet, "/api/v1/updates/self-upgrade/attempts", nil, &out)
	return out, err
}
