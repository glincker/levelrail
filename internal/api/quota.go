package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/quota"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envQuotaDefaultApps      = "APP_QUOTA_DEFAULT_MAX_APPS"
	envQuotaDefaultDatabases = "APP_QUOTA_DEFAULT_MAX_DATABASES"
	envQuotaDefaultMemoryMB  = "APP_QUOTA_DEFAULT_MAX_MEMORY_MB"
	envQuotaDefaultCPU       = "APP_QUOTA_DEFAULT_MAX_CPU"
	envQuotaDefaultPreviews  = "APP_QUOTA_DEFAULT_MAX_PREVIEWS"
	envQuotaWarnPercent      = "APP_QUOTA_WARN_PERCENT"

	defaultQuotaWarnPercent = 80
	quotaExceededCode       = "quota_exceeded"
	bytesPerMB              = 1 << 20
	nanoCPUsPerCore         = 1_000_000_000
)

// quotaStore is what quota enforcement and its handlers need; *store.DB
// satisfies it.
type quotaStore interface {
	GetQuota(ctx context.Context, scope, scopeID string) (quota.Amounts, error)
	SetQuota(ctx context.Context, scope, scopeID string, a quota.Amounts) error
	DeleteQuota(ctx context.Context, scope, scopeID string) error
	QuotaUsage(ctx context.Context, scope, scopeID string) (quota.Amounts, error)
	QuotaScopeIDs(ctx context.Context, scope string) ([]string, error)
	TeamOfProject(ctx context.Context, projectID string) (string, error)
	OwnerOfApp(ctx context.Context, name string) (*store.ResourceOwner, error)
	OwnerOfDatabase(ctx context.Context, name string) (*store.ResourceOwner, error)
}

func (rt *Router) quotaDB() quotaStore {
	for _, candidate := range []any{rt.apps, rt.projects, rt.policies} {
		if s, ok := candidate.(quotaStore); ok {
			return s
		}
	}
	return nil
}

// quotaDefaults reads the instance wide limits applied wherever a team or
// project has none of its own. All unset means unlimited.
func quotaDefaults() quota.Amounts {
	num := func(key string) int64 {
		n, err := strconv.ParseInt(os.Getenv(key), 10, 64)
		if err != nil || n < 0 {
			return 0
		}
		return n
	}
	cpu, err := strconv.ParseFloat(os.Getenv(envQuotaDefaultCPU), 64)
	if err != nil || cpu < 0 {
		cpu = 0
	}
	return quota.Amounts{
		Apps:        num(envQuotaDefaultApps),
		Databases:   num(envQuotaDefaultDatabases),
		MemoryBytes: num(envQuotaDefaultMemoryMB) * bytesPerMB,
		NanoCPUs:    int64(cpu * nanoCPUsPerCore),
		Previews:    num(envQuotaDefaultPreviews),
	}
}

func quotaWarnPercent() int {
	if n, err := strconv.Atoi(os.Getenv(envQuotaWarnPercent)); err == nil && n > 0 && n <= 100 {
		return n
	}
	return defaultQuotaWarnPercent
}

// effectiveQuota returns a scope's limits with env defaults filled in.
func effectiveQuota(ctx context.Context, db quotaStore, scope, scopeID string) (quota.Amounts, error) {
	explicit, err := db.GetQuota(ctx, scope, scopeID)
	if err != nil {
		return quota.Amounts{}, err
	}
	return quota.Merge(explicit, quotaDefaults()), nil
}

// serviceReservation is what one service holds: memory and CPU per replica.
func serviceReservation(res *store.ServiceResources, replicas int) quota.Amounts {
	if res == nil {
		return quota.Amounts{}
	}
	n := int64(max(replicas, 1))
	// /dev/shm is charged to the memory cgroup, so mem_limit bounds it when set.
	mem := res.MemoryBytes
	if mem == 0 {
		mem = res.ShmSizeBytes
	}
	return quota.Amounts{MemoryBytes: mem * n, NanoCPUs: res.NanoCPUs * n}
}

