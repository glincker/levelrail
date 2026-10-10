package dbaccess

// Audit action names written to the audit log's method column. They never
// carry a credential, only the database and the subject role or principal.
const (
	ActionUserCreate  = "database.user.create"
	ActionUserRotate  = "database.user.rotate"
	ActionUserDisable = "database.user.disable"
	ActionUserEnable  = "database.user.enable"
	ActionUserDelete  = "database.user.delete"

	ActionTempIssue  = "database.temp.issue"
	ActionTempRevoke = "database.temp.revoke"
	ActionTempExpire = "database.temp.expire"

	ActionGrantApply  = "database.access.grant"
	ActionGrantRemove = "database.access.ungrant"

	ActionRulesApply  = "database.network.rules.apply"
	ActionRulesRemove = "database.network.rules.remove"
	ActionMakePrivate = "database.network.make_private"
	ActionScopeSet    = "database.network.scope.set"
	ActionTLSSet      = "database.network.tls.set"
)

// SystemActor names the sweeper in audit rows.
const SystemActor = "database-access-sweeper"
