package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// stackVerbPatterns are the root-only routes the narrow verbs unlock inside a
// stack call, each naming the verb it needs. Reset (DELETE) of a setting and
// every other node route stay absent, so a token can never reach them.
var stackVerbPatterns = map[string]string{
	"GET /api/v1/nodes":                      store.StackVerbNode,
	"GET /api/v1/nodes/{id}/labels":          store.StackVerbNode,
	"GET /api/v1/nodes/{id}/taints":          store.StackVerbNode,
	"PUT /api/v1/nodes/{id}/labels":          store.StackVerbNode,
	"PUT /api/v1/nodes/{id}/taints":          store.StackVerbNode,
	"GET /api/v1/settings/runtime":           store.StackVerbSetting,
	"GET /api/v1/settings/runtime/{key}":     store.StackVerbSetting,
	"PUT /api/v1/settings/runtime/{key}":     store.StackVerbSetting,
	"GET /api/v1/apps/{name}/placement":      store.StackVerbPlacement,
	"GET /api/v1/databases/{name}/placement": store.StackVerbPlacement,
	"GET /api/v1/nodes/placement-simulation": store.StackVerbPlacement,
	"PUT /api/v1/apps/{name}/placement":      store.StackVerbPlacement,
	"PUT /api/v1/databases/{name}/placement": store.StackVerbPlacement,
}

// stackVerbGrantKey marks the one routed pattern a narrow verb let through, so
// the ability check may treat it as root for that pattern and no other.
type stackVerbGrantKey struct{}

// stackVerbGranted reports whether the stack gate granted r's own pattern.
func stackVerbGranted(r *http.Request) bool {
	p, ok := r.Context().Value(stackVerbGrantKey{}).(string)
	return ok && p != "" && p == r.Pattern
}

type stackNodeStore interface {
	GetNode(ctx context.Context, id string) (*store.Node, error)
	GetNodeTaints(ctx context.Context, nodeID string) ([]store.NodeTaint, error)
}

// stackVerbSubRequestAllowed decides a sub-request on a verb pattern. Reads
// pass with plan or the verb; writes need the verb, an apply scope and a
// target this stack owns. Nothing here can weaken a setting or a taint.
func (rt *Router) stackVerbSubRequestAllowed(r *http.Request, db stackGateStore, stack store.StackRecord, call *stackTokenCall, verb string) (bool, *http.Request, error) {
	if !store.StackVerbsFitScope([]string{verb}, stack.Scope) {
		return false, r, nil
	}
	ts := rt.tokenStackScopes()
	if ts == nil {
		return false, r, nil
	}
	sc, err := ts.GetAPITokenStackScope(r.Context(), call.tokenID)
	if err != nil || sc == nil || sc.StackID != stack.ID {
		return false, r, err
	}
	read := r.Method == http.MethodGet
	if !sc.Has(verb) && (!read || !sc.Has(store.StackVerbPlan)) {
		return false, r, nil
	}
	if !read {
		if scope, applying := iac.StackScopeFrom(r.Context()); !applying || scope.StackID != stack.ID {
			return false, r, nil
		}
	}
	if r.URL.Query().Has("confirm_weakening") {
		return false, r, nil
	}
	switch verb {
	case store.StackVerbNode:
		if r.PathValue("id") == "" {
			return read, r, nil
		}
		ok, err := rt.stackNodeOwned(r.Context(), db, stack, r.PathValue("id"), read)
		if err != nil || !ok || read || !strings.HasSuffix(r.Pattern, "/taints") {
			return ok, r, err
		}
		return rt.stackTaintsKeepProtected(r, r.PathValue("id"))
	case store.StackVerbSetting:
		return stackSettingAllowed(r, db, stack, read)
	case store.StackVerbPlacement:
		if r.PathValue("name") == "" {
			return read, r, nil
		}
		kind := iac.KindApp
		if strings.HasPrefix(r.Pattern, "GET /api/v1/databases/") || strings.HasPrefix(r.Pattern, "PUT /api/v1/databases/") {
			kind = iac.KindDatabase
		}
		key := ledgerKeyOf(kind, r.PathValue("name"))
		e, found, err := db.GetStackResourceOwner(r.Context(), string(kind), key)
		if err != nil {
			return false, r, err
		}
		if owned := (found && e.StackID == stack.ID) || call.wasCreated(key); !owned || read {
			return owned, r, nil
		}
		return rt.stackTolerationsKeepProtected(r)
	}
	return false, r, nil
}

