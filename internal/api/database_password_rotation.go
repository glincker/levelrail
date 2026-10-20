package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbrotate"
	"github.com/GLINCKER/levelrail/internal/store"
)

// WithDatabasePasswordRotation enables database owner password rotation and
// its schedule. *store.DB satisfies s.
func WithDatabasePasswordRotation(s dbrotate.Store) Option {
	return func(rt *Router) { rt.dbRotationStore = s }
}

const (
	dbPasswordRotationStepUpApp = "database-password"
	errDBPasswordRotationOff    = "database password rotation is not configured on this control plane"
	hoursPerDay                 = 24
)

// databasePasswordRotationResource never carries a password.
type databasePasswordRotationResource struct {
	Database        string   `json:"database"`
	Engine          string   `json:"engine"`
	Supported       bool     `json:"supported"`
	Reason          string   `json:"reason,omitempty"`
	RestartRequired bool     `json:"restart_required"`
	Apps            []string `json:"apps"`
	SuspendedApps   []string `json:"suspended_apps"`
	State           string   `json:"state"`
	LastError       string   `json:"last_error,omitempty"`
	LastChangedAt   string   `json:"last_changed_at,omitempty"`
	AgeDays         *int     `json:"age_days,omitempty"`
	IntervalDays    int      `json:"interval_days"`
	NextDue         string   `json:"next_due,omitempty"`
	RestartedApps   []string `json:"restarted_apps,omitempty"`
	RotatedAt       string   `json:"rotated_at,omitempty"`
}

type databasePasswordRotateRequest struct {
	DryRun bool         `json:"dry_run"`
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

type databasePasswordPolicyRequest struct {
	IntervalDays int `json:"interval_days"`
}

// dbRotationService builds the one shared service, or nil when rotation is
// not wired. Handlers and the scheduler share it so its per-database lock holds.
func (rt *Router) dbRotationService() *dbrotate.Service {
	rt.dbRotationOnce.Do(func() {
		if rt.dbRotationStore == nil || rt.secrets == nil || rt.databases == nil || rt.execRuntime == nil {
			return
		}
		rt.dbRotationSvc = &dbrotate.Service{
			Store:     rt.dbRotationStore,
			Databases: rt.databases,
			Secrets:   dbRotationSecrets{rt.secrets},
			Target:    dbRotationTarget{rt},
			Apps:      dbRotationApps{rt},
			Auditor:   dbRotationAuditor{rt},
			Logger:    rt.logger,
		}
	})
	return rt.dbRotationSvc
}

// RunDatabasePasswordRotation settles interrupted rotations and runs scheduled
// ones until ctx ends. It returns nil at once when rotation is not wired.
func (rt *Router) RunDatabasePasswordRotation(ctx context.Context, interval time.Duration) error {
	svc := rt.dbRotationService()
	if svc == nil {
		return nil
	}
	return svc.Run(ctx, interval)
}

type dbRotationSecrets struct{ SecretSetter }

func (s dbRotationSecrets) DeleteValue(ctx context.Context, serviceName, envKey string) error {
	if d, ok := s.SecretSetter.(interface {
		DeleteValue(ctx context.Context, serviceName, envKey string) error
	}); ok {
		return d.DeleteValue(ctx, serviceName, envKey)
	}
	return s.SetValueGuarded(ctx, serviceName, envKey, "", true)
}

type dbRotationTarget struct{ rt *Router }

func (t dbRotationTarget) Locate(ctx context.Context, d *store.DesiredDatabase) (dbrotate.Exec, string, error) {
	nodeRuntime, err := t.rt.execRuntime(d.NodeID)
	if err != nil {
		return nil, "", fmt.Errorf("resolve runtime for database %q: %w", d.Name, err)
	}
	inspectCtx, cancel := context.WithTimeout(ctx, dockerInspectTimeout)
	defer cancel()
	state, err := nodeRuntime.InspectByName(inspectCtx, databaseContainerName(d.Name))
	if err != nil {
		return nil, "", fmt.Errorf("inspect database %q: %w", d.Name, err)
	}
	if state == nil || !state.Running {
		return nil, "", dbrotate.ErrNotRunning
	}
	return nodeRuntime, state.ID, nil
}

type dbRotationApps struct{ rt *Router }

func (a dbRotationApps) Dependents(ctx context.Context, database string) ([]string, []string, error) {
	svcs, err := a.rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("list apps: %w", err)
	}
	running, suspended := []string{}, []string{}
	for i := range svcs {
		if !usesDatabase(&svcs[i], database) {
			continue
		}
		if svcs[i].Suspended {
			suspended = append(suspended, svcs[i].Name)
		} else {
			running = append(running, svcs[i].Name)
		}
	}
	return running, suspended, nil
}

func (a dbRotationApps) Restart(ctx context.Context, database string) []string {
	return a.rt.restartAppsUsingDatabase(ctx, database)
}

type dbRotationAuditor struct{ rt *Router }

func (a dbRotationAuditor) Record(ctx context.Context, action, database, actor string) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		a.rt.logger.Warn("api: database password audit id failed", slog.String("error", err.Error()))
		return
	}
	kind, client := "user", "api"
	if actor == dbrotate.SystemActor {
		kind, client = "system", "system"
	}
	entry := store.AuditEntry{
		ID: id, ActorType: kind, ActorName: actor, Ability: AbilityRoot, Method: action,
		Path: "/api/v1/databases/" + database, StatusCode: http.StatusOK,
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: client,
	}
	if err := a.rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		a.rt.logger.Warn("api: database password audit save failed", slog.String("error", err.Error()), slog.String("database", database))
	}
}

func (rt *Router) rotationOrOff(w http.ResponseWriter) *dbrotate.Service {
	svc := rt.dbRotationService()
	if svc == nil {
		writeError(w, http.StatusNotImplemented, errDBPasswordRotationOff)
	}
	return svc
}

