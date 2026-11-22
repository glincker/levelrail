package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/changes"
	"github.com/GLINCKER/levelrail/internal/configrev"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	configRevisionActorBaseline = "baseline"
	appsPathPrefix              = "/api/v1/apps/"
	configRevisionListLimit     = 50
)

// configRevisionStore is the optional store surface behind config history.
// *store.DB satisfies it; a Router over a store without it keeps no history.
type configRevisionStore interface {
	AddAppConfigRevision(ctx context.Context, rev store.AppConfigRevision) (int, error)
	ListAppConfigRevisions(ctx context.Context, app string, limit int) ([]store.AppConfigRevision, error)
	GetAppConfigRevision(ctx context.Context, app string, n int) (store.AppConfigRevision, bool, error)
}

func (rt *Router) configRevisions() configRevisionStore {
	s, _ := rt.apps.(configRevisionStore)
	return s
}

// configRevision is the wire shape of one revision: who, when and the diff.
type configRevision struct {
	N        int                     `json:"n"`
	At       time.Time               `json:"at"`
	Actor    string                  `json:"actor"`
	Kinds    []string                `json:"kinds"`
	Changes  []configrev.FieldChange `json:"changes"`
	RevertOf int                     `json:"revert_of,omitempty"`
}

func toConfigRevision(rev store.AppConfigRevision) configRevision {
	out := configRevision{N: rev.N, At: rev.At, Actor: rev.Actor, RevertOf: rev.RevertOf, Changes: []configrev.FieldChange{}}
	_ = jsonUnmarshalString(rev.Changes, &out.Changes)
	out.Kinds = configrev.Kinds(out.Changes)
	return out
}

// recordConfigRevision stores a new revision when the app's revertible fields
// differ from the latest one, and returns its number (0 when nothing changed).
// Failures are logged and never returned: the change it describes already landed.
func (rt *Router) recordConfigRevision(ctx context.Context, app, actor string, revertOf int) int {
	s := rt.configRevisions()
	if s == nil {
		return 0
	}
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil {
		return 0
	}
	cur := configrev.Capture(*svc)
	last, err := s.ListAppConfigRevisions(ctx, app, 1)
	if err != nil {
		rt.logger.Warn("api: list config revisions failed", slog.String("error", err.Error()), slog.String("name", app))
		return 0
	}
	var diff []configrev.FieldChange
	if len(last) > 0 {
		prev, err := configrev.Unmarshal(last[0].Snapshot)
		if err == nil {
			if configrev.Equal(prev, cur) {
				return 0
			}
			diff = configrev.Diff(prev, cur)
		}
	} else {
		actor = configRevisionActorBaseline
	}
	snap, err := configrev.Marshal(cur)
	if err != nil {
		return 0
	}
	diffJSON, err := jsonMarshalString(diff)
	if err != nil {
		return 0
	}
	n, err := s.AddAppConfigRevision(ctx, store.AppConfigRevision{App: app, At: time.Now(), Actor: actor, RevertOf: revertOf, Snapshot: snap, Changes: diffJSON})
	if err != nil {
		rt.logger.Warn("api: record config revision failed", slog.String("error", err.Error()), slog.String("name", app))
		return 0
	}
	return n
}

// recordConfigRevisionAfterWrite is called from the audit hook after a
// successful write under /api/v1/apps/{name}, so every edit path (env, scale,
// domains, health, update) is captured without each handler opting in.
func (rt *Router) recordConfigRevisionAfterWrite(ctx context.Context, r *http.Request, actorType, actorName string, status int) {
	if status >= http.StatusMultipleChoices || r.Method == http.MethodGet || r.Method == http.MethodHead {
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, appsPathPrefix)
	if !ok || rest == "" {
		return
	}
	name, _, _ := strings.Cut(rest, "/")
	actor := actorName
	if actorType == store.PrincipalTypeToken {
		actor = "token:" + actorName
	}
	rt.recordConfigRevision(ctx, name, actor, 0)
}

// appChangesResponse is the aggregated change list plus the config revisions
// each of which can be reverted to.
type appChangesResponse struct {
	changes.Result
	Revisions []configRevision `json:"revisions"`
}

func (rt *Router) listConfigRevisions(ctx context.Context, app string) []configRevision {
	out := []configRevision{}
	s := rt.configRevisions()
	if s == nil {
		return out
	}
	list, err := s.ListAppConfigRevisions(ctx, app, configRevisionListLimit)
	if err == nil && len(list) == 0 {
		rt.recordConfigRevision(ctx, app, configRevisionActorBaseline, 0)
		list, err = s.ListAppConfigRevisions(ctx, app, configRevisionListLimit)
	}
	if err != nil {
		rt.logger.Warn("api: list config revisions failed", slog.String("error", err.Error()), slog.String("name", app))
		return out
	}
	for _, rev := range list {
		out = append(out, toConfigRevision(rev))
	}
	return out
}

