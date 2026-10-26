package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Lock mode (ADR 042 section 7): a stack with drift_policy "lock" refuses
// manual changes to the resources its ledger owns. The guard runs before the
// route handlers so no mutation handler needs to know about stacks.

const (
	errCodeStackLocked      = "stack_locked"
	auditStackLockOverride  = "stack.lock_override"
	headerOverrideStackLock = "X-Levelrail-Override-Stack-Lock"
	stackDriftLock          = "lock"
)

// lockSafeVerbs are the last path segments of POST routes that run or inspect
// without changing declared configuration. Everything else on a locked
// resource is refused, so a new route is locked until listed here.
var lockSafeVerbs = map[string]bool{
	"start": true, "stop": true, "restart": true, "now": true, "wake": true,
	"exec": true, "keep": true, "purge": true, "run": true, "runs": true,
	"cancel": true, "retry": true, "rerun": true, "hold": true, "verify": true,
	"dry-run": true, "check": true, "preview": true, "lint": true, "test": true,
	"detect": true, "affected": true, "validate": true, "validate-spec": true,
	"preflight": true, "doctor": true, "discover": true, "reveal": true,
	"export": true, "scan": true, "renew": true, "silence": true, "diagnose": true,
	"explain": true, "query": true, "builds": true, "backups": true, "base-backups": true,
	"cert": true, "clone": true, "environments": true,
}

type lockTarget struct {
	kind iac.Kind
	// id is a project or environment id that must be resolved to a ledger name.
	id string
	// name is the ledger name for apps, databases and domains.
	name string
}

type stackLockStore interface {
	stackGateStore
	GetProject(ctx context.Context, id string) (store.Project, error)
	GetEnvironment(ctx context.Context, id string) (store.Environment, error)
}

func (rt *Router) stackLockDB() stackLockStore {
	if s, ok := rt.policies.(stackLockStore); ok {
		return s
	}
	if s, ok := rt.apps.(stackLockStore); ok {
		return s
	}
	return nil
}

