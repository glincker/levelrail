package plugin

import "context"

// Audit actions written for plugin lifecycle and ability use.
const (
	AuditInstall     = "plugin.install"
	AuditApprove     = "plugin.approve"
	AuditEnable      = "plugin.enable"
	AuditDisable     = "plugin.disable"
	AuditRemove      = "plugin.remove"
	AuditAbilityUsed = "plugin.ability_used"
	AuditTrustAdd    = "plugin.trust_add"
	AuditTrustRemove = "plugin.trust_remove"
)

// AuditEvent is one plugin audit record. Detail must never carry a secret.
type AuditEvent struct {
	Action string
	Actor  string
	Plugin string
	Detail map[string]string
}

// Auditor persists audit events. A failure on an ability use refuses the call.
type Auditor interface {
	Record(ctx context.Context, e AuditEvent) error
}