type revertRequest struct {
	To     int  `json:"to"`
	DryRun bool `json:"dry_run,omitempty"`
}

type revertResponse struct {
	App        string                  `json:"app"`
	RevertedTo int                     `json:"reverted_to"`
	Revision   int                     `json:"revision,omitempty"`
	DryRun     bool                    `json:"dry_run,omitempty"`
	Changes    []configrev.FieldChange `json:"changes"`
	// Skipped names env keys whose masked value cannot be restored.
	Skipped []string `json:"skipped,omitempty"`
	// PendingApply is true when env changed: it applies on the next restart or deploy.
	PendingApply bool `json:"pending_apply"`
}

// handleRevertApp handles POST /api/v1/apps/{name}/revert {to}: it applies the
// recorded revision's env, scale, resources, domains and health as a new
// revision. Databases, volumes and the image are never touched.
func (rt *Router) handleRevertApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s := rt.configRevisions()
	if s == nil {
		writeError(w, http.StatusNotImplemented, "config history is not supported by this store")
		return
	}
	var req revertRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.To <= 0 {
		writeError(w, http.StatusBadRequest, "to must be a revision number from the app's change history")
		return
	}
	live, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: revert app: load app failed", err, slog.String("name", name))
		return
	}
	// Record any edit the hook has not seen yet so the revert has a true "before".
	rt.recordConfigRevision(r.Context(), name, rt.eventActor(r), 0)
	rev, found, err := s.GetAppConfigRevision(r.Context(), name, req.To)
	if err != nil {
		rt.internalError(w, "api: revert app: load revision failed", err, slog.String("name", name))
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "revision not found for this app")
		return
	}
	target, err := configrev.Unmarshal(rev.Snapshot)
	if err != nil {
		rt.internalError(w, "api: revert app: decode revision failed", err, slog.String("name", name))
		return
	}
	if msg := rt.revertBlocker(r.Context(), name, *live, target); msg != "" {
		writeCodedError(w, http.StatusConflict, "revert_target_missing", msg)
		return
	}
	plan := configrev.Apply(*live, target)
	diff := configrev.Diff(configrev.Capture(*live), configrev.Capture(plan.Service))
	resp := revertResponse{App: name, RevertedTo: req.To, DryRun: req.DryRun, Changes: diff, Skipped: plan.Skipped}
	if diff == nil {
		resp.Changes = []configrev.FieldChange{}
	}
	if req.DryRun || len(diff) == 0 {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	desired := plan.Service
	desired.EnvDirty = live.EnvDirty || envDiffers(live.Env, desired.Env)
	if err := rt.apps.SaveDesiredService(r.Context(), desired); err != nil {
		var taken *store.ErrDomainTaken
		if errors.As(err, &taken) {
			writeCodedError(w, http.StatusConflict, "revert_target_missing", taken.Error())
			return
		}
		rt.internalError(w, "api: revert app failed", err, slog.String("name", name))
		return
	}
	rt.recordEnvChanges(r, name, live.Env, desired.Env, envChangeSourceAPI)
	for _, ev := range updateEvents(name, *live, desired) {
		rt.recordAppEvent(r, ev)
	}
	resp.Revision = rt.recordConfigRevision(r.Context(), name, rt.eventActor(r), req.To)
	resp.PendingApply = desired.EnvDirty
	rt.nudgeReconciler()
	rt.changes.Publish(changeApps, name)
	writeJSON(w, http.StatusOK, resp)
}

func envDiffers(a, b map[string]string) bool {
	if len(a) != len(b) {
		return true
	}
	for k, v := range a {
		if o, ok := b[k]; !ok || o != v {
			return true
		}
	}
	return false
}

// revertBlocker returns a human reason when the target references something
// that no longer exists: a stored secret value or a domain owned by another app.
func (rt *Router) revertBlocker(ctx context.Context, name string, live store.DesiredService, target configrev.Snapshot) string {
	has, _ := rt.apps.(interface {
		HasSecretValue(ctx context.Context, serviceName, envKey string) (bool, error)
	})
	for _, ref := range target.SecretEnv {
		if has == nil || containsRef(live.SecretEnv, ref.Name) {
			continue
		}
		if ok, err := has.HasSecretValue(ctx, name, ref.Name); err == nil && !ok {
			return "secret " + ref.Name + " no longer exists, set it again before reverting"
		}
	}
	return ""
}

func containsRef(refs []store.SecretEnvRef, name string) bool {
	for _, r := range refs {
		if r.Name == name {
			return true
		}
	}
	return false
}

func jsonMarshalString(v any) (string, error) {
	if v == nil {
		return "[]", nil
	}
	b, err := json.Marshal(v)
	return string(b), err
}

func jsonUnmarshalString(s string, dst any) error { return json.Unmarshal([]byte(s), dst) }
