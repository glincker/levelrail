package api

import "net/http"

// registerStackRoutes: stacks and their revisions. Reads (and plan, which
// persists nothing) need AbilityRead; apply, rollback, pin and delete need
// AbilityWrite on stack:<name>; unlock, settings, approve and reject need root.
func (rt *Router) registerStackRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/stacks", rt.requireAbility(AbilityRead, rt.handleListStacks))
	mux.HandleFunc("POST /api/v1/stacks", rt.requireAbility(AbilityWrite, rt.handleCreateStack))
	mux.HandleFunc("GET /api/v1/stacks/{name}", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleGetStack))
	mux.HandleFunc("PATCH /api/v1/stacks/{name}", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleUpdateStackSettings))
	mux.HandleFunc("DELETE /api/v1/stacks/{name}", rt.requireAbilityForResource(AbilityWrite, stackResourceFromPath, rt.handleDeleteStack))
	mux.HandleFunc("POST /api/v1/stacks/{name}/plan", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleStackPlan))
	mux.HandleFunc("POST /api/v1/stacks/{name}/apply", rt.requireAbilityForResource(AbilityWrite, stackResourceFromPath, rt.handleStackApply))
	mux.HandleFunc("GET /api/v1/stacks/{name}/revisions", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleStackRevisions))
	mux.HandleFunc("GET /api/v1/stacks/{name}/revisions/{n}", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleStackRevision))
	mux.HandleFunc("GET /api/v1/stacks/{name}/revisions/{n}/diff", rt.requireAbilityForResource(AbilityRead, stackResourceFromPath, rt.handleStackDiff))
	mux.HandleFunc("POST /api/v1/stacks/{name}/revisions/{n}/rollback", rt.requireAbilityForResource(AbilityWrite, stackResourceFromPath, rt.handleStackRollback))
	mux.HandleFunc("POST /api/v1/stacks/{name}/revisions/{n}/approve", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleStackApprove))
	mux.HandleFunc("POST /api/v1/stacks/{name}/revisions/{n}/reject", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleStackReject))
	mux.HandleFunc("PUT /api/v1/stacks/{name}/pin", rt.requireAbilityForResource(AbilityWrite, stackResourceFromPath, rt.handleStackPin))
	mux.HandleFunc("DELETE /api/v1/stacks/{name}/pin", rt.requireAbilityForResource(AbilityWrite, stackResourceFromPath, rt.handleStackUnpin))
	mux.HandleFunc("POST /api/v1/stacks/{name}/unlock", rt.requireAbilityForResource(AbilityRoot, stackResourceFromPath, rt.handleStackUnlock))
}
