package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploy/secretscan"
)

const secretScanBodyLimit = 4 << 10

// SecretScanService is the deploy-time secret scanning surface; *secretscan.Service satisfies it.
type SecretScanService interface {
	Limits() secretscan.Limits
	Policy(ctx context.Context, app string) (secretscan.Policy, error)
	SetPolicy(ctx context.Context, app, policy string) (secretscan.Policy, error)
	AllowList(ctx context.Context, app string) ([]secretscan.AllowEntry, error)
	Allow(ctx context.Context, app, fingerprint, reason, actor string) (secretscan.AllowEntry, error)
	Unallow(ctx context.Context, app, fingerprint string) (bool, error)
	Run(ctx context.Context, attemptID string) (secretscan.Run, error)
	ScanEnvNow(ctx context.Context, app string, env map[string]string) (secretscan.Result, error)
	DeleteApp(ctx context.Context, app string) error
}

// SetSecretScan enables the secret scanning routes.
func (rt *Router) SetSecretScan(s SecretScanService) { rt.secretScan = s }

type secretScanLimitsResource struct {
	MaxFiles       int   `json:"max_files"`
	MaxFileBytes   int64 `json:"max_file_bytes"`
	MaxTotalBytes  int64 `json:"max_total_bytes"`
	MaxFindings    int   `json:"max_findings"`
	TimeoutSeconds int   `json:"timeout_seconds"`
}

type secretScanSettingsResource struct {
	App       string                   `json:"app"`
	Policy    string                   `json:"policy"`
	AllowList []secretscan.AllowEntry  `json:"allow_list"`
	Limits    secretScanLimitsResource `json:"limits"`
}

type secretScanRunResource struct {
	Scope        string               `json:"scope"`
	FilesScanned int                  `json:"files_scanned"`
	Truncated    bool                 `json:"truncated"`
	Findings     []secretscan.Finding `json:"findings"`
	ScannedAt    time.Time            `json:"scanned_at"`
}

func (rt *Router) secretScanReady(w http.ResponseWriter, r *http.Request) bool {
	if rt.secretScan == nil {
		writeError(w, http.StatusNotImplemented, "secret scanning is not configured on this control plane")
		return false
	}
	return rt.requireApp(w, r)
}

func (rt *Router) writeSecretScanError(w http.ResponseWriter, err error, msg, app string) {
	switch {
	case errors.Is(err, secretscan.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, secretscan.ErrRunNotFound):
		writeError(w, http.StatusNotFound, "no secret scan recorded for this deployment")
	default:
		rt.internalError(w, msg, err, slog.String("name", app))
	}
}

func (rt *Router) writeSecretScanSettings(w http.ResponseWriter, r *http.Request, app string) {
	policy, err := rt.secretScan.Policy(r.Context(), app)
	if err != nil {
		rt.writeSecretScanError(w, err, "api: load secret scan policy failed", app)
		return
	}
	allow, err := rt.secretScan.AllowList(r.Context(), app)
	if err != nil {
		rt.writeSecretScanError(w, err, "api: load secret scan allow-list failed", app)
		return
	}
	l := rt.secretScan.Limits()
	writeJSON(w, http.StatusOK, secretScanSettingsResource{
		App: app, Policy: string(policy), AllowList: allow,
		Limits: secretScanLimitsResource{MaxFiles: l.MaxFiles, MaxFileBytes: l.MaxFileBytes, MaxTotalBytes: l.MaxTotalBytes, MaxFindings: l.MaxFindings, TimeoutSeconds: int(l.Timeout.Seconds())},
	})
}

// handleGetSecretScan handles GET /api/v1/apps/{name}/secret-scan.
func (rt *Router) handleGetSecretScan(w http.ResponseWriter, r *http.Request) {
	if !rt.secretScanReady(w, r) {
		return
	}
	rt.writeSecretScanSettings(w, r, r.PathValue("name"))
}

// handlePutSecretScan handles PUT /api/v1/apps/{name}/secret-scan.
func (rt *Router) handlePutSecretScan(w http.ResponseWriter, r *http.Request) {
	if !rt.secretScanReady(w, r) {
		return
	}
	var req struct {
		Policy string `json:"policy"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, secretScanBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	app := r.PathValue("name")
	if _, err := rt.secretScan.SetPolicy(r.Context(), app, req.Policy); err != nil {
		rt.writeSecretScanError(w, err, "api: save secret scan policy failed", app)
		return
	}
	rt.writeSecretScanSettings(w, r, app)
}

// handleRunSecretScan handles POST /api/v1/apps/{name}/secret-scan/run.
func (rt *Router) handleRunSecretScan(w http.ResponseWriter, r *http.Request) {
	if !rt.secretScanReady(w, r) {
		return
	}
	app := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: load app for secret scan failed", err, slog.String("name", app))
		return
	}
	res, err := rt.secretScan.ScanEnvNow(r.Context(), app, svc.Env)
	if err != nil {
		rt.writeSecretScanError(w, err, "api: scan app env failed", app)
		return
	}
	if res.Findings == nil {
		res.Findings = []secretscan.Finding{}
	}
	writeJSON(w, http.StatusOK, secretScanRunResource{Scope: secretscan.SourceEnv, Truncated: res.Truncated, Findings: res.Findings, ScannedAt: time.Now().UTC()})
}

// handleAllowSecretScan handles POST /api/v1/apps/{name}/secret-scan/allow.
func (rt *Router) handleAllowSecretScan(w http.ResponseWriter, r *http.Request) {
	if !rt.secretScanReady(w, r) {
		return
	}
	var req struct {
		Fingerprint string `json:"fingerprint"`
		Reason      string `json:"reason"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, secretScanBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	app := r.PathValue("name")
	_, _, actor, _ := rt.currentActor(r)
	entry, err := rt.secretScan.Allow(r.Context(), app, req.Fingerprint, req.Reason, actor)
	if err != nil {
		rt.writeSecretScanError(w, err, "api: allow secret scan fingerprint failed", app)
		return
	}
	rt.logger.Warn("api: secret scan fingerprint allow-listed", slog.String("name", app), slog.String("fingerprint", entry.Fingerprint), slog.String("actor", actor))
	writeJSON(w, http.StatusCreated, entry)
}

// handleUnallowSecretScan handles DELETE /api/v1/apps/{name}/secret-scan/allow/{fingerprint}.
func (rt *Router) handleUnallowSecretScan(w http.ResponseWriter, r *http.Request) {
	if !rt.secretScanReady(w, r) {
		return
	}
	app := r.PathValue("name")
	ok, err := rt.secretScan.Unallow(r.Context(), app, r.PathValue("fingerprint"))
	if err != nil {
		rt.writeSecretScanError(w, err, "api: remove secret scan allow entry failed", app)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "allow-list entry not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetDeploymentSecretScan handles GET /api/v1/apps/{name}/deployments/{id}/secret-scan.
func (rt *Router) handleGetDeploymentSecretScan(w http.ResponseWriter, r *http.Request) {
	if !rt.secretScanReady(w, r) {
		return
	}
	app := r.PathValue("name")
	run, err := rt.secretScan.Run(r.Context(), r.PathValue("id"))
	if err == nil && run.App != app {
		err = secretscan.ErrRunNotFound
	}
	if err != nil {
		rt.writeSecretScanError(w, err, "api: get deployment secret scan failed", app)
		return
	}
	writeJSON(w, http.StatusOK, run)
}
