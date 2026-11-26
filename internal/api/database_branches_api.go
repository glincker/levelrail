package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbauthz"
	"github.com/GLINCKER/levelrail/internal/dbbranch"
	"github.com/GLINCKER/levelrail/internal/diskguard"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Audit actions of the branch lifecycle. Details carry names and modes, never row data.
const (
	AuditActionDatabaseBranchCreate    = "database.branch.create"
	AuditActionDatabaseBranchReset     = "database.branch.reset"
	AuditActionDatabaseBranchPromote   = "database.branch.promote"
	AuditActionDatabaseBranchProtect   = "database.branch.protect"
	AuditActionDatabaseBranchUnprotect = "database.branch.unprotect"
	AuditActionDatabaseBranchExtend    = "database.branch.extend"
	AuditActionDatabaseBranchDelete    = "database.branch.delete"
	AuditActionDatabaseBranchExpire    = "database.branch.expire"
	AuditActionDatabaseBranchFailed    = "database.branch.failed"
)

// BranchCloner runs the slow path clone of one branch row in the background.
type BranchCloner interface {
	Run(ctx context.Context, b store.DatabaseBranch, engine string) error
}

// WithBranchCloner enables creating and resetting branches through the branch API.
func WithBranchCloner(c BranchCloner) Option {
	return func(rt *Router) { rt.branchCloner = c }
}

// branchAPIStore is what the real store offers; fakes without it get a 501.
type branchAPIStore interface {
	dbbranch.Store
	CreateDatabaseBranch(ctx context.Context, b store.DatabaseBranch) error
	CreateDatabaseBranchWithinQuota(ctx context.Context, b store.DatabaseBranch, q store.BranchQuota) error
	GetDatabaseBranch(ctx context.Context, name string) (store.DatabaseBranch, error)
	ListDatabaseBranchesByRoot(ctx context.Context, root string) ([]store.DatabaseBranch, error)
	ListDatabaseBranchesByParent(ctx context.Context, parent string) ([]store.DatabaseBranch, error)
	ListAllDatabaseBranches(ctx context.Context) ([]store.DatabaseBranch, error)
	SetDatabaseBranchProtected(ctx context.Context, id string, from, to bool, now string) error
	SetDatabaseBranchExpiry(ctx context.Context, id, expiresAt, preset, now string) error
}

// branchSizer measures the parent so quotas and the disk guard see its real size.
type branchSizer interface {
	ParentSize(ctx context.Context, engine, database string) (int64, error)
}

func (rt *Router) branchStore(w http.ResponseWriter) (branchAPIStore, bool) {
	s, ok := rt.databases.(branchAPIStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "database branches are not available on this control plane")
	}
	return s, ok
}

// branchResource is one node of the branch tree; ParentName links the tree.
type branchResource struct {
	Name            string `json:"name"`
	ParentName      string `json:"parent"`
	RootName        string `json:"root"`
	Mode            string `json:"mode"`
	Source          string `json:"source"`
	State           string `json:"state"`
	StateReason     string `json:"state_reason,omitempty"`
	ClonePath       string `json:"clone_path,omitempty"`
	ClonePathReason string `json:"clone_path_reason,omitempty"`
	Protected       bool   `json:"protected"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	ExpiryPreset    string `json:"expiry_preset,omitempty"`
	SizeBytes       int64  `json:"size_bytes"`
	LastActiveAt    string `json:"last_active_at,omitempty"`
	BranchedAt      string `json:"branched_at,omitempty"`
	CreatedAt       string `json:"created_at"`
}

func toBranchResource(b store.DatabaseBranch) branchResource {
	return branchResource{
		Name: b.DatabaseName, ParentName: b.ParentName, RootName: b.RootName, Mode: b.Mode, Source: b.Source,
		State: b.State, StateReason: b.StateReason, ClonePath: b.ClonePath, ClonePathReason: b.ClonePathReason,
		Protected: b.Protected, ExpiresAt: b.ExpiresAt, ExpiryPreset: b.ExpiryPreset, SizeBytes: b.SizeBytes,
		LastActiveAt: b.LastActiveAt, BranchedAt: b.BranchedAt, CreatedAt: b.CreatedAt,
	}
}

// createBranchRequest is POST /api/v1/databases/{name}/branches. backup_id or at keep the legacy meaning.
type createBranchRequest struct {
	Name      string `json:"name"`
	Mode      string `json:"mode,omitempty"`
	Parent    string `json:"parent,omitempty"`
	TTL       string `json:"ttl,omitempty"`
	Protected bool   `json:"protected,omitempty"`
	BackupID  string `json:"backup_id,omitempty"`
	At        string `json:"at,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

// parseBranchTTL reads "never", "1h", "7d" or any Go duration. A zero duration means never expires.
func parseBranchTTL(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "never" || s == "0" {
		return 0, nil
	}
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil || days <= 0 {
			return 0, errors.New("ttl must be never, a positive number of days like 7d, or a duration like 12h")
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, errors.New("ttl must be never, a positive number of days like 7d, or a duration like 12h")
	}
	return d, nil
}

func newBranchID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate branch id: %w", err)
	}
	return "dbb_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// branchLimits reads the live branches.* settings; a nil service yields no limits (unit tests only).