// quotaDenial is a refused change, naming the scope, limit and usage.
type quotaDenial struct {
	scope     string
	scopeName string
	v         quota.Violation
}

func (d quotaDenial) message() string {
	return fmt.Sprintf("%s %q has reached its %s quota: %s of %s used, %s more requested",
		d.scope, d.scopeName, d.v.Resource,
		formatQuotaAmount(d.v.Resource, d.v.Used), formatQuotaAmount(d.v.Resource, d.v.Limit),
		formatQuotaAmount(d.v.Resource, d.v.Requested))
}

func formatQuotaAmount(resource string, n int64) string {
	switch resource {
	case quota.ResourceMemory:
		return fmt.Sprintf("%d MB", n/bytesPerMB)
	case quota.ResourceCPU:
		return strconv.FormatFloat(float64(n)/nanoCPUsPerCore, 'f', -1, 64) + " CPU"
	}
	return strconv.FormatInt(n, 10)
}

func writeQuotaDenied(w http.ResponseWriter, d quotaDenial) {
	writeCodedError(w, http.StatusConflict, quotaExceededCode, d.message())
}

// scopeName is a project or team's display name, falling back to its id.
func (rt *Router) quotaScopeName(ctx context.Context, scope, id string) string {
	if scope == quota.ScopeProject && rt.projects != nil {
		if p, err := rt.projects.GetProject(ctx, id); err == nil && p.Name != "" {
			return p.Name
		}
	}
	if scope == quota.ScopeTeam {
		if o, ok := rt.projects.(interface {
			GetOrganization(context.Context, string) (store.Organization, error)
		}); ok {
			if org, err := o.GetOrganization(ctx, id); err == nil && org.Name != "" {
				return org.Name
			}
		}
	}
	return id
}

// quotaDenialFor checks delta against the destination project and team,
// skipping any scope the resource already sits in (from). Callers that
// race must hold quotaMu, or be serialized some other way.
func (rt *Router) quotaDenialFor(ctx context.Context, db quotaStore, projectID string, from store.ResourceOwner, delta quota.Amounts) (*quotaDenial, error) {
	teamID := store.DefaultTeamID
	if projectID != "" {
		t, err := db.TeamOfProject(ctx, projectID)
		if err != nil {
			return nil, err
		}
		teamID = t
	}
	type target struct{ scope, id string }
	var targets []target
	if projectID != "" && projectID != from.ProjectID {
		targets = append(targets, target{quota.ScopeProject, projectID})
	}
	if teamID != from.TeamID {
		targets = append(targets, target{quota.ScopeTeam, teamID})
	}
	for _, t := range targets {
		limits, err := effectiveQuota(ctx, db, t.scope, t.id)
		if err != nil {
			return nil, err
		}
		if limits.Unlimited() {
			continue
		}
		used, err := db.QuotaUsage(ctx, t.scope, t.id)
		if err != nil {
			return nil, err
		}
		if vs := quota.Check(limits, used, delta); len(vs) > 0 {
			return &quotaDenial{scope: t.scope, scopeName: rt.quotaScopeName(ctx, t.scope, t.id), v: vs[0]}, nil
		}
	}
	return nil, nil
}

// admitQuota is the atomic check for one create or change. On success the
// returned release must be called after the change is saved, so concurrent
// creates cannot both pass the same remaining headroom.
func (rt *Router) admitQuota(ctx context.Context, projectID string, from store.ResourceOwner, delta quota.Amounts) (release func(), denial *quotaDenial, err error) {
	noop := func() {}
	db := rt.quotaDB()
	if db == nil {
		return noop, nil, nil
	}
	rt.quotaMu.Lock()
	denial, err = rt.quotaDenialFor(ctx, db, projectID, from, delta)
	if err != nil || denial != nil {
		rt.quotaMu.Unlock()
		return noop, denial, err
	}
	return rt.quotaMu.Unlock, nil, nil
}

