package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxHostedBody        = 16 << 10
	hostedPreflightLimit = 3 * time.Minute
)

// hostedImportRequest is the body of both hosted routes. URL is a credential:
// it is read once, handed to the helper container and never stored or logged.
type hostedImportRequest struct {
	URL          string `json:"url"`
	WritesFrozen bool   `json:"writes_frozen"`
	AllowLarge   bool   `json:"allow_large"`
}

// hostedPreflightResponse carries only the redacted source and the findings.
type hostedPreflightResponse struct {
	Database        string                       `json:"database"`
	Source          string                       `json:"source"`
	Blocked         bool                         `json:"blocked"`
	TargetVersion   string                       `json:"target_version"`
	RequiredBytes   int64                        `json:"required_bytes"`
	EstimateSeconds int                          `json:"estimate_seconds"`
	Checks          []datamigrate.PreflightCheck `json:"checks"`
	Probe           datamigrate.HostedProbe      `json:"probe"`
}

// hostedTarget validates the request and resolves the existing managed
// Postgres database and its node runtime. It writes the error response itself.
func (rt *Router) hostedTarget(w http.ResponseWriter, r *http.Request, name string) (*store.DesiredDatabase, hostedImportRequest, datamigrate.HostedConfig, docker.Runtime, bool) {
	var (
		req hostedImportRequest
		cfg datamigrate.HostedConfig
	)
	db, ok := rt.loadDatabaseForRunner(w, r, name, "api: hosted import: load database failed")
	if !ok {
		return nil, req, cfg, nil, false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxHostedBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return nil, req, cfg, nil, false
	}
	if db.Engine != datamigrate.EnginePostgres {
		writeError(w, http.StatusBadRequest, "importing a hosted database needs a managed postgres database as the target")
		return nil, req, cfg, nil, false
	}
	cfg, err := datamigrate.HostedConfigFromProcessEnv()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, req, cfg, nil, false
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "copying data is not available on this control plane")
		return nil, req, cfg, nil, false
	}
	runtime, err := rt.execRuntime(db.NodeID)
	if err != nil {
		rt.logger.Error("api: hosted import: resolve node runtime failed", slog.String("error", err.Error()), slog.String("database", name), slog.String("node_id", db.NodeID))
		writeError(w, http.StatusBadGateway, "the database's node is not currently reachable")
		return nil, req, cfg, nil, false
	}
	return db, req, cfg, runtime, true
}

func hostedPlanFor(db *store.DesiredDatabase, req hostedImportRequest) datamigrate.HostedPlan {
	// The target exists by design: the user creates it first, so the name
	// collision check treats it as ours.
	return datamigrate.HostedPlan{URL: datamigrate.SecretURL(req.URL), TargetName: db.Name, TargetVersion: db.Version,
		TargetExists: true, OwnTarget: true, WritesFrozen: req.WritesFrozen, AllowLarge: req.AllowLarge}
}

// handleHostedPreflight handles POST /api/v1/imports/platform/databases/{name}/hosted/preflight.
// It probes the source with read-only queries and writes nothing.
func (rt *Router) handleHostedPreflight(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	db, req, cfg, runtime, ok := rt.hostedTarget(w, r, name)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), hostedPreflightLimit)
	defer cancel()
	copier := &datamigrate.Copier{Runtime: runtime, Logger: rt.logger}
	hs, res, err := copier.PreflightHosted(ctx, hostedPlanFor(db, req), cfg, envFloat(envHubDiskMargin, 1.5), envFloat(envHubMBPerSecond, 20))
	if err != nil {
		writeError(w, http.StatusBadRequest, datamigrate.Redact(hs.Scrub(err.Error())))
		return
	}
	writeJSON(w, http.StatusOK, hostedPreflightResponse{
		Database: name, Source: res.Redacted, Blocked: res.Preflight.Blocked, TargetVersion: res.Preflight.TargetVersion,
		RequiredBytes: res.Preflight.RequiredBytes, EstimateSeconds: res.Preflight.EstimateSeconds,
		Checks: res.Preflight.Checks, Probe: res.Probe,
	})
}

// handleHostedRun handles POST /api/v1/imports/platform/databases/{name}/hosted/run.
// It answers 202 and the move runs detached, read it with the data copy status route.
func (rt *Router) handleHostedRun(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	db, req, cfg, runtime, ok := rt.hostedTarget(w, r, name)
	if !ok {
		return
	}
	if !req.WritesFrozen {
		writeError(w, http.StatusConflict, datamigrate.ErrWritesNotFrozen.Error())
		return
	}
	hs, err := datamigrate.ParseHostedURL(req.URL, cfg)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	timeout := copyTimeout()
	err = rt.dataImports.ClaimDatabaseDataImport(r.Context(), name, hs.Source.Host, hs.Source.Port, hs.Source.Database, time.Now(), timeout)
	if errors.Is(err, store.ErrDataImportInProgress) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: hosted import: claim failed", err, slog.String("database", name))
		return
	}
	copier := &datamigrate.Copier{Runtime: runtime, Restorer: &backup.ContainerRestorer{Runtime: runtime}, Logger: rt.logger}
	go rt.runHostedMove(copier, hostedPlanFor(db, req), cfg, timeout, hs) //nolint:gosec // detached on purpose: r.Context() is cancelled when this handler returns

	row, found, _ := rt.dataImports.GetDatabaseDataImport(r.Context(), name)
	writeJSON(w, http.StatusAccepted, toDataImportResource(*db, row, found))
}

func (rt *Router) runHostedMove(c *datamigrate.Copier, plan datamigrate.HostedPlan, cfg datamigrate.HostedConfig, timeout time.Duration, hs datamigrate.HostedSource) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	res, err := c.MoveHosted(ctx, plan, cfg, envFloat(envHubDiskMargin, 1.5), envFloat(envHubMBPerSecond, 20))
	v := res.Verification
	status, reason := store.DataImportVerified, ""
	if err != nil {
		status, reason = store.DataImportFailed, datamigrate.Redact(hs.Scrub(err.Error()))
		rt.logger.Warn("api: hosted database move failed", slog.String("database", plan.TargetName), slog.String("reason", reason))
	} else {
		rt.logger.Info("api: hosted database move verified", slog.String("database", plan.TargetName), slog.Int("tables", v.Checked))
	}
	detail, _ := json.Marshal(v)
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer finishCancel()
	if ferr := rt.dataImports.FinishDatabaseDataImport(finishCtx, plan.TargetName, status, reason, v.Checked, v.Mismatched, string(detail), time.Now()); ferr != nil {
		rt.logger.Error("api: record hosted database move outcome failed", slog.String("error", ferr.Error()), slog.String("database", plan.TargetName))
	}
}
