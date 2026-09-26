package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Pending change kinds and apply actions.
const (
	pendingKindEnv    = "env"
	pendingKindSecret = "secret"
	pendingKindConfig = "config"

	pendingApplyRestart = "restart"
)

type pendingChange struct {
	Kind  string   `json:"kind"`
	Keys  []string `json:"keys,omitempty"`
	Since string   `json:"since"`
}

type pendingChangesResource struct {
	Pending     bool            `json:"pending"`
	Changes     []pendingChange `json:"changes"`
	ApplyAction string          `json:"apply_action"`
}

// computePendingChanges compares desired state with the last release that
// went live. A snapshot naming an older release still counts: a rollout in
// flight has not made anything live yet. Without any snapshot (a container
// from before snapshots) only the env_dirty latch and secret events are known.
func (rt *Router) computePendingChanges(ctx context.Context, svc store.DesiredService) (pendingChangesResource, error) {
	res := pendingChangesResource{Changes: []pendingChange{}, ApplyAction: pendingApplyRestart}
	events := rt.appEvents()
	if svc.Suspended || events == nil {
		return res, nil
	}
	applied, err := events.GetAppliedConfig(ctx, svc.Name)
	if err != nil {
		return res, err
	}
	var envKeys, secretKeys, configKeys []string
	switch {
	case applied != nil:
		secretValues, unresolved := rt.declaredSecretValues(ctx, svc)
		hashes := make(map[string]string, len(applied.EnvHashes))
		for k, h := range applied.EnvHashes {
			if !slices.Contains(unresolved, k) {
				hashes[k] = h
			}
		}
		snapshot := *applied
		snapshot.EnvHashes = hashes
		wantEnv := store.AppEnvHashes(svc.Name, svc.Env, secretValues)
		for _, k := range unresolved {
			delete(wantEnv, k)
		}
		drift := store.DiffApplied(&snapshot, wantEnv, store.AppliedFields(svc))
		secretSet := append(store.SecretEnvNames(svc.SecretEnv), strings.Split(applied.Fields[store.AppliedFieldSecretKeys], ",")...)
		for _, k := range drift.EnvKeys {
			if slices.Contains(secretSet, k) {
				secretKeys = append(secretKeys, k)
			} else {
				envKeys = append(envKeys, k)
			}
		}
		configKeys = drift.ConfigKeys
	default:
		if svc.EnvDirty {
			res.Changes = append(res.Changes, pendingChange{Kind: pendingKindEnv, Since: rt.pendingSince(ctx, events, svc.Name, nil, store.AppEventEnvChange)})
		}
		if secretEvents, err := events.ListAppEvents(ctx, svc.Name, nil, time.Time{}, []string{store.AppEventSecretChange}, 1); err == nil && len(secretEvents) > 0 {
			res.Changes = append(res.Changes, pendingChange{Kind: pendingKindSecret, Since: rt.pendingSince(ctx, events, svc.Name, nil, store.AppEventSecretChange)})
		}
	}

	add := func(kind string, keys []string, eventKind string) {
		if len(keys) == 0 {
			return
		}
		res.Changes = append(res.Changes, pendingChange{Kind: kind, Keys: keys, Since: rt.pendingSince(ctx, events, svc.Name, applied, eventKind)})
	}
	add(pendingKindEnv, envKeys, store.AppEventEnvChange)
	add(pendingKindSecret, secretKeys, store.AppEventSecretChange)
	add(pendingKindConfig, configKeys, store.AppEventConfigChange)
	res.Pending = len(res.Changes) > 0
	return res, nil
}

// declaredSecretValues resolves the app's declared secrets for hashing only.
// unresolved names keys whose value could not be read, which the comparison
// then ignores instead of reporting as removed.
func (rt *Router) declaredSecretValues(ctx context.Context, svc store.DesiredService) (values map[string]string, unresolved []string) {
	values = map[string]string{}
	for _, ref := range svc.SecretEnv {
		if rt.secrets == nil {
			unresolved = append(unresolved, ref.Name)
			continue
		}
		exists, err := rt.secrets.Exists(ctx, svc.Name, ref.Name)
		if err != nil {
			unresolved = append(unresolved, ref.Name)
			continue
		}
		if !exists {
			continue
		}
		v, err := rt.secrets.Resolve(ctx, svc.Name, ref.Name)
		if err != nil {
			unresolved = append(unresolved, ref.Name)
			continue
		}
		values[ref.Name] = v
	}
	return values, unresolved
}

// pendingSince is when the oldest still-unapplied change of eventKind was
// made: the first such event after the container was created, else the
// creation time itself.
func (rt *Router) pendingSince(ctx context.Context, events appEventStore, name string, applied *store.AppliedConfig, eventKind string) string {
	var after time.Time
	if applied != nil {
		after = applied.AppliedAt
	}
	fallback := time.Now()
	if applied != nil {
		fallback = applied.AppliedAt
	}
	list, err := events.ListAppEvents(ctx, name, nil, after, []string{eventKind}, timelineMaxLimit)
	if err == nil && len(list) > 0 {
		fallback = list[len(list)-1].CreatedAt
	}
	return fallback.UTC().Format(time.RFC3339)
}

// handlePendingChanges handles GET /api/v1/apps/{name}/pending-changes.
func (rt *Router) handlePendingChanges(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: pending changes: load app failed", err, slog.String("name", name))
		return
	}
	res, err := rt.computePendingChanges(r.Context(), *svc)
	if err != nil {
		rt.internalError(w, "api: pending changes failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type applyPendingResult struct {
	AttemptID string `json:"attempt_id,omitempty"`
}

// handleApplyPending handles POST /api/v1/apps/{name}/apply-pending: recreates
// the container so it picks up the pending env, secret and config changes.
// Nothing pending is a no-op that still answers 202.
func (rt *Router) handleApplyPending(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: apply pending: load app failed", err, slog.String("name", name))
		return
	}
	res, err := rt.computePendingChanges(r.Context(), *svc)
	if err != nil {
		rt.internalError(w, "api: apply pending failed", err, slog.String("name", name))
		return
	}
	if !res.Pending {
		writeJSON(w, http.StatusAccepted, applyPendingResult{})
		return
	}
	if err := rt.apps.RestartService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: apply pending: restart failed", err, slog.String("name", name))
		return
	}
	var keys []string
	var kinds []string
	for _, c := range res.Changes {
		kinds = append(kinds, c.Kind)
		keys = append(keys, c.Keys...)
	}
	slices.Sort(keys)
	rt.recordAppEvent(r, store.AppEvent{
		AppName: name, Kind: store.AppEventRestart, Keys: keys,
		Title:  "Applied pending changes",
		Detail: "restarted to apply " + strings.Join(kinds, ", "),
	})
	rt.nudgeReconciler()
	writeJSON(w, http.StatusAccepted, applyPendingResult{})
}
