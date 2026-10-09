package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type environmentMoveKind string

const (
	environmentMoveApp      environmentMoveKind = "app"
	environmentMoveDatabase environmentMoveKind = "database"
)

var errNoDatabaseEnvironmentStore = errors.New("api: database store cannot set environments")

type databaseEnvironmentStore interface {
	SetDatabaseEnvironment(ctx context.Context, databaseName, envID string) error
	DatabaseNamesInEnvironment(ctx context.Context, envID string) (map[string]bool, error)
}

// environmentMove is one request to retag an app or database.
type environmentMove struct {
	kind      environmentMoveKind
	name      string
	currentID string
	targetID  string
	confirm   bool
	freeze    freezeOverride
}

// moveToEnvironment retags an app or database. A protected source or
// target needs confirm: true and then becomes a pending approval; the
// freeze gate always applies. It writes its own response.
func (rt *Router) moveToEnvironment(w http.ResponseWriter, r *http.Request, m environmentMove) {
	ctx := r.Context()
	if m.targetID == m.currentID {
		rt.writeMovedResource(w, r, m)
		return
	}
	var target, source store.Environment
	if m.targetID != "" {
		e, err := rt.environments.GetEnvironment(ctx, m.targetID)
		if errors.Is(err, store.ErrEnvironmentNotFound) {
			writeError(w, http.StatusBadRequest, "unknown environment_id")
			return
		}
		if err != nil {
			rt.internalError(w, "api: move to environment: load target failed", err, slog.String("name", m.name))
			return
		}
		target = e
	}
	if m.currentID != "" {
		e, err := rt.environments.GetEnvironment(ctx, m.currentID)
		switch {
		case err == nil:
			source = e
		case errors.Is(err, store.ErrEnvironmentNotFound):
		default:
			rt.internalError(w, "api: move to environment: load source failed", err, slog.String("name", m.name))
			return
		}
	}
	note, ok := rt.freezeGate(w, r, m.name, m.freeze)
	if !ok {
		return
	}
	if gate := protectedMoveEnvironment(source, target); gate.ID != "" {
		if !m.confirm {
			writeEnvironmentConfirmationRequired(w, gate)
			return
		}
		rt.requestMoveApproval(w, r, m, gate, note)
		return
	}
	if err := rt.applyEnvironmentMove(ctx, m.kind, m.name, m.targetID); err != nil {
		rt.writeMoveError(w, m, err)
		return
	}
	rt.recordMoveEvent(r, m, source, target)
	rt.writeMovedResource(w, r, m)
}

// protectedMoveEnvironment returns the protected environment a move touches,
// preferring the target, or a zero value when neither is protected.
func protectedMoveEnvironment(source, target store.Environment) store.Environment {
	if target.Protected {
		return target
	}
	if source.Protected {
		return source
	}
	return store.Environment{}
}

func (rt *Router) requestMoveApproval(w http.ResponseWriter, r *http.Request, m environmentMove, gate store.Environment, note string) {
	action := store.DeployApprovalActionMoveApp
	if m.kind == environmentMoveDatabase {
		action = store.DeployApprovalActionMoveDatabase
	}
	approval, ok := rt.requestDeployApproval(w, r, gate, m.name, "", action, m.targetID, deployApprovalOptions{freezeOverride: note})
	if !ok {
		return
	}
	writeJSON(w, http.StatusAccepted, deployTriggerResult{PendingApproval: &approval})
}

func (rt *Router) applyEnvironmentMove(ctx context.Context, kind environmentMoveKind, name, targetID string) error {
	if kind == environmentMoveDatabase {
		ds, ok := rt.databases.(databaseEnvironmentStore)
		if !ok {
			return errNoDatabaseEnvironmentStore
		}
		return ds.SetDatabaseEnvironment(ctx, name, targetID)
	}
	if err := rt.environments.SetServiceEnvironment(ctx, name, targetID); err != nil {
		return err
	}
	// The active environment picks the routed domain set.
	rt.nudgeReconciler()
	return nil
}

func (rt *Router) writeMoveError(w http.ResponseWriter, m environmentMove, err error) {
	switch {
	case errors.Is(err, store.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "app not found")
	case errors.Is(err, store.ErrDatabaseNotFound):
		writeError(w, http.StatusNotFound, "database not found")
	default:
		rt.internalError(w, "api: move to environment failed", err, slog.String("name", m.name))
	}
}

func (rt *Router) writeMovedResource(w http.ResponseWriter, r *http.Request, m environmentMove) {
	if m.kind == environmentMoveDatabase {
		rt.reloadAndWriteDatabase(w, r, m.name, "set database environment")
		return
	}
	rt.reloadAndWriteApp(w, r, m.name, "set app environment")
}

