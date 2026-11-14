package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/eventhook"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/sourcecache"
	"github.com/GLINCKER/levelrail/internal/stacksync"
	"github.com/GLINCKER/levelrail/internal/store"
)

const auditStackSyncSettings = "stack.sync_settings"

// stackSyncStore is the sync slice of the store; nil disables the sync routes.
type stackSyncStore interface {
	GetStackSync(ctx context.Context, stackID string) (store.StackSyncRecord, bool, error)
	PutStackSyncConfig(ctx context.Context, stackID string, c store.StackSyncConfig, now time.Time) error
	ListEnabledStackSyncs(ctx context.Context) ([]store.StackSyncRecord, error)
	SetStackSyncStatus(ctx context.Context, stackID string, st store.StackSyncStatus) error
}

func (rt *Router) stackSyncStore() stackSyncStore {
	if s, ok := rt.apps.(stackSyncStore); ok {
		return s
	}
	if s, ok := rt.policies.(stackSyncStore); ok {
		return s
	}
	return nil
}

// stackSyncers holds one syncer per router, kept off the Router struct so the
// sync slice stays in its own files.
var stackSyncers sync.Map

type stackSyncState struct {
	syncer  *stacksync.Syncer
	mu      sync.Mutex
	refresh map[string]time.Time
}

func (rt *Router) stackSyncState() *stackSyncState {
	if v, ok := stackSyncers.Load(rt); ok {
		return v.(*stackSyncState)
	}
	st := &stackSyncState{refresh: map[string]time.Time{}}
	if ss := rt.stackSyncStore(); ss != nil {
		st.syncer = &stacksync.Syncer{Store: ss, Fetch: rt.stackSyncFetch, Plan: rt.stackSyncPlan, Emit: rt.emitStackSyncEvent, Limits: rt.stackSyncLimits, Logger: rt.logger}
	}
	v, _ := stackSyncers.LoadOrStore(rt, st)
	return v.(*stackSyncState)
}

// StartStackSync starts the plan only git sync loop; it exits while no stack syncs.
func (rt *Router) StartStackSync(ctx context.Context) {
	if s := rt.stackSyncState().syncer; s != nil {
		s.Start(ctx)
	}
}

func (rt *Router) stackSyncLimits(ctx context.Context) stacksync.Limits {
	if rt.runtimeSettings == nil {
		return stacksync.DefaultLimits()
	}
	i := func(k string) int { return rt.runtimeSettings.Int(ctx, k) }
	return stacksync.Limits{
		Tick:         time.Duration(i(runtimesettings.KeyStackSyncTickSecs)) * time.Second,
		MinInterval:  time.Duration(i(runtimesettings.KeyStackSyncMinIntervalSecs)) * time.Second,
		FetchTimeout: time.Duration(i(runtimesettings.KeyStackSyncFetchTimeout)) * time.Second,
		MaxRepoBytes: int64(i(runtimesettings.KeyStackSyncMaxRepoMB)) << 20,
		MaxFileBytes: int64(i(runtimesettings.KeyStackSyncMaxFileKB)) << 10,
		MaxFiles:     i(runtimesettings.KeyStackSyncMaxFiles),
	}
}

// stackSyncFetch reuses the app git fetcher. A credential ref only lends an
// app's key or token to the exact repository that app is connected to.
func (rt *Router) stackSyncFetch(ctx context.Context, rec store.StackSyncRecord) (string, string, func(), error) {
	if rt.sourceCache == nil {
		return "", "", nil, &stacksync.CodedError{Code: iac.CodeSyncRepoUnreachable, Msg: "git fetches are not configured"}
	}
	url, token := rec.Config.RepoURL, ""
	fctx := sourcecache.WithScope(ctx, "stack:"+rec.StackID)
	if ref := rec.Config.CredentialRef; ref != "" {
		name := strings.TrimPrefix(ref, "app:")
		gs, err := rt.gitSources.GetGitSource(ctx, name)
		if err != nil || gs == nil || !sameRepo(gs.RepoURL, url) {
			return "", "", nil, &stacksync.CodedError{Code: iac.CodeSyncRepoUnreachable, Msg: "credential_ref does not name an app connected to this repository"}
		}
		if rt.gitSourceSecrets != nil {
			key := store.GitSourceSecretsKey(name)
			if has, _ := rt.gitSourceSecrets.Exists(ctx, key, gitSourceTokenKey); has {
				if token, err = rt.gitSourceSecrets.Resolve(ctx, key, gitSourceTokenKey); err != nil {
					return "", "", nil, &stacksync.CodedError{Code: iac.CodeSyncRepoUnreachable, Msg: "credential could not be read"}
				}
			}
		}
		fctx = rt.gitAuthCtx(ctx, name, url)
	}
	dir, commit, cleanup, err := rt.gitCheckout(fctx, url, rec.Config.Branch, token)
	if err != nil {
		return "", "", nil, &stacksync.CodedError{Code: iac.CodeSyncRepoUnreachable, Msg: "fetch failed"}
	}
	return dir, commit, cleanup, nil
}

