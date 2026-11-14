package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudiac"
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
		st.syncer = &stacksync.Syncer{Store: ss, Fetch: rt.stackSyncFetch, Plan: rt.stackSyncPlan, Apply: rt.stackSyncApply,
			FreeBytes: rt.stackSyncFreeBytes, Emit: rt.emitStackSyncEvent, Limits: rt.stackSyncLimits, Logger: rt.logger}
	}
	v, _ := stackSyncers.LoadOrStore(rt, st)
	return v.(*stackSyncState)
}

// StartStackSync starts the git sync loop; it exits while no stack syncs.
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
		MinFreeBytes: int64(i(runtimesettings.KeyStackSyncMinFreeMB)) << 20,
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
	// The mirror may hold history the checkout does not: cap it at the repo cap too.
	fctx = sourcecache.WithMaxMirrorBytes(fctx, rt.stackSyncLimits(ctx).MaxRepoBytes)
	dir, commit, cleanup, err := rt.gitCheckout(fctx, url, rec.Config.Branch, token)
	if errors.Is(err, sourcecache.ErrOverBudget) {
		return "", "", nil, &stacksync.CodedError{Code: iac.CodeSyncLimitsExceeded, Msg: "repository is larger than the size limit"}
	}
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
// read-only live client; it never writes.
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
	// Plan what the apply would do: deletes only when the sync and the stack both allow prune.
	prune := rec.Config.Apply && rec.Config.Prune && stack.AllowPrune
	req := stackRequest{Files: make([]iacFile, len(files)), NoDeploy: true, Prune: prune}
	for i, f := range files {
		req.Files[i] = iacFile{Name: f.Name, Content: f.Content}
	}
	res, opts, issues := req.iac(stack.Name, project, prune).build()
	if is, leaked := iac.SyncSecretIssue(issues); leaked {
		return iac.Plan{}, &stacksync.CodedError{Code: iac.CodeSyncSecretInRepo, Msg: "a document holds a literal secret", Detail: fileLine(is)}
	}
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
	Config *iac.StackSync `json:"config"`
	// WebhookSecretSet says whether a secret is stored, never its value.
	WebhookSecretSet bool                   `json:"webhook_secret_set"`
	Status           *stackSyncStatusOutput `json:"status,omitempty"`
	// RefreshCooldownSeconds is the server cooldown of the refresh route, so the UI never guesses it.
	RefreshCooldownSeconds int `json:"refresh_cooldown_seconds"`
}

// stackSyncRefreshCooldown is the one place the refresh cooldown is read.
func (rt *Router) stackSyncRefreshCooldown(ctx context.Context) time.Duration {
	if rt.runtimeSettings == nil {
		return 30 * time.Second
	}
	return time.Duration(rt.runtimeSettings.Int(ctx, runtimesettings.KeyStackSyncRefreshMinSecs)) * time.Second
}

// stackSyncSecretDeleter is the optional delete side of the sealed store.
type stackSyncSecretDeleter interface {
	DeleteAll(ctx context.Context, serviceName string) error
}

// removeStackSyncSecret drops the sealed webhook secret of a deleted stack so
// a later stack never inherits it. Failure is logged, the delete already won.
func (rt *Router) removeStackSyncSecret(ctx context.Context, stackID string) {
	d, ok := rt.gitSourceSecrets.(stackSyncSecretDeleter)
	if !ok {
		return
	}
	if err := d.DeleteAll(context.WithoutCancel(ctx), store.StackSyncSecretsKey(stackID)); err != nil {
		rt.logger.Warn("api: remove stack sync webhook secret failed", slog.String("stack_id", stackID), slog.Any("error", err))
	}
}

func (rt *Router) stackSyncWebhookSecretSet(ctx context.Context, stackID string) bool {
	if rt.gitSourceSecrets == nil {
		return false
	}
	has, _ := rt.gitSourceSecrets.Exists(ctx, store.StackSyncSecretsKey(stackID), stackSyncWebhookSecretKey)
	return has
}

type stackSyncStatusOutput struct {
	Commit    string         `json:"commit,omitempty"`
	CheckedAt string         `json:"checked_at,omitempty"`
	PlanHash  string         `json:"plan_hash,omitempty"`
	Counts    map[string]int `json:"counts,omitempty"`
	Pending   bool           `json:"pending"`
	Code      string         `json:"code,omitempty"`
	// Detail is the file:line of a failure, never repository content.
	Detail     string `json:"detail,omitempty"`
	AppliedSHA string `json:"applied_commit,omitempty"`
	Revision   int    `json:"revision,omitempty"`
}

