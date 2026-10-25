package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

// A stack-scoped token (ADR 042) may call only its own stack's plan, apply,
// history, diff and rollback routes. The loopback sub-requests those handlers
// make are then bounded to the stack's ledger and project. Everything else is
// denied, and every lookup failure denies.

type tokenStackScopeStore interface {
	SetAPITokenStackScope(ctx context.Context, s store.APITokenStackScope) error
	GetAPITokenStackScope(ctx context.Context, tokenID string) (*store.APITokenStackScope, error)
}

type stackGateStore interface {
	GetStack(ctx context.Context, id string) (store.StackRecord, error)
	GetStackByName(ctx context.Context, name string) (store.StackRecord, error)
	GetStackResourceOwner(ctx context.Context, kind, key string) (store.StackLedgerEntry, bool, error)
	OwnerOfApp(ctx context.Context, name string) (*store.ResourceOwner, error)
	OwnerOfDatabase(ctx context.Context, name string) (*store.ResourceOwner, error)
	TeamOfEnvironment(ctx context.Context, environmentID string) (store.ResourceOwner, error)
}

func (rt *Router) tokenStackScopes() tokenStackScopeStore {
	if s, ok := rt.policies.(tokenStackScopeStore); ok {
		return s
	}
	if s, ok := rt.apps.(tokenStackScopeStore); ok {
		return s
	}
	return nil
}

func (rt *Router) stackGateDB() stackGateStore {
	if s, ok := rt.policies.(stackGateStore); ok {
		return s
	}
	if s, ok := rt.apps.(stackGateStore); ok {
		return s
	}
	return nil
}

// stackVerbAbilities is the most a stack token may hold per verb; write is
// what apply and rollback need, and nothing sensitive is ever granted.
var stackVerbAbilities = map[string][]string{
	store.StackVerbPlan:  {AbilityRead},
	store.StackVerbApply: {AbilityRead, AbilityWrite},
}

const msgStackTokenDenied = "this token is scoped to one stack and cannot call this endpoint"

// resolveTokenStackScope validates the optional stack scope of a new token.
// The minting user must themselves hold the matching ability on that stack.
func (rt *Router) resolveTokenStackScope(w http.ResponseWriter, r *http.Request, req createTokenRequest) (*store.APITokenStackScope, bool) {
	if req.Stack == "" {
		if len(req.StackVerbs) > 0 {
			writeError(w, http.StatusBadRequest, "stack is required with stack_verbs")
			return nil, false
		}
		return nil, true
	}
	verbs := req.StackVerbs
	if len(verbs) == 0 {
		verbs = []string{store.StackVerbPlan}
	}
	if !store.ValidStackVerbs(verbs) {
		writeError(w, http.StatusBadRequest, "stack_verbs may only hold plan and apply")
		return nil, false
	}
	allowed := map[string]bool{}
	need := AbilityRead
	for _, v := range verbs {
		for _, a := range stackVerbAbilities[v] {
			allowed[a] = true
		}
		if v == store.StackVerbApply {
			need = AbilityWrite
		}
	}
	for _, a := range req.Abilities {
		if !allowed[a] {
			writeError(w, http.StatusBadRequest, "ability "+a+" is beyond what the stack verbs allow")
			return nil, false
		}
	}
	db, ss := rt.stackGateDB(), rt.tokenStackScopes()
	ownerID, _ := rt.currentSessionUserID(r)
	if db == nil || ss == nil || ownerID == "" {
		writeError(w, http.StatusBadRequest, "stack scoped tokens need a signed in user and stacks support")
		return nil, false
	}
	stack, err := db.GetStackByName(r.Context(), req.Stack)
	if errors.Is(err, store.ErrStackNotFound) {
		writeError(w, http.StatusBadRequest, "unknown stack")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: create token: stack scope", err)
		return nil, false
	}
	if ok, err := rt.userMayOnStack(r, ownerID, need, stack.Name); err != nil {
		rt.internalError(w, "api: create token: stack scope authz", err)
		return nil, false
	} else if !ok {
		writeError(w, http.StatusForbidden, "you cannot grant "+need+" on that stack")
		return nil, false
	}
	return &store.APITokenStackScope{StackID: stack.ID, Verbs: dedupe(verbs)}, true
}