func isMutatingMethod(m string) bool {
	switch m {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// lockTargetsOf maps a mutating request to the ledger resources it touches.
// It reports false for routes that are operational or outside the ledger.
func lockTargetsOf(method, path string) ([]lockTarget, bool) {
	rest, ok := strings.CutPrefix(path, "/api/v1/")
	if !ok {
		return nil, false
	}
	segs := strings.Split(strings.Trim(rest, "/"), "/")
	if len(segs) < 2 {
		return nil, false
	}
	last := segs[len(segs)-1]
	if method == http.MethodPost && len(segs) > 2 && lockSafeVerbs[last] {
		return nil, false
	}
	switch segs[0] {
	case "apps":
		ts := []lockTarget{{kind: iac.KindApp, name: segs[1]}}
		if len(segs) > 3 && segs[2] == "domains" {
			ts = append(ts, lockTarget{kind: iac.KindDomain, name: segs[3]})
		}
		return ts, true
	case "databases":
		return []lockTarget{{kind: iac.KindDatabase, name: segs[1]}}, true
	case "domains":
		d := segs[1]
		if d == "standalone" && len(segs) > 2 {
			d = segs[2]
		}
		return []lockTarget{{kind: iac.KindDomain, name: d}}, true
	case "projects":
		return []lockTarget{{kind: iac.KindProject, id: segs[1]}}, true
	case "environments":
		return []lockTarget{{kind: iac.KindEnvironment, id: segs[1]}}, true
	}
	return nil, false
}

// lockKey resolves a target to its ledger (kind, key). ok is false when the
// resource does not exist, so nothing can own it.
func lockKey(ctx context.Context, db stackLockStore, t lockTarget) (string, bool, error) {
	switch t.kind {
	case iac.KindProject:
		p, err := db.GetProject(ctx, t.id)
		if errors.Is(err, store.ErrProjectNotFound) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		return ledgerKeyOf(iac.KindProject, p.Name), true, nil
	case iac.KindEnvironment:
		e, err := db.GetEnvironment(ctx, t.id)
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		p, err := db.GetProject(ctx, e.ProjectID)
		if errors.Is(err, store.ErrProjectNotFound) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		return ledgerKeyOf(iac.KindEnvironment, p.Name+"/"+e.Name), true, nil
	}
	return ledgerKeyOf(t.kind, t.name), true, nil
}

// lockedBy returns the lock-policy stack owning any target, if there is one.
// A store error returns it so the caller can fail open with a warning.
func lockedBy(ctx context.Context, db stackLockStore, ts []lockTarget) (store.StackRecord, string, bool, error) {
	for _, t := range ts {
		key, ok, err := lockKey(ctx, db, t)
		if err != nil {
			return store.StackRecord{}, "", false, err
		}
		if !ok {
			continue
		}
		e, found, err := db.GetStackResourceOwner(ctx, string(t.kind), key)
		if err != nil {
			return store.StackRecord{}, "", false, err
		}
		if !found {
			continue
		}
		stack, err := db.GetStack(ctx, e.StackID)
		if errors.Is(err, store.ErrStackNotFound) {
			continue
		}
		if err != nil {
			return store.StackRecord{}, "", false, err
		}
		if stack.DriftPolicy == stackDriftLock {
			return stack, key, true, nil
		}
	}
	return store.StackRecord{}, "", false, nil
}

// stackLockMiddleware answers 409 stack_locked to a mutating request on a
// resource owned by a lock-policy stack, unless it is that stack's own apply
// or a root caller named the stack in the override header.
func (rt *Router) stackLockMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutatingMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		ts, ok := lockTargetsOf(r.Method, r.URL.Path)
		db := rt.stackLockDB()
		if !ok || db == nil {
			next.ServeHTTP(w, r)
			return
		}
		stack, key, locked, err := lockedBy(r.Context(), db, ts)
		if err != nil {
			// Fail open: a broken ledger read must not freeze every edit.
			rt.logger.Warn("api: stack lock lookup failed, allowing request", slog.String("error", err.Error()), slog.String("route", r.Method+" "+r.URL.Path))
			next.ServeHTTP(w, r)
			return
		}
		if !locked {
			next.ServeHTTP(w, r)
			return
		}
		if scope, applying := iac.StackScopeFrom(r.Context()); applying && scope.StackID == stack.ID {
			next.ServeHTTP(w, r)
			return
		}
		if !rt.lockHasPrincipal(r) {
			// Unauthenticated callers get the handler's own 401.
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get(headerOverrideStackLock) == stack.Name && rt.lockOverrideAllowed(r) {
			rt.auditStack(r, auditStackLockOverride, map[string]any{"stack": stack.Name, "route": r.Method + " " + r.URL.Path, "resource": key})
			next.ServeHTTP(w, r)
			return
		}
		writeCodedError(w, http.StatusConflict, errCodeStackLocked,
			"managed by stack "+stack.Name+" (drift policy lock): change the stack manifest and apply it, or ask an administrator to unlock the stack or override the lock")
	})
}

// lockHasPrincipal reports whether the request carries a credential that
// resolves to someone, including a pinned session.
func (rt *Router) lockHasPrincipal(r *http.Request) bool {
	if _, ok := rt.currentPinnedSession(r); ok {
		return true
	}
	_, _, _, ok := rt.currentActor(r)
	return ok
}

// lockOverrideAllowed is true only for a root session or non-CI root token
// that is not stack-scoped. Any lookup failure refuses the override.
func (rt *Router) lockOverrideAllowed(r *http.Request) bool {
	if _, pinned := rt.currentPinnedSession(r); pinned {
		return false
	}
	c := rt.stackCaller(r)
	if c.kind == "unknown" || c.isCI {
		return false
	}
	if c.kind == store.PrincipalTypeToken && rt.tokenStackScoped(r.Context(), c.id) {
		return false
	}
	abilities, err := rt.callerAbilities(r)
	return err == nil && hasAbilityExactRoot(abilities)
}

func hasAbilityExactRoot(abilities []string) bool {
	for _, a := range abilities {
		if a == AbilityRoot {
			return true
		}
	}
	return false
}