// admitQuotaOrWrite runs admitQuota and writes the 409 or 500 itself.
func (rt *Router) admitQuotaOrWrite(w http.ResponseWriter, r *http.Request, projectID string, from store.ResourceOwner, delta quota.Amounts) (func(), bool) {
	release, denial, err := rt.admitQuota(r.Context(), projectID, from, delta)
	if err != nil {
		rt.internalError(w, "api: quota check failed", err, slog.String("project_id", projectID))
		return nil, false
	}
	if denial != nil {
		writeQuotaDenied(w, *denial)
		return nil, false
	}
	return release, true
}

// previewQuotaReason refuses a new preview that would exceed the owning
// project or team quota. The preview admit mutex already serializes callers.
func (rt *Router) previewQuotaReason(ctx context.Context, appName string) (string, error) {
	db := rt.quotaDB()
	if db == nil {
		return "", nil
	}
	owner, err := db.OwnerOfApp(ctx, appName)
	if err != nil || owner == nil {
		return "", err
	}
	denial, err := rt.quotaDenialFor(ctx, db, owner.ProjectID, store.ResourceOwner{}, quota.Amounts{Previews: 1})
	if err != nil || denial == nil {
		return "", err
	}
	return "Quota exceeded: " + denial.message() + ".", nil
}

type quotaLimitsResource struct {
	MaxApps      int64   `json:"max_apps"`
	MaxDatabases int64   `json:"max_databases"`
	MaxMemoryMB  int64   `json:"max_memory_mb"`
	MaxCPU       float64 `json:"max_cpu"`
	MaxPreviews  int64   `json:"max_previews"`
}

type quotaUsageResource struct {
	Apps      int64   `json:"apps"`
	Databases int64   `json:"databases"`
	MemoryMB  int64   `json:"memory_mb"`
	CPU       float64 `json:"cpu"`
	Previews  int64   `json:"previews"`
}

type quotaResource struct {
	Scope       string              `json:"scope"`
	ScopeID     string              `json:"scope_id"`
	Name        string              `json:"name"`
	Limits      quotaLimitsResource `json:"limits"`
	Effective   quotaLimitsResource `json:"effective_limits"`
	Usage       quotaUsageResource  `json:"usage"`
	WarnPercent int                 `json:"warn_percent"`
	CanManage   bool                `json:"can_manage"`
}

func limitsResource(a quota.Amounts) quotaLimitsResource {
	return quotaLimitsResource{
		MaxApps: a.Apps, MaxDatabases: a.Databases, MaxMemoryMB: a.MemoryBytes / bytesPerMB,
		MaxCPU: float64(a.NanoCPUs) / nanoCPUsPerCore, MaxPreviews: a.Previews,
	}
}

func (l quotaLimitsResource) amounts() (quota.Amounts, error) {
	if l.MaxApps < 0 || l.MaxDatabases < 0 || l.MaxMemoryMB < 0 || l.MaxCPU < 0 || l.MaxPreviews < 0 ||
		math.IsNaN(l.MaxCPU) || math.IsInf(l.MaxCPU, 0) {
		return quota.Amounts{}, errors.New("quota limits cannot be negative; use 0 for unlimited")
	}
	return quota.Amounts{
		Apps: l.MaxApps, Databases: l.MaxDatabases, MemoryBytes: l.MaxMemoryMB * bytesPerMB,
		NanoCPUs: int64(l.MaxCPU * nanoCPUsPerCore), Previews: l.MaxPreviews,
	}, nil
}

func usageResource(a quota.Amounts) quotaUsageResource {
	return quotaUsageResource{
		Apps: a.Apps, Databases: a.Databases, MemoryMB: a.MemoryBytes / bytesPerMB,
		CPU: float64(a.NanoCPUs) / nanoCPUsPerCore, Previews: a.Previews,
	}
}