// userMayOnStack runs the minting user through the same resource decision a
// request on the stack would get, so Deny policies and team scope still apply.
func (rt *Router) userMayOnStack(r *http.Request, userID, ability, stackName string) (bool, error) {
	user, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil {
		return false, err
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), store.PrincipalTypeUser, userID)
	if err != nil {
		return false, err
	}
	allowed, hidden, err := rt.authorizeResourceInEnvironment(r.Context(), store.PrincipalTypeUser, userID, user.Abilities, policies, ability, "stack:"+stackName)
	return allowed && !hidden, err
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// stackTokenCall is the server-side marker an allowed stack route attaches to
// its context. Loopback sub-requests inherit it; nothing a client sends can.
type stackTokenCall struct {
	tokenID string
	stackID string
	mu      sync.Mutex
	// created holds databases this call created, which are projectless until
	// the follow-up project assignment.
	created map[string]bool
}

func (c *stackTokenCall) markCreated(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.created[key] = true
}

func (c *stackTokenCall) wasCreated(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.created[key]
}

type stackTokenCallKey struct{}

// tokenStackScoped reports whether tokenID is bound to a stack, failing closed
// on a lookup error. Handlers outside requireAbility use it to refuse them.
func (rt *Router) tokenStackScoped(ctx context.Context, tokenID string) bool {
	ts := rt.tokenStackScopes()
	if ts == nil {
		return false
	}
	sc, err := ts.GetAPITokenStackScope(ctx, tokenID)
	return err != nil || sc != nil
}

// gateStackToken enforces the stack scope of tokenID, if any. It returns the
// request to continue with, or false after writing the denial.
func (rt *Router) gateStackToken(w http.ResponseWriter, r *http.Request, tokenID string) (*http.Request, bool) {
	ts := rt.tokenStackScopes()
	if ts == nil {
		return r, true
	}
	sc, err := ts.GetAPITokenStackScope(r.Context(), tokenID)
	if err != nil {
		rt.internalError(w, "api: stack token scope lookup failed", err)
		return r, false
	}
	if sc == nil {
		return r, true
	}
	db := rt.stackGateDB()
	if db == nil {
		writeError(w, http.StatusForbidden, msgStackTokenDenied)
		return r, false
	}
	stack, err := db.GetStack(r.Context(), sc.StackID)
	if errors.Is(err, store.ErrStackNotFound) {
		writeError(w, http.StatusForbidden, msgStackTokenDenied)
		return r, false
	}
	if err != nil {
		rt.internalError(w, "api: stack token stack lookup failed", err)
		return r, false
	}
	var ok bool
	if call, inCall := r.Context().Value(stackTokenCallKey{}).(*stackTokenCall); inCall {
		if call.tokenID != tokenID || call.stackID != stack.ID {
			ok = false
		} else if ok, r, err = rt.stackSubRequestAllowed(r, db, stack, call); err != nil {
			rt.internalError(w, "api: stack token sub-request check failed", err)
			return r, false
		}
	} else if ok, r, err = stackRouteAllowed(r, *sc, stack); err == nil && ok {
		call := &stackTokenCall{tokenID: tokenID, stackID: stack.ID, created: map[string]bool{}}
		r = r.WithContext(context.WithValue(r.Context(), stackTokenCallKey{}, call))
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return r, false
	}
	if !ok {
		if msg := hiddenStackRead(r); msg != "" {
			// Same answer as a name that does not exist, so a plan cannot probe other projects.
			writeError(w, http.StatusNotFound, msg)
			return r, false
		}
		rt.logger.Info("api: stack token denied", slog.String("pattern", r.Pattern), slog.String("token_id", tokenID), slog.String("stack", stack.Name))
		writeError(w, http.StatusForbidden, msgStackTokenDenied)
		return r, false
	}
	return r, true
}

