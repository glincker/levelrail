// Wire type for GET /api/v1/apps/{name}/deploys
// (internal/api/deploys.go's handleDeployHistory). The response is the
// app's *current* reconcile conditions, one per (controller, condition
// type) pair: internal/store.UpsertConditions only ever keeps the latest
// row for each type, there is no attempt-by-attempt deploy log yet. This
// type name and every consumer of it should keep reading as "status",
// not "history".
//
// Field names are capitalized (Type, Status, Reason, ...) because
// internal/reconcile.Condition carries no `json:"..."` struct tags, so
// encoding/json falls back to the exported Go field name verbatim. This
// is the one place in this codebase's frontend types that isn't
// camelCase or snake_case; that's a direct reflection of the actual
// wire format, not a typo.
export type ConditionStatus = 'True' | 'False' | 'Unknown'

export interface ReconcileCondition {
  Type: string
  Status: ConditionStatus
  Reason: string
  Message: string
  LastTransitionTime: string
}
