package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultStackLockTTL      = 10 * time.Minute
	defaultStackHistoryLimit = 50
	stackCIPrefix            = "ci:"
)

// Audit action names for stack verbs.
const (
	auditStackCreate   = "stack.create"
	auditStackApply    = "stack.apply"
	auditStackRollback = "stack.rollback"
	auditStackPin      = "stack.pin"
	auditStackUnlock   = "stack.unlock"
	auditStackDestroy  = "stack.destroy"
	auditStackSettings = "stack.settings"
)

var stackEnvRe = regexp.MustCompile(`\$\{\{\s*env\.([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// stackAfterLease is a test seam that runs once the lease is held, so a test
// can force two applies to overlap. It is nil in production.
var stackAfterLease func()

// StackStore is the store surface the stack handlers use.
type StackStore interface {
	CreateStack(ctx context.Context, s store.StackRecord, now time.Time) (store.StackRecord, error)
	GetStackByName(ctx context.Context, name string) (store.StackRecord, error)
	ListStacks(ctx context.Context) ([]store.StackRecord, error)
	UpdateStackSettings(ctx context.Context, id string, allowPrune bool, driftPolicy string, requireApproval bool) error
	DeleteStack(ctx context.Context, id, mode string) ([]store.StackLedgerEntry, error)
	SetStackPin(ctx context.Context, id string, revision int) error
	ClearStackPin(ctx context.Context, id string) error
	AppendStackRevision(ctx context.Context, in store.NewStackRevision, now time.Time) (store.StackRevisionRecord, error)
	SetStackRevisionStatus(ctx context.Context, stackID string, number int, next string, result []byte, now time.Time) error
	GetStackRevision(ctx context.Context, stackID string, number int) (store.StackRevisionRecord, error)
	ListStackRevisions(ctx context.Context, stackID string, limit int) ([]store.StackRevisionRecord, error)
	ListStackLedger(ctx context.Context, stackID string) ([]store.StackLedgerEntry, error)
	UpsertStackResource(ctx context.Context, e store.StackLedgerEntry) error
	DeleteStackResource(ctx context.Context, stackID, kind, key string) error
	GetStackResourceOwner(ctx context.Context, kind, key string) (store.StackLedgerEntry, bool, error)
	GetStackLock(ctx context.Context, stackID string) (store.StackLockRecord, bool, error)
	AcquireStackLock(ctx context.Context, stackID, holder string, revision int, ttl time.Duration, now time.Time) (store.StackLockRecord, error)
	RenewStackLock(ctx context.Context, stackID, holder string, ttl time.Duration, now time.Time) (store.StackLockRecord, error)
	ReleaseStackLock(ctx context.Context, stackID, holder string) error
	ForceReleaseStackLock(ctx context.Context, stackID string) error
}

func (rt *Router) stackStore() StackStore {
	if s, ok := rt.apps.(StackStore); ok {
		return s
	}
	if s, ok := rt.policies.(StackStore); ok {
		return s
	}
	return nil
}

func stackResourceFromPath(r *http.Request) string { return "stack:" + r.PathValue("name") }

// stackLockRenewEvery is how often an apply renews its lease: the TTL divided
// by APP_STACK_LOCK_RENEW_FRACTION (default 3), so one missed tick is survivable.
func stackLockRenewEvery(ttl time.Duration) time.Duration {
	fraction := 3
	if n, err := strconv.Atoi(os.Getenv("APP_STACK_LOCK_RENEW_FRACTION")); err == nil && n >= 2 {
		fraction = n
	}
	return ttl / time.Duration(fraction)
}

// errStackLeaseLost cancels an apply whose lease another holder took over.
var errStackLeaseLost = errors.New("the stack lease was lost during apply; another apply may hold it now")

// keepStackLease renews the lease until stop is called and cancels the
// returned context when a renewal finds the lease gone.
func keepStackLease(ctx context.Context, ss StackStore, stackID, holder string, ttl time.Duration) (context.Context, func() bool) {
	lctx, cancel := context.WithCancelCause(ctx)
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		t := time.NewTicker(stackLockRenewEvery(ttl))
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-lctx.Done():
				return
			case <-t.C:
				if _, err := ss.RenewStackLock(context.WithoutCancel(ctx), stackID, holder, ttl, time.Now()); errors.Is(err, store.ErrStackLockLost) {
					cancel(errStackLeaseLost)
					return
				}
			}
		}
	}()
	return lctx, func() bool {
		close(done)
		<-finished
		lost := errors.Is(context.Cause(lctx), errStackLeaseLost)
		cancel(nil)
		return lost
	}
}

// stackBeforeApply is a test seam that runs right before iac.Apply.
var stackBeforeApply func()

func stackLockTTL() time.Duration {
	if d, err := time.ParseDuration(os.Getenv("APP_STACK_LOCK_TTL")); err == nil && d > 0 {
		return d
	}
	return defaultStackLockTTL
}

func stackHistoryLimit() int {
	if n, err := strconv.Atoi(os.Getenv("APP_STACK_HISTORY_LIMIT")); err == nil && n > 0 {
		return n
	}
	return defaultStackHistoryLimit
}

// Wire types.

type stackRequest struct {
	Files            []iacFile         `json:"files"`
	Vars             map[string]string `json:"vars,omitempty"`
	Secrets          map[string]string `json:"secrets,omitempty"`
	Project          string            `json:"project,omitempty"`
	Prune            bool              `json:"prune,omitempty"`
	AllowMassPrune   bool              `json:"allow_mass_prune,omitempty"`
	Adopt            bool              `json:"adopt,omitempty"`
	NoDeploy         bool              `json:"no_deploy,omitempty"`
	ContinueOnError  bool              `json:"continue_on_error,omitempty"`
	ExpectedPlanHash string            `json:"expected_plan_hash,omitempty"`
	GitRepo          string            `json:"git_repo,omitempty"`
	GitRef           string            `json:"git_ref,omitempty"`
	GitSHA           string            `json:"git_sha,omitempty"`
	CIRunURL         string            `json:"ci_run_url,omitempty"`
	IdempotencyKey   string            `json:"idempotency_key,omitempty"`
}

type stackLockResource struct {
	Holder    string `json:"holder"`
	Revision  int    `json:"revision"`
	ExpiresAt string `json:"expires_at"`
}

type stackLedgerResource struct {
	Kind          string `json:"kind"`
	Key           string `json:"key"`
	LiveID        string `json:"live_id"`
	SinceRevision int    `json:"since_revision"`
}

type stackResource struct {
	Name            string                `json:"name"`
	ProjectID       string                `json:"project_id"`
	Project         string                `json:"project,omitempty"`
	EnvironmentID   string                `json:"environment_id,omitempty"`
	AllowPrune      bool                  `json:"allow_prune"`
	DriftPolicy     string                `json:"drift_policy"`
	PinnedRevision  int                   `json:"pinned_revision,omitempty"`
	RequireApproval bool                  `json:"require_approval"`
	CreatedAt       string                `json:"created_at"`
	CreatedBy       string                `json:"created_by"`
	LatestRevision  int                   `json:"latest_revision"`
	LatestStatus    string                `json:"latest_status,omitempty"`
	Lock            *stackLockResource    `json:"lock,omitempty"`
	Drift           stackDriftResource    `json:"drift"`
	Resources       []stackLedgerResource `json:"resources,omitempty"`
}

type stackRevisionSummary struct {
	Number         int    `json:"number"`
	Status         string `json:"status"`
	PlanHash       string `json:"plan_hash"`
	RollbackOf     int    `json:"rollback_of,omitempty"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	Actor          string `json:"actor"`
	TokenID        string `json:"token_id,omitempty"`
	GitRepo        string `json:"git_repo,omitempty"`
	GitRef         string `json:"git_ref,omitempty"`
	GitSHA         string `json:"git_sha,omitempty"`
	CIRunURL       string `json:"ci_run_url,omitempty"`
	CreatedAt      string `json:"created_at"`
	FinishedAt     string `json:"finished_at,omitempty"`
	// ExpiresAt is when a pending_approval revision stops being approvable.
	ExpiresAt string `json:"expires_at,omitempty"`
	// Rejected marks a pending revision a reviewer turned down; its status stays failed.
	Rejected bool `json:"rejected,omitempty"`
}

