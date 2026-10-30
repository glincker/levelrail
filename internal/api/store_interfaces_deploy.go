package api

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// DeployStore is the store surface the deploy-history handler needs.
type DeployStore interface {
	GetConditions(ctx context.Context, controllerName string) ([]reconcile.Condition, error)
	// GetConditionsForControllers is handleListApps' batched status
	// source (apps.go): one query for every app's application
	// controller, not a GetConditions call per app.
	GetConditionsForControllers(ctx context.Context, controllerNames []string) (map[string][]reconcile.Condition, error)
}

// DeployAttemptStore is the store surface for row-per-attempt deploy history, backing
// GET /api/v1/apps/{name}/deploy-attempts. *store.DB satisfies this structurally.
type DeployAttemptStore interface {
	SaveDeployAttempt(ctx context.Context, a store.DeployAttempt) error
	SetDeployAttemptCommit(ctx context.Context, id, image, commitSHA string) error
	FinishDeployAttempt(ctx context.Context, id, status string, finishedAt time.Time, errMsg string) error
	GetDeployAttempt(ctx context.Context, id string) (*store.DeployAttempt, error)
	ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
	ListFailedDeploysSince(ctx context.Context, since time.Time) ([]store.FailedDeploy, error)
}

// ProbeAttemptStore is the store surface GET
// /api/v1/apps/{name}/deploys/{deployId}/probes needs: the individual
// readiness-probe attempts (migrations/0284_probe_attempts.sql,
// internal/probe.WithOnAttempt) one deploy attempt's cutover made.
// *store.DB satisfies this structurally.
type ProbeAttemptStore interface {
	ListProbeAttempts(ctx context.Context, deployID string) ([]store.ProbeAttempt, error)
}

// WebhookDeliveryStore is the store surface real inbound webhook
// delivery history needs: row-per-delivery CRUD backing
// GET /api/v1/apps/{name}/webhook-deliveries and its replay endpoint.
// *store.DB satisfies this structurally.
type WebhookDeliveryStore interface {
	SaveWebhookDelivery(ctx context.Context, d store.WebhookDelivery) error
	GetWebhookDelivery(ctx context.Context, id string) (*store.WebhookDelivery, error)
	ListWebhookDeliveries(ctx context.Context, serviceName string, limit int, before *time.Time) ([]store.WebhookDelivery, error)
	// DeleteWebhookDeliveriesOlderThan backs the retention sweep
	// (webhook_delivery_retention.go), the same age-based deletion
	// AuditStore.DeleteAuditEntriesOlderThan already provides for audit_log.
	DeleteWebhookDeliveriesOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
	// PruneFailedWebhookDeliveries caps unsigned deliveries per app so
	// junk requests cannot grow the table between retention sweeps.
	PruneFailedWebhookDeliveries(ctx context.Context, serviceName string, keep int) (int64, error)
}

// DeployLogQuerier replays a finished attempt's logs for handleDeployLogStream. Optional:
// nil returns 501 for finished attempts, the live tail is unaffected. *telemetry.DB satisfies it.
type DeployLogQuerier interface {
	QueryDeployLog(ctx context.Context, attemptID string) ([]telemetry.DeployLogEntry, error)
}

// HookRunStore is the store surface GET /api/v1/apps/{name}/hook-runs
// (apps_hooks.go) needs: the most recent outcome of each of a service's
// pre/post-deploy hooks (internal/reconcile/application's own
// HookRunRecorder, migrations/0083_service_hook_runs.sql). Always set,
// the same "core Store interface, not an optional plug-in" shape
// domainMaintenance above establishes.
type HookRunStore interface {
	GetHookRuns(ctx context.Context, serviceName string) ([]store.HookRun, error)
}

// DeployApprovalStore is the surface deploy_approvals.go needs: create a
// pending approval gate for a deploy/promote into a protected
// environment, list/get it, and record a terminal decision (approve,
// reject, or expire). Always set, the same "core Store interface, not
// an optional plug-in" shape HookRunStore above establishes.
type DeployApprovalStore interface {
	SaveDeployApproval(ctx context.Context, a store.DeployApproval) error
	GetDeployApproval(ctx context.Context, id string) (store.DeployApproval, error)
	ListDeployApprovals(ctx context.Context, status, serviceName string) ([]store.DeployApproval, error)
	DecideDeployApproval(ctx context.Context, id, newStatus, approvedByType, approvedBy, approvedByName, reason, decidedAt string) (bool, error)
	ClaimDeployApproval(ctx context.Context, id, approvedByType, approvedBy, approvedByName, claimedAt string) (bool, error)
	FinishDeployApproval(ctx context.Context, id, claimedAt, decidedAt string) (bool, error)
	ReleaseDeployApproval(ctx context.Context, id, claimedAt string) (bool, error)
}