// hiddenStackRead returns the 404 message for a denied loader read of an app or
// database, or "" when the denial should stay a 403.
func hiddenStackRead(r *http.Request) string {
	if _, inCall := r.Context().Value(stackTokenCallKey{}).(*stackTokenCall); !inCall || r.Method != http.MethodGet {
		return ""
	}
	_, path, _ := strings.Cut(r.Pattern, " ")
	switch stackReadPatterns[path] {
	case "app":
		return "app not found"
	case "database":
		return "database not found"
	}
	return ""
}

// stackRouteVerbs lists the only stack routes a stack token can reach and the
// verb each needs. Settings, delete, unlock, pin, adopt and list are absent.
var stackRouteVerbs = map[string]string{
	"POST /api/v1/stacks/{name}/plan":                   store.StackVerbPlan,
	"GET /api/v1/stacks/{name}":                         "",
	"GET /api/v1/stacks/{name}/revisions":               "",
	"GET /api/v1/stacks/{name}/revisions/{n}":           "",
	"GET /api/v1/stacks/{name}/revisions/{n}/diff":      "",
	"POST /api/v1/stacks/{name}/apply":                  store.StackVerbApply,
	"POST /api/v1/stacks/{name}/revisions/{n}/rollback": store.StackVerbApply,
}

// stackRouteAllowed decides a direct call: only the token's own stack, only a
// listed route, only a granted verb, and an apply may not widen itself.
func stackRouteAllowed(r *http.Request, sc store.APITokenStackScope, stack store.StackRecord) (bool, *http.Request, error) {
	verb, listed := stackRouteVerbs[r.Pattern]
	if !listed || r.PathValue("name") != stack.Name {
		return false, r, nil
	}
	if verb != "" && !sc.Has(verb) {
		return false, r, nil
	}
	if verb != store.StackVerbApply {
		return true, r, nil
	}
	var body struct {
		Prune          bool `json:"prune"`
		Adopt          bool `json:"adopt"`
		AllowMassPrune bool `json:"allow_mass_prune"`
	}
	raw, r, err := peekBody(r)
	if err != nil {
		return false, r, err
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &body); err != nil {
			return false, r, err
		}
	}
	// Adopt and mass prune widen what one apply may touch; a human confirms those.
	if body.Adopt || body.AllowMassPrune || (body.Prune && !stack.AllowPrune) {
		return false, r, nil
	}
	return true, r, nil
}

// stackReadPatterns are the reads the plan and apply loader makes. Values
// name what the path addresses so it can be bounded to the stack.
var stackReadPatterns = map[string]string{
	"/api/v1/apps":                          "",
	"/api/v1/databases":                     "",
	"/api/v1/projects":                      "",
	"/api/v1/tags":                          "",
	"/api/v1/tags/{id}/apps":                "",
	"/api/v1/notification-channels":         "",
	"/api/v1/apps/{name}":                   "app",
	"/api/v1/apps/{name}/loadbalancer":      "app",
	"/api/v1/apps/{name}/pipelines":         "app",
	"/api/v1/apps/{name}/pipelines/{pname}": "app",
	"/api/v1/apps/{name}/alerts":            "app",
	"/api/v1/databases/{name}":              "database",
	"/api/v1/projects/{id}":                 "project",
	"/api/v1/projects/{id}/env":             "project",
	"/api/v1/projects/{id}/environments":    "project",
	"/api/v1/environments/{id}/env":         "environment",
}

