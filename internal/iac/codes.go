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

// Compose reference codes (ADR 081).
const (
	// CodeComposePathInvalid means spec.compose.path is not a clean relative path.
	CodeComposePathInvalid = "compose_path_invalid"
	// CodeComposeRefMissing means a referenced compose file was not sent with the request.
	CodeComposeRefMissing = "compose_ref_missing"
	// CodeComposeTooLarge means a compose file or the attachment total is over its cap.
	CodeComposeTooLarge = "compose_too_large"
	// CodeComposeTooManyFiles means the request holds more compose files than the cap.
	CodeComposeTooManyFiles = "compose_too_many_files"
	// CodeComposeModeConflict means an App sets both spec.compose and spec.service.
	CodeComposeModeConflict = "compose_mode_conflict"
)

// Async cloud plan job and approval race codes.
const (
	// CodeApprovalPending means a cloud plan would replace the saved engine plan of a pending approval.
	CodeApprovalPending = "approval_pending"
	// CodePlanSuperseded means a later plan replaced the saved engine plan an approval named.
	CodePlanSuperseded = "plan_superseded"
	// CodeInterrupted means the control plane stopped while the plan job ran.
	CodeInterrupted = "interrupted"
	// CodePlanJobNotFound means no plan job has this id for the stack, or it was pruned.
	CodePlanJobNotFound = "plan_job_not_found"
	// CodePlanTimeout means the plan job ran past cloud.engine_timeout_seconds.
	CodePlanTimeout = "plan_timeout"
)

// Git sync codes. They never carry repository content or credentials.
const (
	// CodeSyncRepoUnreachable means the sync repository could not be fetched.
	CodeSyncRepoUnreachable = "sync_repo_unreachable"
	// CodeSyncPathInvalid means the sync path leaves the checkout or holds a symlink.
	CodeSyncPathInvalid = "sync_path_invalid"
	// CodeSyncLimitsExceeded means the repository or its files passed a sync limit.
	CodeSyncLimitsExceeded = "sync_limits_exceeded"
	// CodeSyncPlanFailed means the synced documents could not be planned.
	CodeSyncPlanFailed = "sync_plan_failed"
)

// AllCodes lists every frozen code, for docs and guard tests.
var AllCodes = []string{
	CodeItemFailed, CodePermissionDenied, CodeUnavailable, CodeReadFailed,
	CodeProtectedResource, CodeImmutableField, CodeSyncedExternally,
	CodeRefProjectMissing, CodeRefEnvironmentMissing, CodeRefAppMissing, CodeRefSecretMissing,
	CodeSchemaUnknownField, CodeSchemaVersionUnsupported, CodeOwnershipConflict,
	CodePlanStale, CodePlanHashVersion, CodeStackLocked, CodeStackSourceSync, CodePruneGuard, CodeApplyDisabled,
	CodeInvalidRequest, CodeInvalidDocument,
	CodeApprovalPending, CodePlanSuperseded, CodeInterrupted, CodePlanJobNotFound, CodePlanTimeout,
	CodeSyncRepoUnreachable, CodeSyncPathInvalid, CodeSyncLimitsExceeded, CodeSyncPlanFailed,
	CodeComposePathInvalid, CodeComposeRefMissing, CodeComposeTooLarge, CodeComposeTooManyFiles,
	CodeComposeModeConflict,
}
