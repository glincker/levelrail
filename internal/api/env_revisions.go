package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const envRevisionsListMax = 200

type envRevisionStore interface {
	AddEnvRevision(ctx context.Context, app, actor string, names []string, snapshot []byte, keep int, maxAge time.Duration) (int, error)
	ListEnvRevisions(ctx context.Context, app string, limit int) ([]store.EnvRevision, error)
	GetEnvRevision(ctx context.Context, app string, rev int) (store.EnvRevision, error)
}

// envSnapshotSealer is the part of the secrets manager that encrypts an env snapshot.
type envSnapshotSealer interface {
	SealEnvRevision(app string, env map[string]string) ([]byte, error)
	OpenEnvRevision(app string, blob []byte) (map[string]string, error)
}

type envRevisionResource struct {
	Rev   int      `json:"rev"`
	At    string   `json:"at"`
	Actor string   `json:"actor"`
	Names []string `json:"names"`
}

type envRevisionsResponse struct {
	Revisions []envRevisionResource `json:"revisions"`
}

// envRevisionDiffResource lists variable names only. Changed is empty when a snapshot cannot be opened.
type envRevisionDiffResource struct {
	From    int      `json:"from"`
	To      int      `json:"to"`
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Changed []string `json:"changed"`
	// Unreadable means a snapshot could not be opened, so an empty Changed proves nothing.
	Unreadable bool `json:"unreadable,omitempty"`
	// ChangedHidden means value comparison needs a step-up first, so Changed was left out.
	ChangedHidden bool `json:"changed_hidden,omitempty"`
}

// snapshotEnvRevision saves the app's new env as a revision. A failure is logged without any value, never fatal.
func (rt *Router) snapshotEnvRevision(ctx context.Context, app, actor string, env map[string]string) int {
	if rt.envProtect.revisions == nil {
		return 0
	}
	names := make([]string, 0, len(env))
	for k := range env {
		names = append(names, k)
	}
	sort.Strings(names)
	var blob []byte
	if sealer, ok := rt.secrets.(envSnapshotSealer); ok {
		b, err := sealer.SealEnvRevision(app, env)
		if err != nil {
			rt.logger.Warn("api: seal env revision failed", slog.String("name", app))
		}
		blob = b
	}
	keep := rt.runtimeSettings.Int(ctx, runtimesettings.KeyEnvRevisionsKeep)
	maxAge := time.Duration(rt.runtimeSettings.Int(ctx, runtimesettings.KeyAppsUndoWindowDay)) * 24 * time.Hour
	rev, err := rt.envProtect.revisions.AddEnvRevision(ctx, app, actor, names, blob, keep, maxAge)
	if err != nil {
		rt.logger.Warn("api: save env revision failed", slog.String("name", app))
		return 0
	}
	return rev
}

// handleListEnvRevisions handles GET /api/v1/apps/{name}/env/revisions (limit). Names only.
func (rt *Router) handleListEnvRevisions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := rt.loadRevealApp(w, r, name); !ok {
		return
	}
	out := envRevisionsResponse{Revisions: []envRevisionResource{}}
	if rt.envProtect.revisions != nil {
		limit := envHistoryDefaultLimit
		if n, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && n > 0 {
			limit = min(n, envRevisionsListMax)
		}
		rows, err := rt.envProtect.revisions.ListEnvRevisions(r.Context(), name, limit)
		if err != nil {
			rt.internalError(w, "api: list env revisions failed", err, slog.String("name", name))
			return
		}
		for _, rev := range rows {
			names := rev.Names
			if names == nil {
				names = []string{}
			}
			out.Revisions = append(out.Revisions, envRevisionResource{Rev: rev.Rev, At: rev.At.UTC().Format(time.RFC3339), Actor: rev.Actor, Names: names})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDiffEnvRevisions handles GET /api/v1/apps/{name}/env/revisions/diff?from=N&to=M.
func (rt *Router) handleDiffEnvRevisions(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok := rt.loadRevealApp(w, r, name); !ok {
		return
	}
	from, errFrom := strconv.Atoi(r.URL.Query().Get("from"))
	to, errTo := strconv.Atoi(r.URL.Query().Get("to"))
	if errFrom != nil || errTo != nil || from < 1 || to < 1 {
		writeError(w, http.StatusBadRequest, "from and to must be revision numbers")
		return
	}
	if rt.envProtect.revisions == nil {
		writeError(w, http.StatusNotFound, "revision not found")
		return
	}
	a, errA := rt.envProtect.revisions.GetEnvRevision(r.Context(), name, from)
	b, errB := rt.envProtect.revisions.GetEnvRevision(r.Context(), name, to)
	if errors.Is(errA, store.ErrEnvRevisionNotFound) || errors.Is(errB, store.ErrEnvRevisionNotFound) {
		writeError(w, http.StatusNotFound, "revision not found")
		return
	}
	if errA != nil || errB != nil {
		rt.internalError(w, "api: read env revision failed", errors.Join(errA, errB), slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, rt.diffEnvRevisions(name, a, b, rt.envRevisionCompareAllowed(r, name)))
}

// envRevisionCompareAllowed gates the changed list: comparing values lets a caller test guesses, so it needs the reveal step-up.
func (rt *Router) envRevisionCompareAllowed(r *http.Request, app string) bool {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		return false
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	_, granted := rt.envProtect.grants.check(c.Value, userID, stepUpActionReveal, app, time.Now())
	return granted
}

func (rt *Router) diffEnvRevisions(app string, a, b store.EnvRevision, compare bool) envRevisionDiffResource {
	out := envRevisionDiffResource{From: a.Rev, To: b.Rev, Added: []string{}, Removed: []string{}, Changed: []string{}}
	inA, inB := map[string]bool{}, map[string]bool{}
	for _, n := range a.Names {
		inA[n] = true
	}
	for _, n := range b.Names {
		inB[n] = true
		if !inA[n] {
			out.Added = append(out.Added, n)
		}
	}
	for _, n := range a.Names {
		if !inB[n] {
			out.Removed = append(out.Removed, n)
		}
	}
	// Values are compared in memory only to flag a name as changed, never returned.
	sealer, ok := rt.secrets.(envSnapshotSealer)
	switch {
	case !compare:
		out.ChangedHidden = true
	case !ok || len(a.Snapshot) == 0 || len(b.Snapshot) == 0:
		out.Unreadable = true
	default:
		envA, errA := sealer.OpenEnvRevision(app, a.Snapshot)
		envB, errB := sealer.OpenEnvRevision(app, b.Snapshot)
		if errA != nil || errB != nil {
			out.Unreadable = true
			break
		}
		for k, vb := range envB {
			if va, had := envA[k]; had && va != vb {
				out.Changed = append(out.Changed, k)
			}
		}
	}
	sort.Strings(out.Added)
	sort.Strings(out.Removed)
	sort.Strings(out.Changed)
	return out
}