func (rt *Router) recordMoveEvent(r *http.Request, m environmentMove, source, target store.Environment) {
	rt.logger.Info("api: environment changed", slog.String("kind", string(m.kind)), slog.String("name", m.name), slog.String("from", source.ID), slog.String("to", target.ID))
	if m.kind != environmentMoveApp {
		return
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: m.name, Kind: store.AppEventConfigChange, Title: "Environment changed", Detail: environmentLabel(source) + " to " + environmentLabel(target)})
}

func environmentLabel(e store.Environment) string {
	if e.ID == "" {
		return "none"
	}
	return e.Name
}

// applyMoveApproval runs an approved environment move and records the
// decision. The approval's Image column carries the target environment id.
func (rt *Router) applyMoveApproval(ctx context.Context, a store.DeployApproval, deciderType, deciderID, deciderName string) (deployApprovalDecisionResponse, error) {
	kind := environmentMoveApp
	if a.Action == store.DeployApprovalActionMoveDatabase {
		kind = environmentMoveDatabase
	}
	if a.Image != "" {
		if _, err := rt.environments.GetEnvironment(ctx, a.Image); err != nil {
			return deployApprovalDecisionResponse{}, &deployApprovalDecisionError{http.StatusConflict, "the target environment no longer exists"}
		}
	}
	if _, err := rt.approvalFreezeNote(ctx, a); err != nil {
		return deployApprovalDecisionResponse{}, err
	}
	if err := rt.applyEnvironmentMove(ctx, kind, a.ServiceName, a.Image); err != nil {
		if errors.Is(err, store.ErrServiceNotFound) || errors.Is(err, store.ErrDatabaseNotFound) {
			return deployApprovalDecisionResponse{}, &deployApprovalDecisionError{http.StatusConflict, a.ServiceName + " no longer exists"}
		}
		return deployApprovalDecisionResponse{}, err
	}
	decidedAt := store.FormatAuditTime(time.Now())
	if _, err := rt.deployApprovals.DecideDeployApproval(ctx, a.ID, store.DeployApprovalStatusApproved, deciderType, deciderID, deciderName, "", decidedAt); err != nil {
		rt.logger.Warn("api: record deploy approval decision failed", slog.String("error", err.Error()), slog.String("id", a.ID))
	}
	final, err := rt.deployApprovals.GetDeployApproval(ctx, a.ID)
	if err != nil {
		return deployApprovalDecisionResponse{}, err
	}
	resp := deployApprovalDecisionResponse{Approval: toDeployApprovalResource(final)}
	if kind == environmentMoveApp {
		if svc, err := rt.apps.GetDesiredService(ctx, a.ServiceName); err == nil {
			resp.App = toAppResource(*svc)
		}
	}
	return resp, nil
}

type setDatabaseEnvironmentRequest struct {
	EnvironmentID string `json:"environment_id"`
	Confirm       bool   `json:"confirm,omitempty"`
	freezeOverride
}

// handleSetDatabaseEnvironment handles PUT /api/v1/databases/{name}/environment.
func (rt *Router) handleSetDatabaseEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req setDatabaseEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set database environment: load failed", err, slog.String("name", name))
		return
	}
	ref, err := rt.environments.EnvironmentOfDatabase(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: set database environment: resolve current failed", err, slog.String("name", name))
		return
	}
	current := ""
	if ref != nil {
		current = ref.ID
	}
	rt.moveToEnvironment(w, r, environmentMove{
		kind: environmentMoveDatabase, name: name, currentID: current,
		targetID: req.EnvironmentID, confirm: req.Confirm, freeze: req.freezeOverride,
	})
}

// filterDatabasesByEnvironment applies ?environment=<id or name> to a database list.
func (rt *Router) filterDatabasesByEnvironment(r *http.Request, dbs []store.DesiredDatabase) ([]store.DesiredDatabase, error) {
	want := r.URL.Query().Get("environment")
	ds, ok := rt.databases.(databaseEnvironmentStore)
	if want == "" || !ok {
		return dbs, nil
	}
	envID := want
	if e, err := rt.environments.GetEnvironment(r.Context(), want); err == nil {
		envID = e.ID
	} else if all, listErr := rt.environments.ListAllEnvironments(r.Context()); listErr == nil {
		for _, e := range all {
			if e.Name == want {
				envID = e.ID
				break
			}
		}
	}
	members, err := ds.DatabaseNamesInEnvironment(r.Context(), envID)
	if err != nil {
		return nil, err
	}
	out := dbs[:0:0]
	for _, d := range dbs {
		if members[d.Name] {
			out = append(out, d)
		}
	}
	return out, nil
}