func (rt *Router) branchLimits(ctx context.Context) dbbranch.Limits {
	s := rt.runtimeSettings
	headroom, _ := strconv.ParseFloat(s.String(ctx, runtimesettings.KeyBranchesSlowPathHeadroom), 64)
	return dbbranch.Limits{
		MaxPerRoot:       s.Int(ctx, runtimesettings.KeyBranchesMaxPerRoot),
		MaxTotal:         s.Int(ctx, runtimesettings.KeyBranchesMaxTotal),
		MaxTotalBytes:    int64(s.Int(ctx, runtimesettings.KeyBranchesMaxTotalBytes)),
		MaxDepth:         s.Int(ctx, runtimesettings.KeyBranchesMaxDepth),
		SlowPathMaxBytes: int64(s.Int(ctx, runtimesettings.KeyBranchesSlowPathMaxBytes)),
		Headroom:         headroom,
	}
}

// dbActionVerdict resolves one db action on one database for the caller.
func (rt *Router) dbActionVerdict(r *http.Request, action dbauthz.Action, database string) (allowed, hidden bool) {
	pt, pid, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return false, true
	}
	resolver := &dbauthz.Resolver{Engine: &dbPolicyEngine{rt: rt}}
	for _, a := range impliedActions(action) {
		d, err := resolver.Resolve(r.Context(), dbauthz.Caller{PrincipalType: pt, PrincipalID: pid, Abilities: abilities}, dbauthz.ResourcePrefix+database, a)
		if err != nil {
			// Fail closed: an engine error must never make a database look visible.
			return false, true
		}
		if d.Hidden {
			return false, true
		}
		if d.Allowed {
			return true, false
		}
	}
	return false, false
}

// requireDBAction answers 404 for a hidden database and 403 for a denied action, and reports whether the caller may go on.
func (rt *Router) requireDBAction(w http.ResponseWriter, r *http.Request, action dbauthz.Action, database string) bool {
	allowed, hidden := rt.dbActionVerdict(r, action, database)
	switch {
	case hidden:
		writeError(w, http.StatusNotFound, "database not found")
	case !allowed:
		writeError(w, http.StatusForbidden, "this action needs "+string(action)+" on database "+database)
	default:
		return true
	}
	return false
}

// branchTreeRoot is the root name of the tree a database belongs to.
func branchTreeRoot(ctx context.Context, bs branchAPIStore, name string) (root string, isBranch bool, err error) {
	b, err := bs.GetDatabaseBranch(ctx, name)
	if errors.Is(err, store.ErrDatabaseBranchNotFound) {
		return name, false, nil
	}
	if err != nil {
		return "", false, err
	}
	return b.RootName, true, nil
}

// loadPathBranch loads {branch} and checks it belongs to the tree of {name}, so a route scoped to one database
// never reaches a branch of another. It also checks the action on the branch itself.
func (rt *Router) loadPathBranch(w http.ResponseWriter, r *http.Request, action dbauthz.Action) (store.DatabaseBranch, branchAPIStore, bool) {
	bs, ok := rt.branchStore(w)
	if !ok {
		return store.DatabaseBranch{}, nil, false
	}
	root, _, err := branchTreeRoot(r.Context(), bs, r.PathValue("name"))
	if err != nil {
		rt.internalError(w, "api: database branch: resolve tree failed", err)
		return store.DatabaseBranch{}, nil, false
	}
	b, err := bs.GetDatabaseBranch(r.Context(), r.PathValue("branch"))
	if errors.Is(err, store.ErrDatabaseBranchNotFound) || (err == nil && b.RootName != root) {
		writeError(w, http.StatusNotFound, "branch not found")
		return store.DatabaseBranch{}, nil, false
	}
	if err != nil {
		rt.internalError(w, "api: database branch: load failed", err)
		return store.DatabaseBranch{}, nil, false
	}
	if !rt.requireDBAction(w, r, action, b.DatabaseName) {
		return store.DatabaseBranch{}, nil, false
	}
	return b, bs, true
}

