package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	bulkActionRedeploy       = "redeploy"
	bulkActionRestart        = "restart"
	bulkActionStop           = "stop"
	bulkActionStart          = "start"
	bulkActionAddTag         = "add-tag"
	bulkActionRemoveTag      = "remove-tag"
	bulkActionSetEnvironment = "set-environment"
	bulkActionMoveToProject  = "move-to-project"
	bulkActionDelete         = "delete"

	bulkStatusOK       = "ok"
	bulkStatusWouldRun = "would_apply"
	bulkStatusDenied   = "denied"
	bulkStatusNotFound = "not_found"
	bulkStatusSkipped  = "skipped"
	bulkStatusError    = "error"
)

// bulkActionAbility maps each action to the ability each app must grant,
// matching the single-app route the action mirrors.
var bulkActionAbility = map[string]string{
	bulkActionRedeploy:       AbilityDeploy,
	bulkActionRestart:        AbilityDeploy,
	bulkActionStop:           AbilityDeploy,
	bulkActionStart:          AbilityDeploy,
	bulkActionAddTag:         AbilityWrite,
	bulkActionRemoveTag:      AbilityWrite,
	bulkActionSetEnvironment: AbilityWrite,
	bulkActionMoveToProject:  AbilityWrite,
	bulkActionDelete:         AbilityWrite,
}

type bulkAppsRequest struct {
	Action string `json:"action"`
	// Names lists target apps; Tag and Environment select by filter when Names is empty.
	Names       []string `json:"names,omitempty"`
	Tag         string   `json:"tag,omitempty"`
	Environment string   `json:"environment,omitempty"`
	// Value is the tag, environment ID or name, or project ID the action applies.
	Value  string `json:"value,omitempty"`
	DryRun bool   `json:"dry_run,omitempty"`
	// ConfirmNames must equal the resolved target list for delete.
	ConfirmNames []string `json:"confirm_names,omitempty"`
}

type bulkAppResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type bulkAppsResponse struct {
	Action  string          `json:"action"`
	DryRun  bool            `json:"dry_run"`
	Results []bulkAppResult `json:"results"`
	Counts  map[string]int  `json:"counts"`
}

func envIntDefault(key string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return def
}

// bulkPrincipal is the caller identity plus the IAM policies each app is
// checked against.
type bulkPrincipal struct {
	actorType, actorID string
	abilities          []string
	policies           []store.Policy
}

func (p bulkPrincipal) allowed(ability, app string) bool {
	return authorizeResource(p.abilities, p.policies, ability, "app:"+app)
}

// handleBulkApps handles POST /api/v1/apps/bulk. Each app is authorized
// against the caller's IAM on its own: denied apps are reported per item and
// never fail the batch. Always answers 207 once the request itself is valid.
func (rt *Router) handleBulkApps(w http.ResponseWriter, r *http.Request) {
	var req bulkAppsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ability, ok := bulkActionAbility[req.Action]
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown action")
		return
	}
	if err := validateBulkValue(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	names, err := rt.resolveBulkTargets(r.Context(), req)
	if err != nil {
		rt.internalError(w, "api: bulk apps: resolve targets failed", err)
		return
	}
	if len(names) == 0 {
		writeError(w, http.StatusBadRequest, "no target apps: set names, tag or environment")
		return
	}
	if maxN := envIntDefault("APP_BULK_MAX_APPS", 200); len(names) > maxN {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("too many apps (%d, max %d)", len(names), maxN))
		return
	}
	if req.Action == bulkActionDelete && !req.DryRun && !sameNameSet(req.ConfirmNames, names) {
		writeError(w, http.StatusConflict, "delete requires confirm_names listing exactly the target apps")
		return
	}

	principal, err := rt.bulkPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: bulk apps: resolve caller failed", err)
		return
	}

	results := make([]bulkAppResult, len(names))
	sem := make(chan struct{}, envIntDefault("APP_BULK_CONCURRENCY", 4))
	var wg sync.WaitGroup
	for i, name := range names {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i] = rt.applyBulkOne(r.Context(), r, principal, ability, req, name)
		}()
	}
	wg.Wait()

	counts := map[string]int{}
	for _, res := range results {
		counts[res.Status]++
	}
	if !req.DryRun {
		rt.nudgeReconciler()
	}
	writeJSON(w, http.StatusMultiStatus, bulkAppsResponse{Action: req.Action, DryRun: req.DryRun, Results: results, Counts: counts})
}