// stackNodeOwned bounds a node route to nodes this stack's ledger holds under
// the same id; a renamed node fails here. Unclaimed nodes are readable only.
func (rt *Router) stackNodeOwned(ctx context.Context, db stackGateStore, stack store.StackRecord, id string, read bool) (bool, error) {
	ns, ok := db.(stackNodeStore)
	if !ok {
		return false, nil
	}
	n, err := ns.GetNode(ctx, id)
	if errors.Is(err, store.ErrNodeNotFound) {
		return read, nil
	}
	if err != nil {
		return false, err
	}
	e, found, err := db.GetStackResourceOwner(ctx, string(iac.KindNode), ledgerKeyOf(iac.KindNode, n.Name))
	if err != nil {
		return false, err
	}
	if !found {
		return read, nil
	}
	return e.StackID == stack.ID && (e.LiveID == "" || e.LiveID == id), nil
}

var taintStrength = map[placement.TaintEffect]int{
	placement.EffectPreferNoSchedule: 1,
	placement.EffectNoSchedule:       2,
	placement.EffectNoExecute:        3,
}

// stackTaintsKeepProtected refuses a taint write that drops, revalues or
// loosens a taint whose key is protected; that is weakening, never a token's.
func (rt *Router) stackTaintsKeepProtected(r *http.Request, nodeID string) (bool, *http.Request, error) {
	protected := rt.runtimeSettings.List(r.Context(), runtimesettings.KeyNodesProtectedTaintKeys)
	raw, r, err := peekBody(r)
	if err != nil {
		return false, r, err
	}
	body, err := foldedBody(raw)
	if err != nil || body == nil {
		return false, r, err
	}
	if len(protected) == 0 {
		return true, r, nil
	}
	var next []placement.Taint
	if t, has := body["taints"]; has {
		if err := json.Unmarshal(t, &next); err != nil {
			return false, r, nil
		}
	}
	ns, ok := rt.stackGateDB().(stackNodeStore)
	if !ok {
		return false, r, nil
	}
	cur, err := ns.GetNodeTaints(r.Context(), nodeID)
	if err != nil {
		return false, r, err
	}
	return taintsKeepProtected(protected, cur, next), r, nil
}

// stackTolerationsKeepProtected refuses a placement write that tolerates a
// protected taint key, or every key at once; only a human may place there.
func (rt *Router) stackTolerationsKeepProtected(r *http.Request) (bool, *http.Request, error) {
	raw, r, err := peekBody(r)
	if err != nil {
		return false, r, err
	}
	body, err := foldedBody(raw)
	if err != nil || body == nil {
		return false, r, err
	}
	t, has := body["tolerations"]
	if !has {
		return true, r, nil
	}
	var tols []placement.Toleration
	if err := json.Unmarshal(t, &tols); err != nil {
		return false, r, nil
	}
	protected := map[string]bool{}
	for _, k := range rt.runtimeSettings.List(r.Context(), runtimesettings.KeyNodesProtectedTaintKeys) {
		protected[strings.ToLower(k)] = true
	}
	for _, tol := range tols {
		if protected[strings.ToLower(tol.Key)] || (tol.Key == "" && tol.Operator == placement.TolerationExists) {
			return false, r, nil
		}
	}
	return true, r, nil
}

func taintsKeepProtected(protected []string, cur []store.NodeTaint, next []placement.Taint) bool {
	isProtected := map[string]bool{}
	for _, k := range protected {
		isProtected[k] = true
	}
	for _, c := range cur {
		if !isProtected[c.Key] {
			continue
		}
		kept := false
		for _, n := range next {
			if n.Key == c.Key && n.Value == c.Value && taintStrength[n.Effect] >= taintStrength[placement.TaintEffect(c.Effect)] {
				kept = true
				break
			}
		}
		if !kept {
			return false
		}
	}
	return true
}

// stackSettingAllowed bounds a setting route: never a sensitive or unknown
// key, never confirm_weakening, and a write only to a weakens_security key
// (so the server refuses any weaker value) that no other stack owns.
func stackSettingAllowed(r *http.Request, db stackGateStore, stack store.StackRecord, read bool) (bool, *http.Request, error) {
	key := r.PathValue("key")
	if key == "" {
		return read, r, nil
	}
	d, known := settingDefinitionOf(key)
	if !known || d.Sensitive {
		return false, r, nil
	}
	if read {
		return true, r, nil
	}
	if d.Risk != runtimesettings.RiskWeakensSecurity {
		return false, r, nil
	}
	raw, r, err := peekBody(r)
	if err != nil {
		return false, r, err
	}
	body, err := foldedBody(raw)
	if err != nil || body == nil {
		return false, r, err
	}
	if v, has := body["confirm_weakening"]; has && strings.TrimSpace(string(v)) != "false" {
		return false, r, nil
	}
	ok, err := notForeign(r.Context(), db, stack, string(iac.KindSetting), key)
	return ok, r, err
}

func settingDefinitionOf(key string) (runtimesettings.Definition, bool) {
	for _, d := range runtimesettings.Definitions() {
		if d.Key == key {
			return d, true
		}
	}
	return runtimesettings.Definition{}, false
}