// handleCreateDatabaseBranch creates a branch. A body with backup_id or at is the legacy restore based create.
func (rt *Router) handleCreateDatabaseBranch(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxDecodeBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var req createBranchRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !validDatabaseName(req.Name) {
		writeError(w, http.StatusBadRequest, "name must be 1-63 characters: lowercase letters, digits and hyphens, starting with a letter")
		return
	}
	if req.BackupID != "" || req.At != "" {
		if !rt.requireDBAction(w, r, dbauthz.ActionAdmin, r.PathValue("name")) {
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		rt.handleBranchDatabase(w, r)
		return
	}
	rt.createLiveBranch(w, r, req)
}

func (rt *Router) createLiveBranch(w http.ResponseWriter, r *http.Request, req createBranchRequest) {
	ctx := r.Context()
	bs, ok := rt.branchStore(w)
	if !ok {
		return
	}
	if rt.branchCloner == nil {
		writeError(w, http.StatusNotImplemented, "branch clones are not configured on this control plane")
		return
	}
	if rt.runtimeSettings != nil && !rt.runtimeSettings.Bool(ctx, runtimesettings.KeyBranchesEnabled) {
		writeError(w, http.StatusConflict, "database branches are turned off")
		return
	}
	mode := req.Mode
	switch mode {
	case "", "data", dbbranch.ModeFull:
		mode = dbbranch.ModeFull
	case dbbranch.ModeSchema:
	default:
		writeError(w, http.StatusBadRequest, "mode must be data or schema")
		return
	}
	pathName := r.PathValue("name")
	parentName := req.Parent
	if parentName == "" {
		parentName = pathName
	}
	root, _, err := branchTreeRoot(ctx, bs, pathName)
	if err != nil {
		rt.internalError(w, "api: create branch: resolve tree failed", err)
		return
	}
	parentRoot, parentIsBranch, err := branchTreeRoot(ctx, bs, parentName)
	if err != nil {
		rt.internalError(w, "api: create branch: resolve parent failed", err)
		return
	}
	if parentRoot != root {
		writeError(w, http.StatusBadRequest, "parent must be in the same branch tree as "+pathName)
		return
	}
	// A data copy exposes every row of the parent, a schema copy exposes none.
	need := dbauthz.ActionAdmin
	if mode == dbbranch.ModeSchema {
		need = dbauthz.ActionWrite
	}
	if !rt.requireDBAction(w, r, need, parentName) {
		return
	}
	parentDB, ok := rt.loadDatabaseForRunner(w, r, parentName, "api: create branch: load parent failed")
	if !ok {
		return
	}
	if mode == dbbranch.ModeSchema && !schemaBranchEngine(parentDB.Engine) {
		writeError(w, http.StatusConflict, "schema only branches are not available for "+parentDB.Engine+" yet")
		return
	}
	if okMode, reason := dbbranch.ModeAvailable(parentDB.Engine, mode, dbbranch.Hints{PITREnabled: parentDB.PITREnabled}); !okMode {
		writeError(w, http.StatusConflict, "this mode is not available: "+reason)
		return
	}
	var parentRow store.DatabaseBranch
	if parentIsBranch {
		if parentRow, err = bs.GetDatabaseBranch(ctx, parentName); err != nil {
			rt.internalError(w, "api: create branch: load parent branch failed", err)
			return
		}
		if parentRow.State != dbbranch.StateReady {
			writeError(w, http.StatusConflict, "parent branch is "+parentRow.State+", not ready")
			return
		}
	}
	ttl := rt.runtimeSettings.Duration(ctx, runtimesettings.KeyBranchesDefaultTTL)
	preset := "default"
	if req.TTL != "" {
		if ttl, err = parseBranchTTL(req.TTL); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if rt.branchTTLTooShort(ctx, ttl) {
			writeError(w, http.StatusBadRequest, "ttl is shorter than the minimum branch lifetime")
			return
		}
		preset = req.TTL
	}
	if _, err := rt.databases.GetDesiredDatabase(ctx, req.Name); err == nil {
		writeError(w, http.StatusConflict, "a database with this name already exists")
		return
	} else if !errors.Is(err, store.ErrDatabaseNotFound) {
		rt.internalError(w, "api: create branch: check name failed", err, slog.String("name", req.Name))
		return
	}
	parentBytes := parentRow.SizeBytes
	if sz, ok := rt.branchCloner.(branchSizer); ok {
		n, serr := sz.ParentSize(ctx, parentDB.Engine, parentName)
		if serr != nil {
			rt.logger.Warn("api: create branch: measure parent failed", slog.String("parent", parentName), slog.String("error", serr.Error()))
			writeError(w, http.StatusConflict, "could not measure the parent database size, so the copy was refused")
			return
		}
		parentBytes = n
	}
	limits := rt.branchLimits(ctx)
	all, err := bs.ListAllDatabaseBranches(ctx)
	if err != nil {
		rt.internalError(w, "api: create branch: list branches failed", err)
		return
	}
	if rt.writeBranchRefusal(w, dbbranch.CheckQuota(limits, dbbranch.Request{Parent: parentRow, ParentIsRoot: !parentIsBranch, RootName: root, ParentBytes: parentBytes, SlowPath: true}, all, rt.diskGuard)) {
		return
	}

	id, err := newBranchID()
	if err != nil {
		rt.internalError(w, "api: create branch: id failed", err)
		return
	}
	projectID := req.ProjectID
	if projectID == "" {
		projectID = parentDB.ProjectID
	}
	if !rt.createDesiredDatabase(w, r, databaseResource{Name: req.Name, Engine: parentDB.Engine, Version: parentDB.Version, ProjectID: projectID, Resources: copyResources(parentDB.Resources)}) {
		return
	}
	if !rt.finishBranchSetup(w, r, parentDB, req.Name) {
		return
	}
	now := time.Now().UTC()
	row := store.DatabaseBranch{
		ID: id, DatabaseName: req.Name, ParentName: parentName, RootName: root, Mode: mode, Source: "live",
		ClonePath: "dump", State: dbbranch.StateRequested, Protected: req.Protected,
		ExpiresAt: dbbranch.ExpiryFor(now, ttl), ExpiryPreset: preset, BranchedAt: now.Format(time.RFC3339),
		CreatedBy: rt.accessActor(r).id, CreatedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339),
	}
	if err := bs.CreateDatabaseBranchWithinQuota(ctx, row, store.BranchQuota{MaxPerRoot: limits.MaxPerRoot, MaxTotal: limits.MaxTotal, MaxTotalBytes: limits.MaxTotalBytes, ParentBytes: parentBytes}); err != nil {
		rt.discardBranchDatabase(ctx, req.Name, rt.databaseNodeOf(ctx, req.Name))
		if errors.Is(err, store.ErrDatabaseBranchQuota) {
			writeError(w, http.StatusConflict, "branch quota reached")
			return
		}
		rt.internalError(w, "api: create branch: save row failed", err, slog.String("name", req.Name))
		return
	}
	rt.nudgeReconciler()
	rt.auditBranch(ctx, r, AuditActionDatabaseBranchCreate, map[string]any{"parent": parentName, "branch": req.Name, "mode": mode, "clone_path": "dump"})
	rt.startBranchClone(row, parentDB.Engine)
	writeJSON(w, http.StatusAccepted, toBranchResource(row))
}