func validateBulkValue(req bulkAppsRequest) error {
	switch req.Action {
	case bulkActionAddTag, bulkActionRemoveTag:
		if _, err := validateTagName(req.Value); err != nil {
			return err
		}
	case bulkActionMoveToProject:
		if req.Value == "" {
			return errors.New("value (project id) is required")
		}
	}
	return nil
}

func sameNameSet(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(slices.Compact(x), slices.Compact(y))
}

func (rt *Router) resolveBulkTargets(ctx context.Context, req bulkAppsRequest) ([]string, error) {
	if len(req.Names) > 0 {
		names := slices.Clone(req.Names)
		slices.Sort(names)
		return slices.Compact(names), nil
	}
	if req.Tag == "" && req.Environment == "" {
		return nil, nil
	}
	fs, ok := rt.apps.(appFilterStore)
	if !ok {
		return nil, errNoFilteredList
	}
	f := store.AppListFilter{Environment: req.Environment}
	if req.Tag != "" {
		f.Tags = []string{req.Tag}
	}
	return fs.ListAppNamesFiltered(ctx, f)
}

func (rt *Router) bulkPrincipal(r *http.Request) (bulkPrincipal, error) {
	pt, pid, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return bulkPrincipal{}, err
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), pt, pid)
	if err != nil {
		return bulkPrincipal{}, fmt.Errorf("list caller policies: %w", err)
	}
	actorType := "session"
	if pt == store.PrincipalTypeToken {
		actorType = "token"
	}
	return bulkPrincipal{actorType: actorType, actorID: pid, abilities: abilities, policies: policies}, nil
}

func (rt *Router) applyBulkOne(ctx context.Context, r *http.Request, p bulkPrincipal, ability string, req bulkAppsRequest, name string) bulkAppResult {
	if !p.allowed(ability, name) {
		return bulkAppResult{Name: name, Status: bulkStatusDenied, Message: "your account lacks the required ability on this app"}
	}
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		return bulkAppResult{Name: name, Status: bulkStatusNotFound, Message: "app not found"}
	}
	if err != nil {
		rt.logger.Error("api: bulk apps: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		return bulkAppResult{Name: name, Status: bulkStatusError, Message: "internal error"}
	}
	if req.DryRun {
		return bulkAppResult{Name: name, Status: bulkStatusWouldRun, Message: req.Action}
	}
	status, msg := rt.runBulkAction(ctx, req, *svc)
	if status == bulkStatusOK {
		rt.recordBulkAudit(ctx, r, p, ability, req.Action, name)
	}
	return bulkAppResult{Name: name, Status: status, Message: msg}
}

func (rt *Router) runBulkAction(ctx context.Context, req bulkAppsRequest, svc store.DesiredService) (status, message string) {
	fail := func(what string, err error) (string, string) {
		rt.logger.Error("api: bulk apps: "+what+" failed", slog.String("error", err.Error()), slog.String("name", svc.Name), slog.String("action", req.Action))
		return bulkStatusError, what + " failed"
	}
	switch req.Action {
	case bulkActionRedeploy:
		if env, err := rt.environments.GetEnvironment(ctx, svc.EnvironmentID); err == nil && env.Protected {
			return bulkStatusSkipped, "protected environment: redeploy it individually to go through approval"
		}
		if _, err := rt.executeConfirmedDeploy(ctx, svc, svc.Image, confirmedDeployOptions{reason: "bulk redeploy"}); err != nil {
			return fail("redeploy", err)
		}
	case bulkActionRestart:
		if err := rt.apps.RestartService(ctx, svc.Name); err != nil {
			return fail("restart", err)
		}
	case bulkActionStop, bulkActionStart:
		if err := rt.apps.UpdateServiceSuspended(ctx, svc.Name, req.Action == bulkActionStop); err != nil {
			return fail(req.Action, err)
		}
	case bulkActionAddTag:
		tagName, _ := validateTagName(req.Value)
		if _, err := rt.attachTagByName(ctx, svc.Name, tagName); errors.Is(err, errTooManyTags) {
			return bulkStatusSkipped, err.Error()
		} else if err != nil {
			return fail("add tag", err)
		}
	case bulkActionRemoveTag:
		tagName, _ := validateTagName(req.Value)
		tag, err := rt.tags.GetTagByName(ctx, tagName)
		if errors.Is(err, store.ErrTagNotFound) {
			return bulkStatusSkipped, "tag does not exist"
		}
		if err != nil {
			return fail("remove tag", err)
		}
		if err := rt.tags.DetachAppTag(ctx, tag.ID, svc.Name); errors.Is(err, store.ErrTagNotFound) {
			return bulkStatusSkipped, "tag not attached"
		} else if err != nil {
			return fail("remove tag", err)
		}
	case bulkActionSetEnvironment:
		return rt.bulkSetEnvironment(ctx, req.Value, svc)
	case bulkActionMoveToProject:
		return rt.bulkMoveToProject(ctx, req.Value, svc)
	case bulkActionDelete:
		if err := rt.deleteApp(ctx, svc.Name); err != nil {
			return fail("delete", err)
		}
	}
	return bulkStatusOK, ""
}