func (rt *Router) writeDBRotationError(w http.ResponseWriter, msg, name string, err error) {
	switch {
	case errors.Is(err, store.ErrDatabaseNotFound):
		writeError(w, http.StatusNotFound, "database not found")
	case errors.Is(err, dbrotate.ErrUnsupported):
		writeError(w, http.StatusBadRequest, dbrotate.ErrUnsupported.Error())
	case errors.Is(err, dbrotate.ErrBadInterval):
		writeError(w, http.StatusBadRequest, dbrotate.ErrBadInterval.Error())
	case errors.Is(err, dbrotate.ErrNotRunning):
		writeError(w, http.StatusConflict, dbrotate.ErrNotRunning.Error())
	case errors.Is(err, dbrotate.ErrVerifyFailed):
		writeCodedError(w, http.StatusConflict, "verify_failed", dbrotate.ErrVerifyFailed.Error())
	case errors.Is(err, dbrotate.ErrPending):
		writeCodedError(w, http.StatusConflict, "rotation_pending", "a previous rotation is still settling, try again shortly")
	default:
		rt.internalError(w, msg+" ("+name+")", err)
	}
}

func rotationResource(p dbrotate.Preview, now time.Time) databasePasswordRotationResource {
	res := databasePasswordRotationResource{
		Database: p.Database, Engine: p.Engine, Supported: p.Supported, Reason: p.Reason,
		RestartRequired: p.RestartRequired, Apps: nonNil(p.Apps), SuspendedApps: nonNil(p.SuspendedApps),
		State: p.State, LastError: p.LastError, IntervalDays: p.IntervalDays,
	}
	if !p.LastChangedAt.IsZero() {
		res.LastChangedAt = p.LastChangedAt.UTC().Format(time.RFC3339)
		age := int(now.Sub(p.LastChangedAt).Hours() / hoursPerDay)
		res.AgeDays = &age
	}
	if !p.NextDue.IsZero() {
		res.NextDue = p.NextDue.UTC().Format(time.RFC3339)
	}
	return res
}

// visibleApps drops apps the caller may not read, so the preview never names them.
func (rt *Router) visibleApps(r *http.Request, names []string) []string {
	out := []string{}
	for _, n := range names {
		denied, err := rt.firstDeniedRef(r, AbilityRead, appRefs([]string{n}))
		if err == nil && denied == nil {
			out = append(out, n)
		}
	}
	return out
}

func (rt *Router) previewResource(r *http.Request, svc *dbrotate.Service, name string) (databasePasswordRotationResource, error) {
	p, err := svc.Preview(r.Context(), name)
	if err != nil {
		return databasePasswordRotationResource{}, err
	}
	res := rotationResource(p, time.Now())
	res.Apps = rt.visibleApps(r, p.Apps)
	res.SuspendedApps = rt.visibleApps(r, p.SuspendedApps)
	return res, nil
}

// handleGetDatabasePasswordRotation handles GET /api/v1/databases/{name}/password-rotation.
func (rt *Router) handleGetDatabasePasswordRotation(w http.ResponseWriter, r *http.Request) {
	svc := rt.rotationOrOff(w)
	if svc == nil {
		return
	}
	name := r.PathValue("name")
	res, err := rt.previewResource(r, svc, name)
	if err != nil {
		rt.writeDBRotationError(w, "api: database password rotation preview failed", name, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleRotateDatabasePassword handles POST /api/v1/databases/{name}/password-rotation.
// dry_run answers the preview without a step-up. A real rotation needs one.
func (rt *Router) handleRotateDatabasePassword(w http.ResponseWriter, r *http.Request) {
	svc := rt.rotationOrOff(w)
	if svc == nil {
		return
	}
	var req databasePasswordRotateRequest
	if err := decodeJSONBodyLimit(r, &req, stepUpBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, errBadBody)
		return
	}
	name := r.PathValue("name")
	if req.DryRun {
		res, err := rt.previewResource(r, svc, name)
		if err != nil {
			rt.writeDBRotationError(w, "api: database password rotation preview failed", name, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
		return
	}
	userID, ok := rt.stepUpOwner(w, r, req.StepUp, dbPasswordRotationStepUpApp+":"+name, "rotate a database password", "password rotation")
	if !ok {
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	if actor == "" {
		actor = userID
	}
	out, err := svc.Rotate(r.Context(), name, actor)
	if err != nil {
		rt.writeDBRotationError(w, "api: database password rotation failed", name, err)
		return
	}
	res, perr := rt.previewResource(r, svc, name)
	if perr != nil {
		rt.writeDBRotationError(w, "api: database password rotation status failed", name, perr)
		return
	}
	res.RestartedApps = rt.visibleApps(r, out.RestartedApps)
	res.RotatedAt = out.RotatedAt.UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, res)
}

// handleSetDatabasePasswordPolicy handles PUT /api/v1/databases/{name}/password-rotation/policy.
func (rt *Router) handleSetDatabasePasswordPolicy(w http.ResponseWriter, r *http.Request) {
	svc := rt.rotationOrOff(w)
	if svc == nil {
		return
	}
	var req databasePasswordPolicyRequest
	if err := decodeJSONBodyLimit(r, &req, stepUpBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, errBadBody)
		return
	}
	name := r.PathValue("name")
	_, _, actor, _ := rt.currentActor(r)
	if err := svc.SetInterval(r.Context(), name, req.IntervalDays, actor); err != nil {
		rt.writeDBRotationError(w, "api: database password policy failed", name, err)
		return
	}
	res, err := rt.previewResource(r, svc, name)
	if err != nil {
		rt.writeDBRotationError(w, "api: database password policy status failed", name, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
