package api

import "net/http"

// registerStackRoutes: stacks and their revisions. Reads (and plan, which
// persists nothing) need AbilityRead; apply, rollback, pin and delete need
// AbilityWrite on stack:<name>; unlock, settings, approve and reject need root.
func (rt *Router) registerStackRoutes(mux *http.ServeMux) {
	// List stacks. Each row carries its latest revision, status and drift summary.
	mux.HandleFunc("GET /api/v1/stacks", rt.requireAbility(AbilityRead, rt.handleListStacks))
	// Create an empty stack to own a set of declared resources. Apply fills it.
	mux.HandleFunc("POST /api/v1/stacks", rt.requireAbility(AbilityWrite, rt.handleCreateStack))
	// Show one stack with its settings, pinned revision and latest status.
	mux.HandleFunc("GET /api/v1/stacks/{name}", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleGetStack))
	// Change stack settings: prune, drift check and approval requirement. Root only.
	mux.HandleFunc("PATCH /api/v1/stacks/{name}", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleUpdateStackSettings))
	// Delete the stack record. The resources it created are left in place.
	mux.HandleFunc("DELETE /api/v1/stacks/{name}", rt.requireAbilityForResource(AbilityWrite, stackResourceFromPath, rt.handleDeleteStack))
	// Plan only: diff the submitted documents against live state and return the plan and its plan_hash. Persists nothing. With cloud kinds the plan runs as a job: 202 with the job id and poll_url. 409 approval_pending names a revision awaiting approval whose saved engine plan the new plan would replace; pass supersede=true to retire it. Error codes: docs/iac/errors.md.
	mux.HandleFunc("POST /api/v1/stacks/{name}/plan", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleStackPlan))
	// Poll an async cloud plan job: status queued, running, done, failed or expired, the plan once done, and a code on failure (interrupted, plan_timeout, approval_pending). 404 plan_job_not_found after retention.
	mux.HandleFunc("GET /api/v1/stacks/{name}/plan-jobs/{id}", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleStackPlanJob))
	// Apply the documents as a new revision. Send an Idempotency-Key so a retry returns the stored revision. Pass expected_plan_hash to refuse a stale plan with 409 (plan_hash_version when the hash is the old shape). 503 apply_disabled while the kill switch is off. Error codes: docs/iac/errors.md.
	mux.HandleFunc("POST /api/v1/stacks/{name}/apply", rt.requireAbilityForResource(AbilityWrite, stackResourceFromPath, rt.idempotent(rt.handleStackApply)))
	// List revisions, newest first, with status, actor, git metadata and plan hash.
	mux.HandleFunc("GET /api/v1/stacks/{name}/revisions", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleStackRevisions))
	// Show one revision with its documents and the result of its apply.
	mux.HandleFunc("GET /api/v1/stacks/{name}/revisions/{n}", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleStackRevision))
	// Diff a revision against the previous one, or against another with the against query.
	mux.HandleFunc("GET /api/v1/stacks/{name}/revisions/{n}/diff", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleStackDiff))
	// Apply the documents of revision n as a new revision. History is not rewritten. Honors Idempotency-Key. 503 apply_disabled while the kill switch is off.
	mux.HandleFunc("POST /api/v1/stacks/{name}/revisions/{n}/rollback", rt.requireAbilityForResource(AbilityWrite, stackResourceFromPath, rt.idempotent(rt.handleStackRollback)))
	// Approve a revision awaiting approval and apply it. Root only, never the requester. Honors Idempotency-Key. 409 plan_hash_version or a hash mismatch when the plan moved, 409 plan_superseded when a later plan replaced the saved engine plan. 503 apply_disabled while the kill switch is off. confirm_stack: typed stack name, required for solo admin self approval.
	mux.HandleFunc("POST /api/v1/stacks/{name}/revisions/{n}/approve", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.idempotent(rt.handleStackApprove)))
	// Reject a revision awaiting approval, with an optional reason. Root only.
	mux.HandleFunc("POST /api/v1/stacks/{name}/revisions/{n}/reject", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleStackReject))
	// Pin the stack to a revision number.
	mux.HandleFunc("PUT /api/v1/stacks/{name}/pin", rt.requireAbilityForResource(AbilityWrite, stackResourceFromPath, rt.handleStackPin))
	// Remove the revision pin from the stack.
	mux.HandleFunc("DELETE /api/v1/stacks/{name}/pin", rt.requireAbilityForResource(AbilityWrite, stackResourceFromPath, rt.handleStackUnpin))
	// Release a stuck apply lease. Root only.
	mux.HandleFunc("POST /api/v1/stacks/{name}/unlock", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleStackUnlock))
	// Trust a CI OIDC issuer and subject to apply this stack without a long lived token. Root only.
	mux.HandleFunc("POST /api/v1/stacks/{name}/oidc-trusts", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleCreateStackTrust))
	// List the CI OIDC trusts of a stack. Root only.
	mux.HandleFunc("GET /api/v1/stacks/{name}/oidc-trusts", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleListStackTrusts))
	// Remove a CI OIDC trust. Root only.
	mux.HandleFunc("DELETE /api/v1/stacks/{name}/oidc-trusts/{id}", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleDeleteStackTrust))
	mux.HandleFunc("GET /api/v1/stack-templates", rt.requireAbility(AbilityRead, rt.handleListStackTemplates))
	mux.HandleFunc("GET /api/v1/stack-templates/{id}", rt.requireAbility(AbilityRead, rt.handleGetStackTemplate))
	mux.HandleFunc("POST /api/v1/stack-templates/{id}/render", rt.requireAbility(AbilityWrite, rt.handleRenderStackTemplate))
	mux.HandleFunc("POST /api/v1/ci/stack-oidc-exchange", rt.handleStackOIDCExchange)
}