func toStackSyncResource(rec store.StackSyncRecord, found bool) stackSyncResource {
	if !found {
		return stackSyncResource{Config: nil}
	}
	c := rec.Config
	out := stackSyncResource{Config: &iac.StackSync{RepoURL: c.RepoURL, Branch: c.Branch, Path: c.Path, CredentialRef: c.CredentialRef,
		IntervalSeconds: c.IntervalSeconds, WebhookOnly: c.WebhookOnly, Enabled: c.Enabled, Apply: c.Apply, Prune: c.Prune, Deploy: c.Deploy}}
	if !rec.Status.CheckedAt.IsZero() {
		out.Status = statusOutput(rec.Status)
	}
	return out
}

func statusOutput(s store.StackSyncStatus) *stackSyncStatusOutput {
	o := &stackSyncStatusOutput{Commit: s.LastSHA, PlanHash: s.PlanHash, Counts: s.Counts, Pending: s.Pending, Code: s.Code,
		Detail: s.Detail, AppliedSHA: s.AppliedSHA, Revision: s.Revision}
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
	out := toStackSyncResource(rec, found)
	out.WebhookSecretSet = rt.stackSyncWebhookSecretSet(r.Context(), stack.ID)
	out.RefreshCooldownSeconds = int(rt.stackSyncRefreshCooldown(r.Context()) / time.Second)
	writeJSON(w, http.StatusOK, out)
}

// handlePutStackSync handles PUT /api/v1/stacks/{name}/sync.
func (rt *Router) handlePutStackSync(w http.ResponseWriter, r *http.Request) {
	sy, stack, ok := rt.stackSyncTarget(w, r)
	if !ok {
		return
	}
	var in stackSyncPut
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeIaCError(w, http.StatusBadRequest, iac.CodeInvalidRequest, "body must be a stack sync object", nil)
		return
	}
	// Each opt in must be its own explicit field; a missing one is never read as on.
	if in.Apply == nil || in.Prune == nil || in.Deploy == nil {
		writeIaCError(w, http.StatusUnprocessableEntity, iac.CodeInvalidRequest, "apply, prune and deploy must each be set to true or false", nil)
		return
	}
	in.StackSync.Apply, in.StackSync.Prune, in.StackSync.Deploy = *in.Apply, *in.Prune, *in.Deploy
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
		IntervalSeconds: in.IntervalSeconds, WebhookOnly: in.WebhookOnly, Enabled: in.Enabled,
		Apply: in.StackSync.Apply, Prune: in.StackSync.Prune, Deploy: in.StackSync.Deploy}
	if err := sy.PutStackSyncConfig(r.Context(), stack.ID, cfg, time.Now()); err != nil {
		rt.internalError(w, "api: put stack sync failed", err)
		return
	}
	rt.auditStack(r, auditStackSyncSettings, map[string]any{"stack": stack.Name, "repo_url": in.RepoURL, "branch": in.Branch,
		"path": clean, "credential_ref": in.CredentialRef, "interval_seconds": in.IntervalSeconds, "webhook_only": in.WebhookOnly, "enabled": in.Enabled,
		"apply": in.StackSync.Apply, "prune": in.StackSync.Prune, "deploy": in.StackSync.Deploy})
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
	out := toStackSyncResource(rec, found)
	out.WebhookSecretSet = rt.stackSyncWebhookSecretSet(r.Context(), stack.ID)
	out.RefreshCooldownSeconds = int(rt.stackSyncRefreshCooldown(r.Context()) / time.Second)
	writeJSON(w, http.StatusOK, out)
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
	cooldown := rt.stackSyncRefreshCooldown(r.Context())
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

// stackSyncPut is the PUT body: apply, prune and deploy are pointers so an
// omitted flag is refused instead of read as off or on.
type stackSyncPut struct {
	iac.StackSync
	Apply  *bool `json:"apply"`
	Prune  *bool `json:"prune"`
	Deploy *bool `json:"deploy"`
}

const stackSyncActorPrefix = "sync:"

func fileLine(is iac.Issue) string {
	if is.Line > 0 {
		return fmt.Sprintf("%s:%d", is.File, is.Line)
	}
	return is.File
}