// databaseNodeOf is the node of a database just created, which is what a rollback must clean up.
func (rt *Router) databaseNodeOf(ctx context.Context, name string) string {
	d, err := rt.databases.GetDesiredDatabase(ctx, name)
	if err != nil {
		return ""
	}
	return d.NodeID
}

// branchTTLTooShort is true for a finite ttl under branches.min_ttl.
func (rt *Router) branchTTLTooShort(ctx context.Context, ttl time.Duration) bool {
	return ttl > 0 && ttl < rt.runtimeSettings.Duration(ctx, runtimesettings.KeyBranchesMinTTL)
}

// requireTypedConfirm needs the branch name typed back, in the body or the confirm query parameter.
func requireTypedConfirm(w http.ResponseWriter, r *http.Request, name string) bool {
	confirm := r.URL.Query().Get("confirm")
	if r.ContentLength > 0 {
		var req deleteBranchRequest
		if !decodeReq(w, r, &req) {
			return false
		}
		confirm = req.Confirm
	}
	if confirm != name {
		writeError(w, http.StatusBadRequest, "confirm must be the branch name "+name)
		return false
	}
	return true
}

// withBranchReadyGate keeps a branch that is not ready, or failed, unreachable: every database route except
// the branch routes and the plain status read answers 409 branch_not_ready until the copy is complete.
func (rt *Router) withBranchReadyGate(next http.HandlerFunc) http.HandlerFunc {
	const prefix = "/api/v1/databases/"
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		bs, ok := rt.databases.(branchAPIStore)
		if name == "" || !ok || !strings.HasPrefix(r.URL.Path, prefix+name) {
			next(w, r)
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, prefix+name)
		if rest == "/branch-capabilities" || rest == "/branches" || strings.HasPrefix(rest, "/branches/") {
			next(w, r)
			return
		}
		b, err := bs.GetDatabaseBranch(r.Context(), name)
		if errors.Is(err, store.ErrDatabaseBranchNotFound) {
			next(w, r)
			return
		}
		if err != nil {
			rt.internalError(w, "api: branch ready gate failed", err, slog.String("name", name))
			return
		}
		switch {
		case dbbranch.Reachable(b.State):
			next(w, r)
		case rest == "" && (r.Method == http.MethodGet || (r.Method == http.MethodDelete && b.State == dbbranch.StateFailed)):
			next(w, r)
		default:
			writeCodedError(w, http.StatusConflict, "branch_not_ready", "branch "+name+" is "+b.State+" and not available yet")
		}
	}
}

