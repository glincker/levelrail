package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
)

const secretRefNeedsRoot = "a secret reference outside an allowed prefix can only be set by root, the tier that manages secret connections"

// secretRefInPrefixAllowList is the seam for per connection prefix lists; none exist yet,
// so every reference is outside the list and needs root.
func (rt *Router) secretRefInPrefixAllowList(_ context.Context, _ secretsource.Ref) bool {
	return false
}

// allowSecretRefs writes 403 and returns false when the caller may not point an app at these references.
func (rt *Router) allowSecretRefs(w http.ResponseWriter, r *http.Request, refs ...secretsource.Ref) bool {
	for _, ref := range refs {
		if rt.secretRefInPrefixAllowList(r.Context(), ref) || rt.callerHasAbility(r, AbilityRoot) {
			continue
		}
		writeError(w, http.StatusForbidden, secretRefNeedsRoot)
		return false
	}
	return true
}

// envRefOf converts a stored reference to the parsed form.
func envRefOf(v appVaultEnvRef) (secretsource.Ref, error) {
	return secretsource.RefSpec{Conn: v.Conn, Path: v.Path, Key: v.Key, Version: v.Version}.Ref()
}

// liftEnvSecretRefs moves plain env values written as secret://conn/path#key into
// vault_env entries so the reference, not a literal, is what gets stored. A bad
// reference is an error naming the variable only, since the text may be a typo of a value.
func liftEnvSecretRefs(env map[string]string, into map[string]appVaultEnvRef) (map[string]string, map[string]appVaultEnvRef, error) {
	var refs []string
	for k, v := range env {
		if secretsource.IsRef(v) {
			refs = append(refs, k)
		}
	}
	if len(refs) == 0 {
		return env, into, nil
	}
	rest := make(map[string]string, len(env))
	for k, v := range env {
		rest[k] = v
	}
	if into == nil {
		into = map[string]appVaultEnvRef{}
	}
	for _, k := range refs {
		ref, err := secretsource.ParseRef(rest[k])
		if err != nil {
			return nil, nil, fmt.Errorf("env %q: %w", k, err)
		}
		delete(rest, k)
		into[k] = appVaultEnvRef{Conn: ref.Conn, Path: ref.Path, Key: ref.Key, Version: ref.Version}
	}
	return rest, into, nil
}

// secretRefsOf lists the connection references among vault_env entries.
func secretRefsOf(m map[string]appVaultEnvRef) ([]secretsource.Ref, error) {
	var out []secretsource.Ref
	for k, v := range m {
		if v.Conn == "" {
			continue
		}
		ref, err := envRefOf(v)
		if err != nil {
			return nil, fmt.Errorf("vault_env[%q]: %w", k, err)
		}
		out = append(out, ref)
	}
	return out, nil
}

// auditSecretRef records that a reference was added or changed: the reference text, never a value.
func (rt *Router) auditSecretRef(r *http.Request, app, envVar, ref string) {
	a := rt.accessActor(r)
	b, _ := json.Marshal(map[string]string{"app": app, "env": envVar, "ref": ref})
	rt.saveAppExpiryAudit(r.Context(), store.AuditEntry{
		ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: "app.secret_ref.set", Detail: string(b),
	})
}

// parseSecretRefString accepts the string form only when it is a reference.
func parseSecretRefString(s string) (secretsource.Ref, bool) {
	if !strings.HasPrefix(s, "secret://") {
		return secretsource.Ref{}, false
	}
	ref, err := secretsource.ParseRef(s)
	return ref, err == nil
}

// vetSecretRefs lifts secret:// env values into vault_env, validates every connection
// reference, and requires root for each one that is new or changed. It audits those.
// existing is nil on create.
func (rt *Router) vetSecretRefs(w http.ResponseWriter, r *http.Request, app string, existing map[string]store.VaultEnvRef, req *appResource) bool {
	if lifted := anySecretRefEnv(req.Env); lifted {
		base := req.VaultEnv
		if base == nil && len(existing) > 0 {
			base = make(map[string]appVaultEnvRef, len(existing))
			for k, v := range existing {
				base[k] = appVaultEnvRef{Path: v.Path, Key: v.Key, Conn: v.Conn, Version: v.Version}
			}
		}
		env, vault, err := liftEnvSecretRefs(req.Env, base)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return false
		}
		req.Env, req.VaultEnv = env, vault
	}
	refs, err := secretRefsOf(req.VaultEnv)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	if len(refs) == 0 {
		return true
	}
	type change struct{ env, ref string }
	var changed []change
	for k, v := range req.VaultEnv {
		if v.Conn == "" || existing[k] == v.store() {
			continue
		}
		ref, _ := envRefOf(v)
		if !rt.allowSecretRefs(w, r, ref) {
			return false
		}
		changed = append(changed, change{k, ref.String()})
	}
	for _, c := range changed {
		rt.auditSecretRef(r, app, c.env, c.ref)
	}
	return true
}

func anySecretRefEnv(env map[string]string) bool {
	for _, v := range env {
		if secretsource.IsRef(v) {
			return true
		}
	}
	return false
}
