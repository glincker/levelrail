package iac

// Codes are frozen: a released code is never renamed or reused, only added to.
// Clients, docs and the API error envelope key off these strings.

// Plan item codes, set on Change.Code when an item cannot be planned.
const (
	// CodeItemFailed is the fallback for a failure with no more specific code.
	CodeItemFailed = "item_failed"
	// CodePermissionDenied means the token may not read or write the resource.
	CodePermissionDenied = "permission_denied"
	// CodeUnavailable means the control plane does not serve this resource.
	CodeUnavailable = "unavailable"
	// CodeReadFailed means the live state could not be read.
	CodeReadFailed = "read_failed"
	// CodeProtectedResource means the resource is protected from this change.
	CodeProtectedResource = "protected_resource"
	// CodeImmutableField means a declared field cannot change in place.
	CodeImmutableField = "immutable_field"
	// CodeSyncedExternally means the resource is owned by an outside source.
	CodeSyncedExternally = "synced_externally"
)

// Reference codes, for a declared resource that points at one that is absent.
const (
	CodeRefProjectMissing     = "ref_project_missing"
	CodeRefEnvironmentMissing = "ref_environment_missing"
	CodeRefAppMissing         = "ref_app_missing"
	CodeRefSecretMissing      = "ref_secret_missing"
)

// Document and request codes, set outside planning (schema, stacks, apply).
const (
	CodeSchemaUnknownField       = "schema_unknown_field"
	CodeSchemaVersionUnsupported = "schema_version_unsupported"
	CodeOwnershipConflict        = "ownership_conflict"
	CodePlanStale                = "plan_stale"
	CodePlanHashVersion          = "plan_hash_version"
	CodeStackLocked              = "stack_locked"
	CodeStackSourceSync          = "stack_source_sync"
	CodePruneGuard               = "prune_guard"
	CodeApplyDisabled            = "apply_disabled"
	CodeInvalidRequest           = "invalid_request"
	CodeInvalidDocument          = "invalid_document"
)

// AllCodes lists every frozen code, for docs and guard tests.
var AllCodes = []string{
	CodeItemFailed, CodePermissionDenied, CodeUnavailable, CodeReadFailed,
	CodeProtectedResource, CodeImmutableField, CodeSyncedExternally,
	CodeRefProjectMissing, CodeRefEnvironmentMissing, CodeRefAppMissing, CodeRefSecretMissing,
	CodeSchemaUnknownField, CodeSchemaVersionUnsupported, CodeOwnershipConflict,
	CodePlanStale, CodePlanHashVersion, CodeStackLocked, CodeStackSourceSync, CodePruneGuard, CodeApplyDisabled,
	CodeInvalidRequest, CodeInvalidDocument,
}