// stackSyncFreeBytes is the lowest free space of the temp dir checkouts land
// in and the mirror root; a missing dir is probed through its parent.
func (rt *Router) stackSyncFreeBytes(context.Context) (int64, error) {
	dirs := []string{os.TempDir()}
	if rt.sourceCache != nil && rt.sourceCache.MirrorRoot() != "" {
		dirs = append(dirs, rt.sourceCache.MirrorRoot())
	}
	low := int64(math.MaxInt64)
	for _, d := range dirs {
		free, err := cloudiac.FreeBytes(d)
		if errors.Is(err, fs.ErrNotExist) {
			free, err = cloudiac.FreeBytes(filepath.Dir(d))
		}
		if errors.Is(err, cloudiac.ErrFreeSpaceUnsupported) {
			continue
		}
		if err != nil {
			return 0, err
		}
		low = min(low, free)
	}
	return low, nil
}

// stackSyncApply sends the synced documents through POST /stacks/{name}/apply,
// the manual apply path with its ledger, lease, approval rule and events. The
// caller is a token named sync:<stack>, scoped to this stack's apply verb only,
// revoked when the call returns; it never holds root and cannot approve.
func (rt *Router) stackSyncApply(ctx context.Context, rec store.StackSyncRecord, files []stacksync.File, commit string) (stacksync.ApplyOutcome, error) {
	fail := func(code, msg string) (stacksync.ApplyOutcome, error) {
		if !slices.Contains(iac.AllCodes, code) {
			code = iac.CodeSyncApplyFailed
		}
		return stacksync.ApplyOutcome{}, &stacksync.CodedError{Code: code, Msg: msg}
	}
	ss, ts := rt.stackStore(), rt.tokenStackScopes()
	if ss == nil || ts == nil || rt.tokens == nil {
		return fail("", "stack scoped tokens are not configured")
	}
	stack, err := ss.GetStackByName(ctx, rec.StackName)
	if err != nil {
		return fail("", "stack not found")
	}
	verbs := []string{store.StackVerbApply}
	expires := time.Now().Add(stackOIDCTTL())
	raw, tok, err := MintAPIToken(ctx, rt.tokens, stackSyncActorPrefix+stack.Name, stackTrustAbilities(verbs), &expires)
	if err != nil {
		return fail("", "could not mint the sync token")
	}
	defer func() { _ = rt.tokens.RevokeAPIToken(context.WithoutCancel(ctx), tok.ID) }()
	// A token without its stack scope would be unconfined, so it is never sent before both writes land.
	if err := ts.SetAPITokenStackScope(ctx, store.APITokenStackScope{TokenID: tok.ID, StackID: stack.ID, Verbs: verbs}); err != nil {
		return fail("", "could not scope the sync token")
	}
	if err := rt.markSystemToken(ctx, tok.ID); err != nil {
		return fail("", "could not mark the sync token")
	}
	payload, err := json.Marshal(syncApplyRequest(rec, files, commit))
	if err != nil {
		return fail("", "could not encode the apply")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/api/v1/stacks/"+url.PathEscape(stack.Name)+"/apply", bytes.NewReader(payload))
	if err != nil {
		return fail("", "could not build the apply")
	}
	req.Header.Set("Authorization", "Bearer "+raw)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "127.0.0.1:0"
	rt.iac.once.Do(func() { rt.iac.h = rt.Handler() })
	res := &bufferedResponse{header: http.Header{}, status: http.StatusOK}
	rt.iac.h.ServeHTTP(res, req)
	var out stackApplyResponse
	_ = json.Unmarshal(res.body.Bytes(), &out)
	if res.status < 200 || res.status >= 300 {
		return fail(out.Code, "apply refused")
	}
	if out.PendingApproval && out.Revision != nil {
		return stacksync.ApplyOutcome{Revision: out.Revision.Number, PendingApproval: true}, nil
	}
	if out.Revision == nil || (out.Revision.Status != string(store.StackRevApplied) && !out.Noop && !out.Replayed) {
		return fail("", "apply did not finish")
	}
	return stacksync.ApplyOutcome{Revision: out.Revision.Number}, nil
}

// syncApplyRequest maps the sync flags onto the manual apply body; the commit
// is the idempotency key so a replayed tick never applies twice.
func syncApplyRequest(rec store.StackSyncRecord, files []stacksync.File, commit string) stackRequest {
	body := stackRequest{Files: make([]iacFile, len(files)), Prune: rec.Config.Prune, NoDeploy: !rec.Config.Deploy,
		GitRepo: rec.Config.RepoURL, GitRef: rec.Config.Branch, GitSHA: commit, IdempotencyKey: "sync-" + commit}
	for i, f := range files {
		body.Files[i] = iacFile{Name: f.Name, Content: f.Content}
	}
	return body
}