// stackWritePatterns are the mutations an apply makes. Secrets, exec, reveal,
// moves and every other app or database route are absent and so denied.
// Pipeline create and update are absent too: steps run commands with the
// app's secrets, so only a human principal may write them.
var stackWritePatterns = map[string]string{
	"POST /api/v1/apps":                            "app-create",
	"PUT /api/v1/apps/{name}":                      "app",
	"PUT /api/v1/apps/{name}/project":              "app",
	"PUT /api/v1/apps/{name}/environment":          "app",
	"POST /api/v1/apps/{name}/restart":             "app",
	"POST /api/v1/apps/{name}/tags":                "app",
	"DELETE /api/v1/apps/{name}/tags/{id}":         "app",
	"PUT /api/v1/apps/{name}/loadbalancer":         "app",
	"DELETE /api/v1/apps/{name}/loadbalancer":      "app",
	"DELETE /api/v1/apps/{name}/pipelines/{pname}": "app",
	"POST /api/v1/apps/{name}/alerts":              "app",
	"PUT /api/v1/apps/{name}/alerts/{id}":          "app",
	"DELETE /api/v1/apps/{name}/alerts/{id}":       "app",
	"DELETE /api/v1/apps/{name}":                   "app-prune",
	"POST /api/v1/databases":                       "database-create",
	"PUT /api/v1/databases/{name}/project":         "database",
	"POST /api/v1/tags":                            "tag-create",
	"PUT /api/v1/projects/{id}/env":                "project",
	"POST /api/v1/projects/{id}/environments":      "project",
	"PUT /api/v1/environments/{id}/env":            "environment",
}

// stackSubRequestAllowed decides a loopback sub-request. Reads are allowed in
// plan and apply; writes only while the stack handler has set its apply scope.
func (rt *Router) stackSubRequestAllowed(r *http.Request, db stackGateStore, stack store.StackRecord, call *stackTokenCall) (bool, *http.Request, error) {
	method, path, _ := strings.Cut(r.Pattern, " ")
	if method == http.MethodGet {
		target, listed := stackReadPatterns[path]
		if !listed {
			return false, r, nil
		}
		ok, err := rt.stackTargetAllowed(r, db, stack, call, target, true)
		return ok, r, err
	}
	target, listed := stackWritePatterns[r.Pattern]
	if scope, applying := iac.StackScopeFrom(r.Context()); !listed || !applying || scope.StackID != stack.ID {
		return false, r, nil
	}
	raw, r, err := peekBody(r)
	if err != nil {
		return false, r, err
	}
	body, err := foldedBody(raw)
	if err != nil {
		return false, r, err
	}
	if body == nil {
		return false, r, nil
	}
	if ok, err := rt.stackBodyInScope(r.Context(), db, stack, body); err != nil || !ok {
		return false, r, err
	}
	switch target {
	case "app-create":
		name := bodyString(body, "name")
		if name == "" || bodyString(body, "project_id") != stack.ProjectID {
			return false, r, nil
		}
		ok, err := notForeign(r.Context(), db, stack, string(iac.KindApp), name)
		return ok, r, err
	case "database-create":
		name := bodyString(body, "name")
		if name == "" {
			return false, r, nil
		}
		ok, err := notForeign(r.Context(), db, stack, string(iac.KindDatabase), name)
		if ok && err == nil {
			call.markCreated(ledgerKeyOf(iac.KindDatabase, name))
		}
		return ok, r, err
	case "tag-create":
		return true, r, nil
	case "app-prune":
		// Prune deletes only what this stack owns, and only when an admin allowed it.
		if !stack.AllowPrune {
			return false, r, nil
		}
		e, found, err := db.GetStackResourceOwner(r.Context(), string(iac.KindApp), ledgerKeyOf(iac.KindApp, r.PathValue("name")))
		return err == nil && found && e.StackID == stack.ID, r, err
	}
	ok, err := rt.stackTargetAllowed(r, db, stack, call, target, false)
	return ok, r, err
}