// schemaBranchEngine lists the engines whose dump has a schema only flag.
func schemaBranchEngine(engine string) bool {
	return engine == store.EnginePostgres || engine == store.EngineMySQL || engine == store.EngineMariaDB
}

// writeBranchRefusal maps a quota or disk refusal to 409 with its reason and reports whether it wrote one.
func (rt *Router) writeBranchRefusal(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var low *diskguard.LowDiskError
	if errors.As(err, &low) {
		writeCodedError(w, http.StatusConflict, errCodeLowDisk, low.Error())
		return true
	}
	if errors.Is(err, dbbranch.ErrQuota) {
		writeError(w, http.StatusConflict, err.Error())
		return true
	}
	rt.internalError(w, "api: branch admission failed", err)
	return true
}

// startBranchClone runs the clone detached from the request, which ends before the copy does.
func (rt *Router) startBranchClone(row store.DatabaseBranch, engine string) {
	go func() { //nolint:gosec // the request context is cancelled when the handler returns, which would abort the copy
		if err := rt.branchCloner.Run(context.Background(), row, engine); err != nil {
			rt.logger.Error("api: database branch clone failed", slog.String("branch", row.DatabaseName), slog.String("error", err.Error()))
			rt.auditBranch(context.Background(), nil, AuditActionDatabaseBranchFailed, map[string]any{"parent": row.ParentName, "branch": row.DatabaseName, "mode": row.Mode})
		}
	}()
}

// discardBranchDatabase removes a branch database whose row could not be kept.
func (rt *Router) discardBranchDatabase(ctx context.Context, name, nodeID string) {
	ctx = context.WithoutCancel(ctx)
	if err := rt.databases.DeleteDesiredDatabase(ctx, name); err != nil {
		rt.logger.Warn("api: discard branch database failed", slog.String("name", name), slog.String("error", err.Error()))
	}
	_ = rt.runDatabaseTeardown(ctx, name, nodeID, true)
}

// TeardownBranchDatabase deletes a branch database and its data volume. The store refuses a protected
// branch or one with live children, and the branch row goes with the database. It backs the expiry job.
func (rt *Router) TeardownBranchDatabase(ctx context.Context, name string) error {
	defer lockAppName("database:" + name)()
	d, err := rt.databases.GetDesiredDatabase(ctx, name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("api: teardown branch %q: %w", name, err)
	}
	if rt.execRuntime != nil {
		if err := rt.recordDatabaseTeardown(ctx, name, d.NodeID, true); err != nil {
			return err
		}
	}
	if err := rt.databases.DeleteDesiredDatabase(ctx, name); err != nil && !errors.Is(err, store.ErrDatabaseNotFound) {
		return err
	}
	rt.changes.Publish(changeDatabases, name)
	rt.nudgeReconciler()
	return rt.runDatabaseTeardown(ctx, name, d.NodeID, true)
}

// AuditBranchExpired lets the expiry job write its audit row.
func (rt *Router) AuditBranchExpired(ctx context.Context, b store.DatabaseBranch) {
	rt.auditBranch(ctx, nil, AuditActionDatabaseBranchExpire, map[string]any{"parent": b.ParentName, "branch": b.DatabaseName, "mode": b.Mode})
}