func sameRepo(a, b string) bool {
	n := func(s string) string {
		return strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), "/"), ".git")
	}
	return a != "" && n(a) == n(b)
}

// stackSyncPlan is the stack plan endpoint's path with the drift loop's
// read-only live client: no apply, no prune, no deploy.
func (rt *Router) stackSyncPlan(ctx context.Context, rec store.StackSyncRecord, files []stacksync.File) (iac.Plan, error) {
	ss := rt.stackStore()
	if ss == nil {
		return iac.Plan{}, errors.New("stacks are not configured")
	}
	stack, err := ss.GetStackByName(ctx, rec.StackName)
	if err != nil {
		return iac.Plan{}, err
	}
	project := rt.projectName(ctx, stack.ProjectID)
	req := stackRequest{Files: make([]iacFile, len(files)), NoDeploy: true}
	for i, f := range files {
		req.Files[i] = iacFile{Name: f.Name, Content: f.Content}
	}
	res, opts, issues := req.iac(stack.Name, project, false).build()
	if len(issues) > 0 {
		return iac.Plan{}, &stacksync.CodedError{Code: iac.CodeSyncPlanFailed, Msg: "documents do not validate"}
	}
	if resourcesHaveCloud(res) {
		return iac.Plan{}, &stacksync.CodedError{Code: iac.CodeSyncPlanFailed, Msg: "cloud kinds are not planned by sync"}
	}
	lctx, err := rt.stackLedgerContext(ctx, ss, stack.ID, stack.Name, project, stack.Scope, res, req)
	if err != nil {
		return iac.Plan{}, err
	}
	live, release, err := rt.StackDriftLive(ctx)
	if err != nil {
		return iac.Plan{}, err
	}
	defer release()
	st, err := iac.Load(lctx, live, res, opts)
	if err != nil {
		return iac.Plan{}, err
	}
	return iac.PlanFor(st, res, opts), nil
}

// emitStackSyncEvent names the commit, counts and code, never file contents.
func (rt *Router) emitStackSyncEvent(ctx context.Context, typ string, rec store.StackSyncRecord, st store.StackSyncStatus) {
	if rt.eventWebhooks == nil {
		return
	}
	maxNames := 0
	if rt.runtimeSettings != nil {
		maxNames = rt.runtimeSettings.Int(ctx, runtimesettings.KeyStackDriftSummaryMax)
	}
	data := stackEventPayload(rec.StackName, "sync", st.PlanHash, 0, nil, st.Counts, maxNames)
	data["commit"] = st.LastSHA
	if st.Code != "" {
		data["code"] = st.Code
	}
	rt.eventWebhooks.Emit(eventhook.Event{Type: typ, Data: data})
}

type stackSyncResource struct {
	Config *iac.StackSync         `json:"config"`
	Status *stackSyncStatusOutput `json:"status,omitempty"`
}

type stackSyncStatusOutput struct {
	Commit    string         `json:"commit,omitempty"`
	CheckedAt string         `json:"checked_at,omitempty"`
	PlanHash  string         `json:"plan_hash,omitempty"`
	Counts    map[string]int `json:"counts,omitempty"`
	Pending   bool           `json:"pending"`
	Code      string         `json:"code,omitempty"`
}

func toStackSyncResource(rec store.StackSyncRecord, found bool) stackSyncResource {
	if !found {
		return stackSyncResource{Config: nil}
	}
	c := rec.Config
	out := stackSyncResource{Config: &iac.StackSync{RepoURL: c.RepoURL, Branch: c.Branch, Path: c.Path, CredentialRef: c.CredentialRef,
		IntervalSeconds: c.IntervalSeconds, WebhookOnly: c.WebhookOnly, Enabled: c.Enabled}}
	if !rec.Status.CheckedAt.IsZero() {
		out.Status = statusOutput(rec.Status)
	}
	return out
}

func statusOutput(s store.StackSyncStatus) *stackSyncStatusOutput {
	o := &stackSyncStatusOutput{Commit: s.LastSHA, PlanHash: s.PlanHash, Counts: s.Counts, Pending: s.Pending, Code: s.Code}
	if !s.CheckedAt.IsZero() {
		o.CheckedAt = s.CheckedAt.UTC().Format(time.RFC3339)
	}
	return o
}