// stackTargetAllowed bounds one addressed resource to the stack. A resource
// another stack owns is always refused; one nobody owns must sit in the
// stack's project. For reads a resource that does not exist is fine (404).
func (rt *Router) stackTargetAllowed(r *http.Request, db stackGateStore, stack store.StackRecord, call *stackTokenCall, target string, read bool) (bool, error) {
	ctx := r.Context()
	switch target {
	case "":
		return read, nil
	case "project":
		return stack.ProjectID != "" && r.PathValue("id") == stack.ProjectID, nil
	case "environment":
		return environmentInStackProject(ctx, db, stack, r.PathValue("id"))
	case "app", "database":
		kind, name := iac.KindApp, r.PathValue("name")
		ownerOf := db.OwnerOfApp
		if target == "database" {
			kind, ownerOf = iac.KindDatabase, db.OwnerOfDatabase
		}
		e, found, err := db.GetStackResourceOwner(ctx, string(kind), ledgerKeyOf(kind, name))
		if err != nil {
			return false, err
		}
		if found {
			return e.StackID == stack.ID, nil
		}
		if call.wasCreated(ledgerKeyOf(kind, name)) {
			return true, nil
		}
		o, err := ownerOf(ctx, name)
		if err != nil {
			return false, err
		}
		if o == nil {
			return read, nil
		}
		return stack.ProjectID != "" && o.ProjectID == stack.ProjectID, nil
	}
	return false, nil
}

// stackBodyInScope refuses a write whose body names a project or environment
// outside the stack, so no resource can be moved out of it.
func (rt *Router) stackBodyInScope(ctx context.Context, db stackGateStore, stack store.StackRecord, body map[string]json.RawMessage) (bool, error) {
	if _, has := body["project_id"]; has {
		if pid := bodyString(body, "project_id"); pid != "" && pid != stack.ProjectID {
			return false, nil
		}
	}
	if _, has := body["environment_id"]; has {
		if eid := bodyString(body, "environment_id"); eid != "" {
			return environmentInStackProject(ctx, db, stack, eid)
		}
	}
	return true, nil
}

func environmentInStackProject(ctx context.Context, db stackGateStore, stack store.StackRecord, environmentID string) (bool, error) {
	if environmentID == "" || stack.ProjectID == "" {
		return false, nil
	}
	o, err := db.TeamOfEnvironment(ctx, environmentID)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return o.ProjectID == stack.ProjectID, nil
}

func notForeign(ctx context.Context, db stackGateStore, stack store.StackRecord, kind, name string) (bool, error) {
	e, found, err := db.GetStackResourceOwner(ctx, kind, ledgerKeyOf(iac.Kind(kind), name))
	if err != nil {
		return false, err
	}
	return !found || e.StackID == stack.ID, nil
}

func ledgerKeyOf(kind iac.Kind, name string) string { return string(kind) + "/" + name }

// foldedBody decodes a JSON object with lower-cased keys, because handlers
// decode case-insensitively. It returns nil when two keys fold together, so
// a duplicate can never hide a second value from these checks.
func foldedBody(raw []byte) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if len(raw) == 0 {
		return out, nil
	}
	var in map[string]json.RawMessage
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, err
	}
	for k, v := range in {
		lk := strings.ToLower(k)
		if _, dup := out[lk]; dup {
			return nil, nil
		}
		out[lk] = v
	}
	return out, nil
}

// bodyString returns a JSON string field; any other type reads as a value
// that cannot match, so a crafted type never slips past a comparison.
func bodyString(body map[string]json.RawMessage, key string) string {
	raw, ok := body[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "\x00invalid"
	}
	return s
}

// peekBody reads the body for inspection and puts an identical copy back.
func peekBody(r *http.Request) ([]byte, *http.Request, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, r, nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, maxIaCBodyBytes+1))
	_ = r.Body.Close()
	if err != nil {
		return nil, r, err
	}
	if len(raw) > maxIaCBodyBytes {
		return nil, r, errors.New("request body too large")
	}
	r2 := r.Clone(r.Context())
	r2.Body = io.NopCloser(bytes.NewReader(raw))
	return raw, r2, nil
}
