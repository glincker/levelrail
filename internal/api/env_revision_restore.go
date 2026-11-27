package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/store"
)

const envChangeSourceRestore = "restore"

type envRevisionRestoreRequest struct {
	Confirm string `json:"confirm"`
}

// envRevisionRestoreResource carries names only: which variables the restore added, removed or changed.
type envRevisionRestoreResource struct {
	App      string   `json:"app"`
	Restored int      `json:"restored"`
	Revision int      `json:"revision"`
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Changed  []string `json:"changed"`
}

// handleRestoreEnvRevision handles POST /api/v1/apps/{name}/env/revisions/{rev}/restore.
// It writes the old env as a new revision, so history is never rewritten.
func (rt *Router) handleRestoreEnvRevision(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req envRevisionRestoreRequest
	if !decodeReq(w, r, &req) {
		return
	}
	svc, ok := rt.loadRevealApp(w, r, name)
	if !ok {
		return
	}
	rev, err := strconv.Atoi(r.PathValue("rev"))
	if err != nil || rev < 1 {
		writeError(w, http.StatusBadRequest, "revision must be a number")
		return
	}
	if req.Confirm != name {
		writeError(w, http.StatusBadRequest, "restore needs confirm set to the exact app name")
		return
	}
	sealer, ok := rt.secrets.(envSnapshotSealer)
	if !ok || rt.envProtect.revisions == nil {
		writeError(w, http.StatusNotImplemented, "env revisions are not configured on this control plane")
		return
	}
	saved, err := rt.envProtect.revisions.GetEnvRevision(r.Context(), name, rev)
	if errors.Is(err, store.ErrEnvRevisionNotFound) {
		writeError(w, http.StatusNotFound, "revision not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: read env revision failed", err, slog.String("name", name))
		return
	}
	if len(saved.Snapshot) == 0 {
		writeError(w, http.StatusConflict, "this revision has no stored snapshot to restore")
		return
	}
	env, err := sealer.OpenEnvRevision(name, saved.Snapshot)
	if err != nil {
		rt.internalError(w, "api: open env revision failed", err, slog.String("name", name))
		return
	}
	updated := *svc
	updated.Env = env
	if updated.Env == nil {
		updated.Env = map[string]string{}
	}
	merger, ok := rt.apps.(envMergeStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "env merge is not available on this control plane")
		return
	}
	var drop []string
	for k := range svc.Env {
		if _, keep := updated.Env[k]; !keep {
			drop = append(drop, k)
		}
	}
	// Column-scoped so a concurrent spec write cannot undo or be undone by the restore.
	merged, err := merger.MergeServiceEnv(r.Context(), name, updated.Env, drop)
	if errors.Is(err, store.ErrEnvKeyIsSecret) {
		writeError(w, http.StatusConflict, "a restored key is now stored as a secret; remove the secret first")
		return
	}
	if err != nil {
		rt.internalError(w, "api: env restore save failed", err, slog.String("name", name))
		return
	}
	updated.Env = merged.After
	out := envRevisionRestoreResource{App: name, Restored: rev, Added: []string{}, Removed: []string{}, Changed: []string{}}
	for _, c := range diffEnvKeys(svc.Env, updated.Env) {
		switch c.Action {
		case store.EnvChangeAdded:
			out.Added = append(out.Added, c.Key)
		case store.EnvChangeRemoved:
			out.Removed = append(out.Removed, c.Key)
		default:
			out.Changed = append(out.Changed, c.Key)
		}
	}
	_, _, actor, _ := rt.currentActor(r)
	// Always snapshot, even when nothing differs, so the restore itself shows in history.
	out.Revision = rt.snapshotEnvRevision(r.Context(), name, actor, updated.Env)
	if rt.envProtect.changes != nil {
		if changes := diffEnvKeys(svc.Env, updated.Env); len(changes) > 0 {
			if err := rt.envProtect.changes.RecordEnvChanges(r.Context(), name, actor, envChangeSourceRestore, changes); err != nil {
				rt.logger.Warn("api: record env changes failed", slog.String("name", name))
			}
		}
	}
	rt.nudgeReconciler()
	for _, l := range [][]string{out.Added, out.Removed, out.Changed} {
		sort.Strings(l)
	}
	rt.auditEnvRestore(r, out)
	writeJSON(w, http.StatusOK, out)
}

// auditEnvRestore logs names and revision numbers only, never a value.
func (rt *Router) auditEnvRestore(r *http.Request, res envRevisionRestoreResource) {
	detail, _ := json.Marshal(map[string]any{
		"app": res.App, "restored": res.Restored, "revision": res.Revision,
		"added": res.Added, "removed": res.Removed, "changed": res.Changed,
	})
	a := rt.accessActor(r)
	rt.saveAppExpiryAudit(r.Context(), store.AuditEntry{
		ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: http.MethodPost, Path: "/api/v1/apps/" + res.App + "/env/revisions/" + strconv.Itoa(res.Restored) + "/restore",
		StatusCode: http.StatusOK, RemoteAddr: clientIP(r), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: "app.env.restore", Detail: string(detail),
	})
}