func (rt *Router) handleListDatabaseBranches(w http.ResponseWriter, r *http.Request) {
	bs, ok := rt.branchStore(w)
	if !ok {
		return
	}
	root, _, err := branchTreeRoot(r.Context(), bs, r.PathValue("name"))
	if err != nil {
		rt.internalError(w, "api: list branches: resolve tree failed", err)
		return
	}
	rows, err := bs.ListDatabaseBranchesByRoot(r.Context(), root)
	if err != nil {
		rt.internalError(w, "api: list branches failed", err)
		return
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CreatedAt < rows[j].CreatedAt })
	out := make([]branchResource, 0, len(rows))
	visible := map[string]bool{}
	canSee := func(n string) bool {
		v, ok := visible[n]
		if !ok {
			_, hidden := rt.dbActionVerdict(r, dbauthz.ActionRead, n)
			v = !hidden
			visible[n] = v
		}
		return v
	}
	for _, b := range rows {
		if !canSee(b.DatabaseName) {
			continue
		}
		res := toBranchResource(b)
		if res.ParentName != "" && !canSee(res.ParentName) {
			res.ParentName = "hidden"
		}
		if !canSee(res.RootName) {
			res.RootName = "hidden"
		}
		out = append(out, res)
	}
	if !canSee(root) {
		root = "hidden"
	}
	writeJSON(w, http.StatusOK, map[string]any{"root": root, "branches": out})
}

func (rt *Router) handleGetDatabaseBranch(w http.ResponseWriter, r *http.Request) {
	if b, _, ok := rt.loadPathBranch(w, r, dbauthz.ActionRead); ok {
		writeJSON(w, http.StatusOK, toBranchResource(b))
	}
}

func (rt *Router) handleBranchCapabilities(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.loadDatabaseForRunner(w, r, r.PathValue("name"), "api: branch capabilities: load failed")
	if !ok {
		return
	}
	c := dbbranch.CapabilityFor(d.Engine, dbbranch.Hints{PITREnabled: d.PITREnabled, MySQLPITRRunner: rt.mysqlPITRRestoreRunner != nil})
	if m, found := c.Modes[dbbranch.ModeSchema]; found && m.Available && !schemaBranchEngine(d.Engine) {
		c.Modes[dbbranch.ModeSchema] = dbbranch.ModeCap{Reason: "schema only branches are not available for " + d.Engine + " yet"}
	}
	writeJSON(w, http.StatusOK, c)
}

type patchBranchRequest struct {
	TTL       *string `json:"ttl,omitempty"`
	Protected *bool   `json:"protected,omitempty"`
}