func (rt *Router) stackSyncTarget(w http.ResponseWriter, r *http.Request) (stackSyncStore, store.StackRecord, bool) {
	sy := rt.stackSyncStore()
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return nil, store.StackRecord{}, false
	}
	if sy == nil {
		writeError(w, http.StatusNotImplemented, "stack sync is not configured on this control plane")
		return nil, store.StackRecord{}, false
	}
	stack, ok := rt.loadStack(w, r, ss)
	return sy, stack, ok
}

// handleGetStackSync handles GET /api/v1/stacks/{name}/sync.
func (rt *Router) handleGetStackSync(w http.ResponseWriter, r *http.Request) {
	sy, stack, ok := rt.stackSyncTarget(w, r)
	if !ok {
		return
	}
	rec, found, err := sy.GetStackSync(r.Context(), stack.ID)
	if err != nil {
		rt.internalError(w, "api: get stack sync failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toStackSyncResource(rec, found))
}

// handlePutStackSync handles PUT /api/v1/stacks/{name}/sync.
func (rt *Router) handlePutStackSync(w http.ResponseWriter, r *http.Request) {
	sy, stack, ok := rt.stackSyncTarget(w, r)
	if !ok {
		return
	}
	var in iac.StackSync
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeIaCError(w, http.StatusBadRequest, iac.CodeInvalidRequest, "body must be a stack sync object", nil)
		return
	}
	clean, _ := iac.CleanSyncPath(in.Path)
	if code, err := in.Validate(); err != nil {
		writeIaCError(w, http.StatusUnprocessableEntity, code, err.Error(), nil)
		return
	}
	// Lending a stored credential to a repository is an install level decision.
	if in.CredentialRef != "" && !rt.callerIsAdmin(r) {
		writeError(w, http.StatusForbidden, "only an administrator may set credential_ref")
		return
	}
	cfg := store.StackSyncConfig{RepoURL: in.RepoURL, Branch: in.Branch, Path: clean, CredentialRef: in.CredentialRef,
		IntervalSeconds: in.IntervalSeconds, WebhookOnly: in.WebhookOnly, Enabled: in.Enabled}
	if err := sy.PutStackSyncConfig(r.Context(), stack.ID, cfg, time.Now()); err != nil {
		rt.internalError(w, "api: put stack sync failed", err)
		return
	}
	rt.auditStack(r, auditStackSyncSettings, map[string]any{"stack": stack.Name, "repo_url": in.RepoURL, "branch": in.Branch,
		"path": clean, "credential_ref": in.CredentialRef, "interval_seconds": in.IntervalSeconds, "webhook_only": in.WebhookOnly, "enabled": in.Enabled})
	if in.Enabled {
		if s := rt.stackSyncState().syncer; s != nil {
			s.Kick()
		}
	}
	rec, found, err := sy.GetStackSync(r.Context(), stack.ID)
	if err != nil {
		rt.internalError(w, "api: get stack sync failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toStackSyncResource(rec, found))
}

// handleRefreshStackSync handles POST /api/v1/stacks/{name}/sync/refresh.
func (rt *Router) handleRefreshStackSync(w http.ResponseWriter, r *http.Request) {
	sy, stack, ok := rt.stackSyncTarget(w, r)
	if !ok {
		return
	}
	rec, found, err := sy.GetStackSync(r.Context(), stack.ID)
	if err != nil {
		rt.internalError(w, "api: get stack sync failed", err)
		return
	}
	if !found || !rec.Config.Enabled {
		writeIaCError(w, http.StatusConflict, iac.CodeInvalidRequest, "sync is not enabled for this stack", nil)
		return
	}
	state := rt.stackSyncState()
	cooldown := 30 * time.Second
	if rt.runtimeSettings != nil {
		cooldown = time.Duration(rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyStackSyncRefreshMinSecs)) * time.Second
	}
	state.mu.Lock()
	if last, seen := state.refresh[stack.ID]; seen && time.Since(last) < cooldown {
		state.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "a sync refresh of this stack ran recently; try again shortly")
		return
	}
	state.refresh[stack.ID] = time.Now()
	state.mu.Unlock()
	st := state.syncer.SyncOne(r.Context(), rec)
	rt.logger.Info("stack sync refreshed", slog.String("stack", stack.Name), slog.String("code", st.Code))
	writeJSON(w, http.StatusOK, statusOutput(st))
}