func (rt *Router) bulkSetEnvironment(ctx context.Context, value string, svc store.DesiredService) (string, string) {
	envID := ""
	if value != "" {
		env, ok := rt.resolveEnvironmentForApp(ctx, value, svc.ProjectID)
		if !ok {
			return bulkStatusSkipped, "no environment " + value + " in this app's project"
		}
		envID = env.ID
	}
	if err := rt.environments.SetServiceEnvironment(ctx, svc.Name, envID); err != nil {
		rt.logger.Error("api: bulk apps: set environment failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
		return bulkStatusError, "set environment failed"
	}
	return bulkStatusOK, ""
}

// resolveEnvironmentForApp finds value as an environment ID, or as a name
// within the app's own project (environments are project-scoped).
func (rt *Router) resolveEnvironmentForApp(ctx context.Context, value, projectID string) (store.Environment, bool) {
	if env, err := rt.environments.GetEnvironment(ctx, value); err == nil {
		return env, env.ProjectID == projectID
	}
	envs, err := rt.environments.ListEnvironmentsByProject(ctx, projectID)
	if err != nil {
		return store.Environment{}, false
	}
	for _, e := range envs {
		if e.Name == value {
			return e, true
		}
	}
	return store.Environment{}, false
}

func (rt *Router) bulkMoveToProject(ctx context.Context, projectID string, svc store.DesiredService) (string, string) {
	if err := rt.validateProjectID(ctx, projectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			return bulkStatusSkipped, "unknown project"
		}
		return bulkStatusError, "project lookup failed"
	}
	if err := rt.apps.UpdateServiceProject(ctx, svc.Name, projectID); err != nil {
		rt.logger.Error("api: bulk apps: move to project failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
		return bulkStatusError, "move failed"
	}
	if svc.EnvironmentID != "" && svc.ProjectID != projectID {
		if err := rt.environments.SetServiceEnvironment(ctx, svc.Name, ""); err != nil {
			rt.logger.Error("api: bulk apps: clear environment after move failed", slog.String("error", err.Error()), slog.String("name", svc.Name))
			return bulkStatusError, "moved, but clearing the old environment failed"
		}
	}
	return bulkStatusOK, ""
}

// recordBulkAudit writes one audit row per app for an applied bulk action;
// the route-level audit only sees the single POST /apps/bulk.
func (rt *Router) recordBulkAudit(ctx context.Context, r *http.Request, p bulkPrincipal, ability, action, name string) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: bulk audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: p.actorType, ActorID: p.actorID,
		ActorName: rt.auditActorName(ctx, p.actorType, p.actorID, p.actorID),
		Ability:   ability, Method: http.MethodPost,
		Path:       "/api/v1/apps/" + name + "/bulk-" + action,
		StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt:  store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if action == bulkActionDelete {
		entry.Method = http.MethodDelete
		entry.Path = "/api/v1/apps/" + name
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: bulk audit save failed", slog.String("error", err.Error()), slog.String("name", name))
	}
}