func (rt *Router) handlePatchDatabaseBranch(w http.ResponseWriter, r *http.Request) {
	var req patchBranchRequest
	if !decodeReq(w, r, &req) {
		return
	}
	b, bs, ok := rt.loadPathBranch(w, r, dbauthz.ActionWrite)
	if !ok {
		return
	}
	ctx, now := r.Context(), time.Now().UTC()
	if req.TTL != nil {
		ttl, err := parseBranchTTL(*req.TTL)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if b.State != dbbranch.StateReady {
			writeError(w, http.StatusConflict, "only a ready branch can change its expiry")
			return
		}
		if rt.branchTTLTooShort(ctx, ttl) {
			writeError(w, http.StatusBadRequest, "ttl is shorter than the minimum branch lifetime")
			return
		}
		if shortensExpiry(b.ExpiresAt, ttl, now) && !rt.requireDBAction(w, r, dbauthz.ActionAdmin, b.DatabaseName) {
			return
		}
		if err := bs.SetDatabaseBranchExpiry(ctx, b.ID, dbbranch.ExpiryFor(now, ttl), *req.TTL, now.Format(time.RFC3339)); err != nil {
			rt.internalError(w, "api: patch branch: set expiry failed", err)
			return
		}
		rt.auditBranch(ctx, r, AuditActionDatabaseBranchExtend, map[string]any{"branch": b.DatabaseName, "ttl": *req.TTL})
	}
	if req.Protected != nil && *req.Protected != b.Protected {
		if !*req.Protected && !rt.requireDBAction(w, r, dbauthz.ActionAdmin, b.DatabaseName) {
			return
		}
		err := bs.SetDatabaseBranchProtected(ctx, b.ID, b.Protected, *req.Protected, now.Format(time.RFC3339))
		if errors.Is(err, store.ErrDatabaseBranchStateRace) {
			writeError(w, http.StatusConflict, "branch protection changed, retry")
			return
		}
		if err != nil {
			rt.internalError(w, "api: patch branch: set protection failed", err)
			return
		}
		action := AuditActionDatabaseBranchUnprotect
		if *req.Protected {
			action = AuditActionDatabaseBranchProtect
		}
		rt.auditBranch(ctx, r, action, map[string]any{"branch": b.DatabaseName})
	}
	cur, err := bs.GetDatabaseBranch(ctx, b.DatabaseName)
	if err != nil {
		rt.internalError(w, "api: patch branch: reload failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toBranchResource(cur))
}

// shortensExpiry is true when the new lifetime ends earlier than the current one; never is the longest.
func shortensExpiry(current string, ttl time.Duration, now time.Time) bool {
	if ttl == 0 {
		return false
	}
	if current == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, current)
	return err != nil || now.Add(ttl).Before(t)
}

type deleteBranchRequest struct {
	Confirm string `json:"confirm"`
}

// handleDeleteDatabaseBranch needs the branch name typed back. The store guards refuse a protected branch or a parent.
func (rt *Router) handleDeleteDatabaseBranch(w http.ResponseWriter, r *http.Request) {
	b, bs, ok := rt.loadPathBranch(w, r, dbauthz.ActionAdmin)
	if !ok {
		return
	}
	confirm := r.URL.Query().Get("confirm")
	if r.ContentLength > 0 {
		var req deleteBranchRequest
		if !decodeReq(w, r, &req) {
			return
		}
		confirm = req.Confirm
	}
	if confirm != b.DatabaseName {
		writeError(w, http.StatusBadRequest, "confirm must be the branch name "+b.DatabaseName)
		return
	}
	// A branch that is still being copied is not deleted: the clone worker owns its database until it finishes or fails.
	if b.State != dbbranch.StateReady && b.State != dbbranch.StateFailed && b.State != dbbranch.StatePromoted {
		writeError(w, http.StatusConflict, "branch is "+b.State+"; wait for it to finish or fail before deleting it")
		return
	}
	if err := rt.checkDatabaseDeletable(r.Context(), b.DatabaseName); writeBranchBlock(w, err) {
		return
	}
	if b.State == dbbranch.StateReady || b.State == dbbranch.StateFailed {
		err := dbbranch.Move(r.Context(), bs, b.ID, b.State, dbbranch.StateDeleting, "deleted by request", time.Now().UTC().Format(time.RFC3339))
		if errors.Is(err, store.ErrDatabaseBranchProtected) {
			writeBranchBlock(w, err)
			return
		}
		if err != nil {
			writeError(w, http.StatusConflict, "branch state changed, retry")
			return
		}
	}
	err := rt.TeardownBranchDatabase(r.Context(), b.DatabaseName)
	if writeBranchBlock(w, err) {
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete branch failed", err, slog.String("name", b.DatabaseName))
		return
	}
	rt.auditBranch(r.Context(), r, AuditActionDatabaseBranchDelete, map[string]any{"parent": b.ParentName, "branch": b.DatabaseName, "mode": b.Mode})
	w.WriteHeader(http.StatusNoContent)
}

// handlePromoteDatabaseBranch detaches a ready branch from its parent so it survives the parent and expiry.
func (rt *Router) handlePromoteDatabaseBranch(w http.ResponseWriter, r *http.Request) {
	if !requireTypedConfirm(w, r, r.PathValue("branch")) {
		return
	}
	b, bs, ok := rt.loadPathBranch(w, r, dbauthz.ActionAdmin)
	if !ok {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, to := range []string{dbbranch.StatePromoting, dbbranch.StatePromoted} {
		from := dbbranch.StateReady
		if to == dbbranch.StatePromoted {
			from = dbbranch.StatePromoting
		}
		if err := dbbranch.Move(r.Context(), bs, b.ID, from, to, "", now); err != nil {
			writeError(w, http.StatusConflict, "only a ready branch can be promoted")
			return
		}
	}
	rt.auditBranch(r.Context(), r, AuditActionDatabaseBranchPromote, map[string]any{"parent": b.ParentName, "branch": b.DatabaseName})
	cur, err := bs.GetDatabaseBranch(r.Context(), b.DatabaseName)
	if err != nil {
		rt.internalError(w, "api: promote branch: reload failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toBranchResource(cur))
}

// handleResetDatabaseBranch clones the parent's current data again. The database is recreated, so it gets new credentials.
func (rt *Router) handleResetDatabaseBranch(w http.ResponseWriter, r *http.Request) {
	if !requireTypedConfirm(w, r, r.PathValue("branch")) {
		return
	}
	b, bs, ok := rt.loadPathBranch(w, r, dbauthz.ActionAdmin)
	if !ok {
		return
	}
	// A reset copies the parent again, so it needs the same right on the parent as the first copy did.
	need := dbauthz.ActionAdmin
	if b.Mode == dbbranch.ModeSchema {
		need = dbauthz.ActionWrite
	}
	if !rt.requireDBAction(w, r, need, b.ParentName) {
		return
	}
	if rt.branchCloner == nil {
		writeError(w, http.StatusNotImplemented, "branch clones are not configured on this control plane")
		return
	}
	if b.Protected {
		writeBranchBlock(w, store.ErrDatabaseBranchProtected)
		return
	}
	if kids, err := bs.ListDatabaseBranchesByParent(r.Context(), b.DatabaseName); err != nil || hasLiveBranch(kids) {
		writeError(w, http.StatusConflict, "branch has children; delete or promote them first")
		return
	}
	if _, err := bs.GetDatabaseBranch(r.Context(), b.ParentName); err != nil {
		if _, derr := rt.databases.GetDesiredDatabase(r.Context(), b.ParentName); derr != nil {
			writeError(w, http.StatusConflict, "parent database no longer exists")
			return
		}
	}
	old, err := rt.databases.GetDesiredDatabase(r.Context(), b.DatabaseName)
	if err != nil {
		writeError(w, http.StatusNotFound, "branch database not found")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := dbbranch.Move(r.Context(), bs, b.ID, b.State, dbbranch.StateResetting, "", now); err != nil {
		if errors.Is(err, store.ErrDatabaseBranchProtected) {
			writeBranchBlock(w, err)
			return
		}
		writeError(w, http.StatusConflict, "branch state changed, retry")
		return
	}
	row := b
	row.State, row.UpdatedAt = dbbranch.StateResetting, now
	if err := rt.TeardownBranchDatabase(r.Context(), b.DatabaseName); err != nil {
		rt.internalError(w, "api: reset branch: teardown failed", err, slog.String("name", b.DatabaseName))
		return
	}
	if !rt.createDesiredDatabase(w, r, databaseResource{Name: b.DatabaseName, Engine: old.Engine, Version: old.Version, ProjectID: old.ProjectID, Resources: copyResources(old.Resources)}) {
		// The old database and its row are gone; leave a trace instead of a silent loss.
		rt.logger.Error("api: reset branch: recreate failed after teardown", slog.String("name", b.DatabaseName))
		rt.auditBranch(r.Context(), r, AuditActionDatabaseBranchFailed, map[string]any{"parent": b.ParentName, "branch": b.DatabaseName, "step": "reset_recreate"})
		return
	}
	// The delete cascaded the row away, so it is written back with its lineage and the same id.
	if err := bs.CreateDatabaseBranch(r.Context(), row); err != nil {
		rt.discardBranchDatabase(r.Context(), b.DatabaseName, rt.databaseNodeOf(r.Context(), b.DatabaseName))
		rt.auditBranch(r.Context(), r, AuditActionDatabaseBranchFailed, map[string]any{"parent": b.ParentName, "branch": b.DatabaseName, "step": "reset_row"})
		rt.internalError(w, "api: reset branch: restore row failed", err, slog.String("name", b.DatabaseName))
		return
	}
	rt.nudgeReconciler()
	rt.auditBranch(r.Context(), r, AuditActionDatabaseBranchReset, map[string]any{"parent": b.ParentName, "branch": b.DatabaseName, "mode": b.Mode})
	rt.startBranchClone(row, old.Engine)
	writeJSON(w, http.StatusAccepted, toBranchResource(row))
}

func hasLiveBranch(rows []store.DatabaseBranch) bool {
	for _, k := range rows {
		if k.State != dbbranch.StateDeleted && k.State != dbbranch.StatePromoted {
			return true
		}
	}
	return false
}

// auditBranch writes one lifecycle audit row. r is nil for work done by a background job.
func (rt *Router) auditBranch(ctx context.Context, r *http.Request, action string, detail map[string]any) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: branch audit id failed", slog.String("error", err.Error()))
		return
	}
	body, _ := json.Marshal(detail)
	e := store.AuditEntry{ID: id, Ability: AbilityRoot, Action: action, Detail: string(body), CreatedAt: store.FormatAuditTime(time.Now()), ActorType: "system", ActorID: "branch-job", ActorName: "branch job"}
	if r != nil {
		a := rt.accessActor(r)
		e.ActorType, e.ActorID, e.ActorName = a.kind, a.id, a.name
		e.Method, e.Path, e.RemoteAddr, e.StatusCode = r.Method, r.URL.Path, clientIP(r), http.StatusOK
		e.ClientKind = clientKindFromUserAgent(r.Header.Get("User-Agent"))
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(ctx), e); err != nil {
		rt.logger.Warn("api: branch audit failed", slog.String("error", err.Error()))
	}
}