type stackPlanResponse struct {
	Stack       string      `json:"stack"`
	StackExists bool        `json:"stack_exists"`
	Pinned      int         `json:"pinned_revision,omitempty"`
	Plan        *iac.Plan   `json:"plan,omitempty"`
	Issues      []iac.Issue `json:"issues,omitempty"`
	Warnings    []string    `json:"warnings,omitempty"`
}

type stackApplyResponse struct {
	Stack    string                `json:"stack"`
	Revision *stackRevisionSummary `json:"revision,omitempty"`
	Result   *iac.ApplyResult      `json:"result,omitempty"`
	Issues   []iac.Issue           `json:"issues,omitempty"`
	Replayed bool                  `json:"replayed,omitempty"`
	// Noop answers 200 when nothing changed: Revision is the existing latest one.
	Noop     bool     `json:"noop,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	// PendingApproval answers 202: the plan is stored and nothing was applied.
	PendingApproval bool      `json:"pending_approval,omitempty"`
	ExpiresAt       string    `json:"expires_at,omitempty"`
	Plan            *iac.Plan `json:"plan,omitempty"`
}

type stackCreateRequest struct {
	Name            string `json:"name"`
	Project         string `json:"project"`
	AllowPrune      bool   `json:"allow_prune,omitempty"`
	DriftPolicy     string `json:"drift_policy,omitempty"`
	RequireApproval bool   `json:"require_approval,omitempty"`
}

type stackSettingsRequest struct {
	AllowPrune      *bool   `json:"allow_prune,omitempty"`
	DriftPolicy     *string `json:"drift_policy,omitempty"`
	RequireApproval *bool   `json:"require_approval,omitempty"`
}

type stackPinRequest struct {
	Revision int `json:"revision"`
}

type stackDeleteResponse struct {
	Stack     string   `json:"stack"`
	Mode      string   `json:"mode"`
	Destroyed []string `json:"destroyed,omitempty"`
	Forgotten []string `json:"forgotten,omitempty"`
	Failed    []string `json:"failed,omitempty"`
	// DataKept lists databases a destroy left alone because delete_data was not set.
	DataKept []string `json:"data_kept,omitempty"`
}

func fmtStackTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func summarizeRevision(v store.StackRevisionRecord) stackRevisionSummary {
	out := stackRevisionSummary{
		Number: v.Number, Status: v.Status, PlanHash: v.PlanHash, RollbackOf: v.RollbackOf, IdempotencyKey: v.IdempotencyKey,
		Actor: v.Actor, TokenID: v.TokenID, GitRepo: v.GitRepo, GitRef: v.GitRef, GitSHA: v.GitSHA, CIRunURL: v.CIRunURL,
		CreatedAt: fmtStackTime(v.CreatedAt),
	}
	if v.FinishedAt != nil {
		out.FinishedAt = fmtStackTime(*v.FinishedAt)
	}
	if v.Status == store.StackRevPendingApproval {
		out.ExpiresAt = fmtStackTime(approvalExpiry(v))
	}
	out.Rejected = v.Status == store.StackRevFailed && revisionRejected(v.ResultJSON)
	return out
}

// Error mapping: the store cannot import iac, so its sentinels are translated here.

func mapStackStoreError(err error) error {
	switch {
	case errors.Is(err, store.ErrStackLocked):
		return iac.ErrStackLocked
	case errors.Is(err, store.ErrStackLedgerConflict):
		return fmt.Errorf("%w: %s", iac.ErrLedgerConflict, err.Error())
	}
	return err
}

// writeStackError answers the errors every stack handler can hit and
// reports whether it wrote a response.
func (rt *Router) writeStackError(w http.ResponseWriter, what string, err error) {
	err = mapStackStoreError(err)
	switch {
	case errors.Is(err, store.ErrStackNotFound), errors.Is(err, store.ErrStackRevisionNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrStackExists), errors.Is(err, iac.ErrStackLocked), errors.Is(err, iac.ErrStackPinned),
		errors.Is(err, iac.ErrLedgerConflict), errors.Is(err, iac.ErrPlanChanged), errors.Is(err, store.ErrStackBadTransition):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, iac.ErrPruneNotAllowed), errors.Is(err, store.ErrStackSecretLeak):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		rt.internalError(w, "api: stack "+what+" failed", err)
	}
}

func (rt *Router) stackStoreOr501(w http.ResponseWriter) (StackStore, bool) {
	ss := rt.stackStore()
	if ss == nil || rt.projects == nil {
		writeError(w, http.StatusNotImplemented, "stacks are not available on this store")
		return nil, false
	}
	return ss, true
}

func (rt *Router) projectName(ctx context.Context, id string) string {
	p, err := rt.projects.GetProject(ctx, id)
	if err != nil {
		return ""
	}
	return p.Name
}

func (rt *Router) projectByName(ctx context.Context, name string) (store.Project, bool) {
	list, err := rt.projects.ListProjects(ctx)
	if err != nil {
		return store.Project{}, false
	}
	for _, p := range list {
		if p.Name == name {
			return p, true
		}
	}
	return store.Project{}, false
}

func stackOf(s store.StackRecord) *iac.Stack {
	v := toIaCStack(s)
	return &v
}

func toIaCStack(s store.StackRecord) iac.Stack {
	return iac.Stack{
		ID: s.ID, Name: s.Name, ProjectID: s.ProjectID, EnvironmentID: s.EnvironmentID, AllowPrune: s.AllowPrune,
		DriftPolicy: iac.DriftPolicy(s.DriftPolicy), PinnedRevision: s.PinnedRevision, RequireApproval: s.RequireApproval,
		CreatedAt: s.CreatedAt, CreatedBy: s.CreatedBy,
	}
}

// stackCaller identifies who acts. isCI is true for tokens minted by the CI exchange.
type stackCaller struct {
	kind, id, name string
	// userID is the session user or the token's owner, empty when unknown.
	userID string
	isCI   bool
}

func (rt *Router) stackCaller(r *http.Request) stackCaller {
	t, id, name, ok := rt.currentActor(r)
	if !ok {
		return stackCaller{kind: "unknown", id: "unknown", name: "unknown"}
	}
	c := stackCaller{kind: t, id: id, name: name, isCI: t == store.PrincipalTypeToken && strings.HasPrefix(name, stackCIPrefix)}
	switch t {
	case store.PrincipalTypeUser:
		c.userID = id
	case store.PrincipalTypeToken:
		if token, ok := bearerToken(r); ok {
			if rec, err := rt.lookupBearerToken(r.Context(), token); err == nil && rec.ID == id {
				c.userID = rec.OwnerUserID
			}
		}
	}
	return c
}

func (c stackCaller) tokenID() string {
	if c.kind == store.PrincipalTypeToken {
		return c.id
	}
	return ""
}

func (rt *Router) auditStack(r *http.Request, action string, detail map[string]any) {
	c := rt.stackCaller(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: stack audit id failed", slog.String("error", err.Error()))
		return
	}
	raw, _ := json.Marshal(detail)
	entry := store.AuditEntry{
		ID: id, ActorType: c.kind, ActorID: c.id, ActorName: c.name,
		Ability: AbilityWrite, Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")), Action: action, Detail: string(raw),
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: stack audit save failed", slog.String("error", err.Error()), slog.String("action", action))
	}
}

func decodeStackRequest(w http.ResponseWriter, r *http.Request, filesOptional bool) (stackRequest, bool) {
	var req stackRequest
	err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIaCBodyBytes)).Decode(&req)
	if err != nil && (!filesOptional || !errors.Is(err, io.EOF)) {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return req, false
	}
	if !filesOptional && (len(req.Files) == 0 || len(req.Files) > maxIaCFiles) {
		writeError(w, http.StatusBadRequest, "files must hold between 1 and "+strconv.Itoa(maxIaCFiles)+" entries")
		return req, false
	}
	if req.IdempotencyKey == "" {
		req.IdempotencyKey = r.Header.Get(headerIdempotencyKey)
	}
	if len(req.IdempotencyKey) > idempotencyKeyMaxLen {
		writeError(w, http.StatusBadRequest, "idempotency key must be at most 255 characters")
		return req, false
	}
	return req, true
}

func (req stackRequest) iac(stack, project string, prune bool) iacRequest {
	return iacRequest{
		Files: req.Files, Source: stack, Project: project, Prune: prune, Vars: req.Vars, Secrets: req.Secrets,
		NoDeploy: req.NoDeploy, ContinueOnError: req.ContinueOnError,
	}
}

// renderStackFiles substitutes ${{ env.NAME }} so a revision replays without vars.
func renderStackFiles(files []iacFile, vars map[string]string) []iacFile {
	out := make([]iacFile, len(files))
	for i, f := range files {
		content := stackEnvRe.ReplaceAllStringFunc(f.Content, func(m string) string {
			if v, ok := vars[stackEnvRe.FindStringSubmatch(m)[1]]; ok {
				return v
			}
			return m
		})
		out[i] = iacFile{Name: f.Name, Content: content}
	}
	return out
}

func (rt *Router) loadStack(w http.ResponseWriter, r *http.Request, ss StackStore) (store.StackRecord, bool) {
	s, err := ss.GetStackByName(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.writeStackError(w, "get", err)
		return s, false
	}
	rt.expireStackApprovals(r.Context(), ss, s)
	return s, true
}

func (rt *Router) stackResource(ctx context.Context, ss StackStore, s store.StackRecord, withLedger bool) stackResource {
	out := stackResource{
		Name: s.Name, ProjectID: s.ProjectID, Project: rt.projectName(ctx, s.ProjectID), EnvironmentID: s.EnvironmentID,
		AllowPrune: s.AllowPrune, DriftPolicy: s.DriftPolicy, PinnedRevision: s.PinnedRevision, RequireApproval: s.RequireApproval,
		CreatedAt: fmtStackTime(s.CreatedAt), CreatedBy: s.CreatedBy,
	}
	if revs, err := ss.ListStackRevisions(ctx, s.ID, 1); err == nil && len(revs) > 0 {
		out.LatestRevision, out.LatestStatus = revs[0].Number, revs[0].Status
	}
	if l, ok, err := ss.GetStackLock(ctx, s.ID); err == nil && ok && !l.Expired(time.Now()) {
		out.Lock = &stackLockResource{Holder: l.Holder, Revision: l.Revision, ExpiresAt: fmtStackTime(l.ExpiresAt)}
	}
	out.Drift = stackDriftOf(ctx, ss, s.ID)
	if withLedger {
		if entries, err := ss.ListStackLedger(ctx, s.ID); err == nil {
			for _, e := range entries {
				out.Resources = append(out.Resources, stackLedgerResource{Kind: e.Kind, Key: e.Key, LiveID: e.LiveID, SinceRevision: e.SinceRevision})
			}
		}
	}
	return out
}

func (rt *Router) handleListStacks(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	list, err := ss.ListStacks(r.Context())
	if err != nil {
		rt.writeStackError(w, "list", err)
		return
	}
	out := make([]stackResource, 0, len(list))
	for _, s := range list {
		out = append(out, rt.stackResource(r.Context(), ss, s, false))
	}
	writeJSON(w, http.StatusOK, map[string]any{"stacks": out})
}

func (rt *Router) handleGetStack(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.stackResource(r.Context(), ss, s, true))
}

// createStack inserts a stack, checking the name, project and admin-owned settings.
func (rt *Router) createStack(r *http.Request, ss StackStore, in stackCreateRequest, projectID string) (store.StackRecord, error) {
	policy, err := iac.ParseDriftPolicy(in.DriftPolicy)
	if err != nil {
		return store.StackRecord{}, err
	}
	rec := store.StackRecord{
		Name: in.Name, ProjectID: projectID, AllowPrune: in.AllowPrune, DriftPolicy: string(policy),
		RequireApproval: in.RequireApproval, CreatedBy: rt.stackCaller(r).name,
	}
	probe := toIaCStack(rec)
	probe.ID = "probe"
	if err := probe.Validate(); err != nil {
		return store.StackRecord{}, err
	}
	return ss.CreateStack(r.Context(), rec, time.Now())
}

func (rt *Router) handleCreateStack(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	var in stackCreateRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIaCBodyBytes)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if err := iac.ValidateStackName(in.Name); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	proj, found := rt.projectByName(r.Context(), in.Project)
	if !found {
		writeError(w, http.StatusUnprocessableEntity, fmt.Sprintf("project %q was not found", in.Project))
		return
	}
	if (in.AllowPrune || in.RequireApproval || in.DriftPolicy != "") && !rt.callerIsAdmin(r) {
		writeError(w, http.StatusForbidden, "only an administrator may set allow_prune, drift_policy or require_approval")
		return
	}
	rec, err := rt.createStack(r, ss, in, proj.ID)
	if err != nil {
		if errors.Is(err, store.ErrStackExists) {
			rt.writeStackError(w, "create", err)
			return
		}
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	rt.auditStack(r, auditStackCreate, map[string]any{"stack": rec.Name, "project": proj.Name})
	writeJSON(w, http.StatusCreated, rt.stackResource(r.Context(), ss, rec, false))
}

func (rt *Router) handleUpdateStackSettings(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	if rt.stackCaller(r).isCI {
		writeError(w, http.StatusForbidden, "a CI token cannot change stack settings")
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	var in stackSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIaCBodyBytes)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if in.AllowPrune != nil {
		s.AllowPrune = *in.AllowPrune
	}
	if in.RequireApproval != nil {
		s.RequireApproval = *in.RequireApproval
	}
	if in.DriftPolicy != nil {
		p, err := iac.ParseDriftPolicy(*in.DriftPolicy)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		s.DriftPolicy = string(p)
	}
	if err := ss.UpdateStackSettings(r.Context(), s.ID, s.AllowPrune, s.DriftPolicy, s.RequireApproval); err != nil {
		rt.writeStackError(w, "settings", err)
		return
	}
	rt.auditStack(r, auditStackSettings, map[string]any{"stack": s.Name, "allow_prune": s.AllowPrune, "drift_policy": s.DriftPolicy, "require_approval": s.RequireApproval})
	writeJSON(w, http.StatusOK, rt.stackResource(r.Context(), ss, s, false))
}

func (rt *Router) handleStackPlan(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	req, ok := decodeStackRequest(w, r, false)
	if !ok {
		return
	}
	name := r.PathValue("name")
	resp := stackPlanResponse{Stack: name}
	project := req.Project
	prune := req.Prune
	stack, err := ss.GetStackByName(r.Context(), name)
	switch {
	case err == nil:
		resp.StackExists, resp.Pinned = true, stack.PinnedRevision
		project = rt.projectName(r.Context(), stack.ProjectID)
		if perr := stackOf(stack).PruneAllowed(prune); prune && perr != nil {
			resp.Warnings = append(resp.Warnings, perr.Error()+"; the plan is shown without prune")
			prune = false
		}
	case errors.Is(err, store.ErrStackNotFound):
		if prune {
			resp.Warnings = append(resp.Warnings, "the stack does not exist yet; prune stays off until an administrator allows it")
			prune = false
		}
		p, create, ok := rt.resolveNewStack(w, r, req)
		if !ok {
			return
		}
		project = p
		if create {
			resp.Warnings = append(resp.Warnings, fmt.Sprintf("project %q does not exist yet; the first apply creates it, then the stack", project))
		}
	default:
		rt.writeStackError(w, "plan", err)
		return
	}
	res, opts, issues := req.iac(name, project, prune).build()
	if len(issues) > 0 {
		resp.Issues = issues
		writeJSON(w, http.StatusUnprocessableEntity, resp)
		return
	}
	if !resp.StackExists && len(res) == 0 {
		writeError(w, http.StatusUnprocessableEntity, emptyStackReason(req, project))
		return
	}
	lctx, err := rt.stackLedgerContext(r.Context(), ss, stack.ID, name, project, res, req)
	if err != nil {
		rt.writeStackError(w, "plan", err)
		return
	}
	st, err := iac.Load(lctx, rt.iacDoer(r), res, opts)
	if err != nil {
		rt.internalError(w, "api: stack plan failed", err)
		return
	}
	plan := iac.PlanFor(st, res, opts)
	resp.Plan = &plan
	writeJSON(w, http.StatusOK, resp)
}

// stackRun is one apply or rollback after request decoding.
type stackRun struct {
	stack      store.StackRecord
	req        stackRequest
	rollbackOf int
	action     string
	warnings   []string
	// pending is the revision an approval is applying; nil for a fresh apply.
	pending *store.StackRevisionRecord
}

func newHolder(c stackCaller) string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return c.kind + ":" + c.id + ":" + hex.EncodeToString(b)
}

func (rt *Router) handleStackApply(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	req, ok := decodeStackRequest(w, r, false)
	if !ok {
		return
	}
	stack, err := ss.GetStackByName(r.Context(), r.PathValue("name"))
	if errors.Is(err, store.ErrStackNotFound) {
		stack, ok = rt.createStackOnApply(w, r, ss, req)
		if !ok {
			return
		}
	} else if err != nil {
		rt.writeStackError(w, "apply", err)
		return
	}
	rt.runStack(w, r, ss, stackRun{stack: stack, req: req, action: auditStackApply})
}

// createStackOnApply makes a first apply self-contained: it resolves the project
// (creating it when the files declare it and the caller may), refuses an empty
// plan, then creates the stack. Nothing is written before every check passes.
func (rt *Router) createStackOnApply(w http.ResponseWriter, r *http.Request, ss StackStore, req stackRequest) (store.StackRecord, bool) {
	name := r.PathValue("name")
	if err := iac.ValidateStackName(name); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return store.StackRecord{}, false
	}
	project, create, ok := rt.resolveNewStack(w, r, req)
	if !ok {
		return store.StackRecord{}, false
	}
	res, _, issues := req.iac(name, project, false).build()
	if len(issues) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, stackApplyResponse{Stack: name, Issues: issues})
		return store.StackRecord{}, false
	}
	if len(res) == 0 {
		writeError(w, http.StatusUnprocessableEntity, emptyStackReason(req, project))
		return store.StackRecord{}, false
	}
	proj, found := rt.projectByName(r.Context(), project)
	if create {
		if proj, ok = rt.createProjectForStack(w, r, project); !ok {
			return store.StackRecord{}, false
		}
	} else if !found {
		writeError(w, http.StatusUnprocessableEntity, missingProjectMessage(project))
		return store.StackRecord{}, false
	}
	rec, err := rt.createStack(r, ss, stackCreateRequest{Name: name, Project: project}, proj.ID)
	if errors.Is(err, store.ErrStackExists) {
		rec, err = ss.GetStackByName(r.Context(), name)
	}
	if err != nil {
		rt.writeStackError(w, "create", err)
		return store.StackRecord{}, false
	}
	return rec, true
}

// runStack is the shared apply path for apply and rollback: pin check, lease,
// plan hash under the lease, revision, engine, ledger, status and audit.
func (rt *Router) runStack(w http.ResponseWriter, r *http.Request, ss StackStore, run stackRun) {
	ctx := r.Context()
	stack, req := run.stack, run.req
	caller := rt.stackCaller(r)
	if err := stackOf(stack).CheckPin(run.rollbackOf); err != nil {
		rt.writeStackError(w, run.action, err)
		return
	}
	if req.Prune {
		if err := stackOf(stack).PruneAllowed(true); err != nil {
			rt.writeStackError(w, run.action, err)
			return
		}
	}
	project := rt.projectName(ctx, stack.ProjectID)
	res, opts, issues := req.iac(stack.Name, project, req.Prune).build()
	if len(issues) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, stackApplyResponse{Stack: stack.Name, Issues: issues})
		return
	}
	if req.IdempotencyKey != "" {
		if prev, found := rt.findStackRevisionByKey(ctx, ss, stack.ID, req.IdempotencyKey); found {
			s := summarizeRevision(prev)
			writeJSON(w, http.StatusOK, stackApplyResponse{Stack: stack.Name, Revision: &s, Replayed: true})
			return
		}
	}

	if run.pending == nil {
		if cur, same := rt.stackNoop(ctx, ss, run, res, opts, project, rt.iacDoer(r)); same {
			s := summarizeRevision(cur)
			writeJSON(w, http.StatusOK, stackApplyResponse{Stack: stack.Name, Revision: &s, Noop: true, Warnings: run.warnings})
			return
		}
	}

	if stack.RequireApproval && run.pending == nil {
		rt.requestStackApproval(w, r, ss, run, res, opts)
		return
	}

	next := 1
	if run.pending != nil {
		next = run.pending.Number
	} else if revs, err := ss.ListStackRevisions(ctx, stack.ID, 1); err == nil && len(revs) > 0 {
		next = revs[0].Number + 1
	}
	holder := newHolder(caller)
	if _, err := ss.AcquireStackLock(ctx, stack.ID, holder, next, stackLockTTL(), time.Now()); err != nil {
		rt.writeStackError(w, run.action, err)
		return
	}
	defer func() { _ = ss.ReleaseStackLock(context.WithoutCancel(ctx), stack.ID, holder) }()
	if stackAfterLease != nil {
		stackAfterLease()
	}

	// State may have moved while waiting for the lease: re-read the pin.
	if fresh, err := ss.GetStackByName(ctx, stack.Name); err == nil {
		if perr := stackOf(fresh).CheckPin(run.rollbackOf); perr != nil {
			rt.writeStackError(w, run.action, perr)
			return
		}
	}

	if run.pending != nil {
		if !rt.pendingStillApprovable(ctx, w, ss, stack, run.pending.Number) {
			return
		}
	}

	// The ledger is read under the lease so ownership cannot move mid-apply.
	lctx, err := rt.stackLedgerContext(ctx, ss, stack.ID, stack.Name, project, res, req)
	if err != nil {
		rt.writeStackError(w, run.action, err)
		return
	}
	doer := rt.iacDoer(r)
	st, err := iac.Load(lctx, doer, res, opts)
	if err != nil {
		rt.internalError(w, "api: stack apply load failed", err)
		return
	}
	plan := iac.PlanFor(st, res, opts)
	if keys := iac.LedgerConflicts(plan); len(keys) > 0 {
		rt.writeStackError(w, run.action, fmt.Errorf("%w: %s", iac.ErrLedgerConflict, strings.Join(keys, ", ")))
		return
	}
	if iac.MassPruneBlocked(plan) {
		rt.writeStackError(w, run.action, fmt.Errorf("%w: it exceeds the mass-prune guard (APP_STACK_PRUNE_MAX_PERCENT, APP_STACK_PRUNE_MAX_COUNT); plan to review, then pass allow_mass_prune", iac.ErrPruneNotAllowed))
		return
	}
	if req.ExpectedPlanHash != "" && req.ExpectedPlanHash != plan.Hash {
		rt.writeStackError(w, run.action, iac.ErrPlanChanged)
		return
	}
	rev, ok := rt.startStackRevision(ctx, w, ss, run, caller, plan)
	if !ok {
		return
	}
	if rev.Replayed {
		s := summarizeRevision(rev)
		writeJSON(w, http.StatusOK, stackApplyResponse{Stack: stack.Name, Revision: &s, Replayed: true})
		return
	}

	leaseCtx, stopLease := keepStackLease(lctx, ss, stack.ID, holder, stackLockTTL())
	if stackBeforeApply != nil {
		stackBeforeApply()
	}
	applyCtx := iac.WithStackScope(leaseCtx, iac.StackScope{StackID: stack.ID, StackName: stack.Name, Revision: rev.Number})
	out, aerr := iac.Apply(applyCtx, doer, res, opts, plan.Hash)
	leaseLost := stopLease()
	status := store.StackRevFailed
	var resultJSON []byte
	if leaseLost {
		// Ownership may have moved under another holder: record nothing in the ledger.
		resultJSON, _ = json.Marshal(map[string]any{"error": errStackLeaseLost.Error(), "result": out})
		aerr = errStackLeaseLost
	} else if aerr == nil {
		resultJSON, _ = json.Marshal(out)
		switch {
		case out.OK():
			status = store.StackRevApplied
		case out.Applied > 0:
			status = store.StackRevPartial
		}
	}
	fin := context.WithoutCancel(ctx)
	if err := ss.SetStackRevisionStatus(fin, stack.ID, rev.Number, status, resultJSON, time.Now()); err != nil {
		rt.logger.Warn("api: stack revision status failed", slog.String("error", err.Error()), slog.String("stack", stack.Name))
	}
	warnings := run.warnings
	if aerr == nil {
		warnings = append(warnings, rt.recordStackLedger(fin, ss, stack.ID, rev.Number, res, out)...)
	}
	rt.auditStack(r, run.action, map[string]any{
		"stack": stack.Name, "revision": rev.Number, "plan_hash": plan.Hash, "git_sha": req.GitSHA, "status": status, "rollback_of": run.rollbackOf,
		"counts": map[string]int{"create": plan.Summary.Create, "update": plan.Summary.Update, "delete": plan.Summary.Delete, "noop": plan.Summary.Noop, "error": plan.Summary.Error},
	})
	if leaseLost {
		writeError(w, http.StatusConflict, errStackLeaseLost.Error()+"; the revision was marked failed, re-plan and apply again")
		return
	}
	if aerr != nil {
		rt.internalError(w, "api: stack apply failed", aerr)
		return
	}
	rev.Status = status
	now := time.Now()
	rev.FinishedAt = &now
	s := summarizeRevision(rev)
	writeJSON(w, http.StatusOK, stackApplyResponse{Stack: stack.Name, Revision: &s, Result: &out, Warnings: warnings})
}

func (rt *Router) findStackRevisionByKey(ctx context.Context, ss StackStore, stackID, key string) (store.StackRevisionRecord, bool) {
	revs, err := ss.ListStackRevisions(ctx, stackID, 0)
	if err != nil {
		return store.StackRevisionRecord{}, false
	}
	for _, v := range revs {
		if v.IdempotencyKey == key {
			return v, true
		}
	}
	return store.StackRevisionRecord{}, false
}

// stackLedgerContext attaches the stack's ownership view: its own ledger rows
// plus the owner of every declared key another stack holds. Lookups fail closed.
func (rt *Router) stackLedgerContext(ctx context.Context, ss StackStore, stackID, stackName, project string, res []*iac.Resource, req stackRequest) (context.Context, error) {
	var owned []iac.LedgerEntry
	if stackID != "" {
		rows, err := ss.ListStackLedger(ctx, stackID)
		if err != nil {
			return nil, err
		}
		for _, e := range rows {
			owned = append(owned, iac.LedgerEntry{StackID: e.StackID, Kind: iac.Kind(e.Kind), Key: e.Key, LiveID: e.LiveID, SinceRevision: e.SinceRevision})
		}
	}
	l := iac.NewLedger(stackID, stackName, project, owned)
	l.Adopt, l.AllowMassPrune = req.Adopt, req.AllowMassPrune
	for _, ref := range iac.LedgerRefs(res) {
		owner, found, err := ss.GetStackResourceOwner(ctx, string(ref.Kind), ref.Key)
		if err != nil {
			return nil, err
		}
		if found && owner.StackID != stackID {
			l.Foreign[ref.Key] = owner.StackID
		}
	}
	// A pruned domain is checked against the app it was claimed on.
	for _, e := range owned {
		if e.Kind != iac.KindDomain || e.LiveID == "" {
			continue
		}
		appKey := "App/" + e.LiveID
		if _, seen := l.Foreign[appKey]; seen {
			continue
		}
		owner, found, err := ss.GetStackResourceOwner(ctx, string(iac.KindApp), appKey)
		if err != nil {
			return nil, err
		}
		if found && owner.StackID != stackID {
			l.Foreign[appKey] = owner.StackID
		}
	}
	if len(l.Foreign) > 0 {
		if all, err := ss.ListStacks(ctx); err == nil {
			names := make(map[string]string, len(all))
			for _, s := range all {
				names[s.ID] = s.Name
			}
			for k, id := range l.Foreign {
				if n, ok := names[id]; ok {
					l.Foreign[k] = n
				}
			}
		}
	}
	return iac.WithLedger(ctx, l), nil
}

// recordStackLedger claims every declared resource this apply created or
// confirmed, of any kind, and releases what it pruned.
func (rt *Router) recordStackLedger(ctx context.Context, ss StackStore, stackID string, rev int, res []*iac.Resource, out iac.ApplyResult) []string {
	var warnings []string
	u := iac.LedgerChanges(stackID, rev, res, out)
	for _, e := range u.Release {
		if err := ss.DeleteStackResource(ctx, stackID, string(e.Kind), e.Key); err != nil {
			warnings = append(warnings, "ledger release failed for "+e.Key)
		}
	}
	for _, e := range u.Claim {
		err := ss.UpsertStackResource(ctx, store.StackLedgerEntry{StackID: stackID, Kind: string(e.Kind), Key: e.Key, LiveID: e.LiveID, SinceRevision: rev})
		if err != nil {
			warnings = append(warnings, mapStackStoreError(err).Error())
		}
	}
	return warnings
}

func (rt *Router) handleStackPin(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	var in stackPinRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxIaCBodyBytes)).Decode(&in); err != nil || in.Revision < 1 {
		writeError(w, http.StatusBadRequest, "body must be {\"revision\": n} with n >= 1")
		return
	}
	if err := ss.SetStackPin(r.Context(), s.ID, in.Revision); err != nil {
		rt.writeStackError(w, "pin", err)
		return
	}
	s.PinnedRevision = in.Revision
	rt.auditStack(r, auditStackPin, map[string]any{"stack": s.Name, "revision": in.Revision, "pinned": true})
	writeJSON(w, http.StatusOK, rt.stackResource(r.Context(), ss, s, false))
}

func (rt *Router) handleStackUnpin(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	if err := ss.ClearStackPin(r.Context(), s.ID); err != nil {
		rt.writeStackError(w, "unpin", err)
		return
	}
	prev := s.PinnedRevision
	s.PinnedRevision = 0
	rt.auditStack(r, auditStackPin, map[string]any{"stack": s.Name, "revision": prev, "pinned": false})
	writeJSON(w, http.StatusOK, rt.stackResource(r.Context(), ss, s, false))
}

func (rt *Router) handleStackUnlock(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	if rt.stackCaller(r).isCI {
		writeError(w, http.StatusForbidden, "a CI token cannot force-release a stack lock")
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	held, _, _ := ss.GetStackLock(r.Context(), s.ID)
	if err := ss.ForceReleaseStackLock(r.Context(), s.ID); err != nil {
		rt.writeStackError(w, "unlock", err)
		return
	}
	rt.auditStack(r, auditStackUnlock, map[string]any{"stack": s.Name, "holder": held.Holder, "revision": held.Revision})
	writeJSON(w, http.StatusOK, rt.stackResource(r.Context(), ss, s, false))
}

func (rt *Router) handleDeleteStack(w http.ResponseWriter, r *http.Request) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return
	}
	mode, err := iac.ParseStackDeleteMode(r.URL.Query().Get("mode"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	caller := rt.stackCaller(r)
	// Forget releases the whole ledger for adoption, so it is gated like destroy.
	if caller.isCI || !rt.callerIsAdmin(r) {
		writeError(w, http.StatusForbidden, "deleting a stack needs an administrator session or token and is never allowed from CI")
		return
	}
	deleteData := r.URL.Query().Get("delete_data") == "true"
	if deleteData && mode != iac.StackDestroy {
		writeError(w, http.StatusBadRequest, "delete_data only applies with mode=destroy")
		return
	}
	if deleteData && !rt.callerHasAbility(r, AbilityWriteSensitive) {
		writeError(w, http.StatusForbidden, "deleting database data needs the root or write:sensitive ability")
		return
	}
	s, ok := rt.loadStack(w, r, ss)
	if !ok {
		return
	}
	holder := newHolder(caller)
	if _, err := ss.AcquireStackLock(r.Context(), s.ID, holder, s.PinnedRevision, stackLockTTL(), time.Now()); err != nil {
		rt.writeStackError(w, "delete", err)
		return
	}
	defer func() { _ = ss.ReleaseStackLock(context.WithoutCancel(r.Context()), s.ID, holder) }()
	ledger, err := ss.ListStackLedger(r.Context(), s.ID)
	if err != nil {
		rt.writeStackError(w, "delete", err)
		return
	}
	resp := stackDeleteResponse{Stack: s.Name, Mode: string(mode)}
	doer := rt.iacDoer(r)
	// Apps go before databases so the dependency check on a database sees them gone.
	sort.SliceStable(ledger, func(i, j int) bool {
		return ledger[i].Kind == string(iac.KindApp) && ledger[j].Kind != string(iac.KindApp)
	})
	dbsDeleted := 0
	for _, e := range ledger {
		label := e.Kind + "/" + e.Key
		isDB := e.Kind == string(iac.KindDatabase)
		if mode != iac.StackDestroy || (e.Kind != string(iac.KindApp) && (!isDB || !deleteData)) {
			resp.Forgotten = append(resp.Forgotten, label)
			if isDB && mode == iac.StackDestroy {
				resp.DataKept = append(resp.DataKept, label)
			}
			continue
		}
		path := "/api/v1/apps/" + e.LiveID
		if isDB {
			path = "/api/v1/databases/" + e.LiveID + "?delete_data=true&confirm=" + url.QueryEscape(e.LiveID)
		}
		derr := doer.Do(r.Context(), http.MethodDelete, path, nil, nil)
		var se *iac.StatusError
		if derr != nil && (!errors.As(derr, &se) || se.Status != http.StatusNotFound) {
			resp.Failed = append(resp.Failed, label+": "+derr.Error())
			continue
		}
		if isDB {
			dbsDeleted++
		}
		resp.Destroyed = append(resp.Destroyed, label)
		_ = ss.DeleteStackResource(r.Context(), s.ID, e.Kind, e.Key)
	}
	if len(resp.Failed) > 0 {
		writeJSON(w, http.StatusBadGateway, resp)
		return
	}
	if _, err := ss.DeleteStack(r.Context(), s.ID, string(mode)); err != nil {
		rt.writeStackError(w, "delete", err)
		return
	}
	rt.auditStack(r, auditStackDestroy, map[string]any{"stack": s.Name, "mode": string(mode), "destroyed": len(resp.Destroyed), "forgotten": len(resp.Forgotten), "delete_data": deleteData, "databases_deleted": dbsDeleted})
	writeJSON(w, http.StatusOK, resp)
}

// revisionRejected reads the reject marker approval writes into result_json.
func revisionRejected(raw []byte) bool {
	var r struct {
		Error string `json:"error"`
	}
	return len(raw) > 0 && json.Unmarshal(raw, &r) == nil && r.Error == "rejected"
}

// stackNoop reports whether the request changes nothing: the rendered documents
// equal the latest applied revision's and the live plan is empty. It reads only,
// so a no-op takes no lease and writes no revision or ledger row.
func (rt *Router) stackNoop(ctx context.Context, ss StackStore, run stackRun, res []*iac.Resource, opts iac.Options, project string, doer iac.Doer) (store.StackRevisionRecord, bool) {
	req, stack := run.req, run.stack
	if len(req.Secrets) > 0 {
		return store.StackRevisionRecord{}, false
	}
	revs, err := ss.ListStackRevisions(ctx, stack.ID, 1)
	if err != nil || len(revs) == 0 || revs[0].Status != store.StackRevApplied {
		return store.StackRevisionRecord{}, false
	}
	latest, err := ss.GetStackRevision(ctx, stack.ID, revs[0].Number)
	if err != nil {
		return store.StackRevisionRecord{}, false
	}
	rendered, err := json.Marshal(renderStackFiles(req.Files, req.Vars))
	if err != nil || !bytes.Equal(rendered, latest.RenderedDocs) {
		return store.StackRevisionRecord{}, false
	}
	lctx, err := rt.stackLedgerContext(ctx, ss, stack.ID, stack.Name, project, res, req)
	if err != nil {
		return store.StackRevisionRecord{}, false
	}
	st, err := iac.Load(lctx, doer, res, opts)
	if err != nil {
		return store.StackRevisionRecord{}, false
	}
	plan := iac.PlanFor(st, res, opts)
	sum := plan.Summary
	if sum.Create+sum.Update+sum.Delete+sum.Error > 0 || len(iac.LedgerConflicts(plan)) > 0 {
		return store.StackRevisionRecord{}, false
	}
	return latest, true
}