// quotaTarget resolves {id} to the owning team and the scope being edited.
func (rt *Router) quotaTarget(w http.ResponseWriter, r *http.Request, scope string) (db quotaStore, scopeID, teamID string, ok bool) {
	db = rt.quotaDB()
	if db == nil {
		writeError(w, http.StatusNotImplemented, "quotas are not available on this control plane")
		return nil, "", "", false
	}
	scopeID, teamID = r.PathValue("id"), r.PathValue("id")
	if scope == quota.ScopeProject {
		t, err := db.TeamOfProject(r.Context(), scopeID)
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return nil, "", "", false
		}
		if err != nil {
			rt.internalError(w, "api: quota: team of project", err)
			return nil, "", "", false
		}
		teamID = t
	}
	return db, scopeID, teamID, true
}

func (rt *Router) handleGetQuota(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db, scopeID, teamID, ok := rt.quotaTarget(w, r, scope)
		if !ok {
			return
		}
		caller, err := rt.resolveTeamCaller(r, teamID)
		if err != nil {
			rt.internalError(w, "api: quota: resolve caller", err)
			return
		}
		res, err := rt.buildQuotaResource(r.Context(), db, scope, scopeID, caller.canManage(teamID))
		if err != nil {
			rt.internalError(w, "api: quota: load", err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func (rt *Router) buildQuotaResource(ctx context.Context, db quotaStore, scope, scopeID string, canManage bool) (quotaResource, error) {
	explicit, err := db.GetQuota(ctx, scope, scopeID)
	if err != nil {
		return quotaResource{}, err
	}
	usage, err := db.QuotaUsage(ctx, scope, scopeID)
	if err != nil {
		return quotaResource{}, err
	}
	return quotaResource{
		Scope: scope, ScopeID: scopeID, Name: rt.quotaScopeName(ctx, scope, scopeID),
		Limits: limitsResource(explicit), Effective: limitsResource(quota.Merge(explicit, quotaDefaults())),
		Usage: usageResource(usage), WarnPercent: quotaWarnPercent(), CanManage: canManage,
	}, nil
}

func (rt *Router) handleSetQuota(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db, scopeID, teamID, ok := rt.quotaTarget(w, r, scope)
		if !ok {
			return
		}
		caller, err := rt.resolveTeamCaller(r, teamID)
		if err != nil {
			rt.internalError(w, "api: quota: resolve caller", err)
			return
		}
		if !caller.canManage(teamID) {
			writeError(w, http.StatusForbidden, "changing quotas needs the team admin or owner role")
			return
		}
		var body quotaLimitsResource
		if err := decodeJSONBody(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		amounts, err := body.amounts()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := db.SetQuota(r.Context(), scope, scopeID, amounts); err != nil {
			rt.internalError(w, "api: quota: save", err)
			return
		}
		rt.recordTeamEvent(r, teamID, caller.userID, "quota.updated", scopeID, fmt.Sprintf("%s limits set", scope))
		rt.writeQuotaAfterChange(w, r, db, scope, scopeID, true)
	}
}

func (rt *Router) handleDeleteQuota(scope string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db, scopeID, teamID, ok := rt.quotaTarget(w, r, scope)
		if !ok {
			return
		}
		caller, err := rt.resolveTeamCaller(r, teamID)
		if err != nil {
			rt.internalError(w, "api: quota: resolve caller", err)
			return
		}
		if !caller.canManage(teamID) {
			writeError(w, http.StatusForbidden, "changing quotas needs the team admin or owner role")
			return
		}
		if err := db.DeleteQuota(r.Context(), scope, scopeID); err != nil {
			rt.internalError(w, "api: quota: delete", err)
			return
		}
		rt.recordTeamEvent(r, teamID, caller.userID, "quota.cleared", scopeID, fmt.Sprintf("%s limits cleared", scope))
		rt.writeQuotaAfterChange(w, r, db, scope, scopeID, true)
	}
}

func (rt *Router) writeQuotaAfterChange(w http.ResponseWriter, r *http.Request, db quotaStore, scope, scopeID string, canManage bool) {
	res, err := rt.buildQuotaResource(r.Context(), db, scope, scopeID, canManage)
	if err != nil {
		rt.internalError(w, "api: quota: reload", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
