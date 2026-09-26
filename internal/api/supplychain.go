package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/supplychain"
)

const supplyChainBodyLimit = 4 << 10

// SupplyChainService is the SBOM and scan surface; *supplychain.Service satisfies it.
type SupplyChainService interface {
	Config() supplychain.Config
	Settings(ctx context.Context, app string) (supplychain.Settings, error)
	SaveSettings(ctx context.Context, app string, patch supplychain.SettingsPatch) (supplychain.Settings, error)
	ArmOverride(ctx context.Context, app, reason string) (supplychain.Settings, error)
	Get(ctx context.Context, app, attemptID string) (supplychain.Record, error)
	OpenSBOM(ctx context.Context, app, attemptID string) ([]byte, supplychain.Record, error)
	ScanNow(ctx context.Context, app, attemptID string) (supplychain.Record, error)
	Lookup(ctx context.Context, ids []string) (map[string]supplychain.Record, error)
	DeleteApp(ctx context.Context, app string) error
}

// SetSupplyChain enables the SBOM and scan routes.
func (rt *Router) SetSupplyChain(s SupplyChainService) { rt.supplyChain = s }

type sbomResource struct {
	DeploymentID string                     `json:"deployment_id"`
	Format       string                     `json:"format"`
	PackageCount int                        `json:"package_count"`
	Types        []supplychain.TypeCount    `json:"types"`
	Licenses     []supplychain.LicenseCount `json:"licenses"`
	Unlicensed   int                        `json:"unlicensed"`
	TopPackages  []supplychain.Package      `json:"top_packages"`
	Provenance   bool                       `json:"provenance"`
	// Available is false once retention removed the document; the summary stays.
	Available   bool      `json:"available"`
	Bytes       int64     `json:"bytes"`
	GeneratedAt time.Time `json:"generated_at"`
	DownloadURL string    `json:"download_url,omitempty"`
}

type vulnScanResource struct {
	Status    string              `json:"status"`
	Scanner   string              `json:"scanner,omitempty"`
	ScannedAt *time.Time          `json:"scanned_at,omitempty"`
	Error     string              `json:"error,omitempty"`
	Counts    *supplychain.Counts `json:"counts,omitempty"`
	Fixable   int                 `json:"fixable"`
	Top       []supplychain.Vuln  `json:"top"`
}

type vulnGateResource struct {
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

type vulnResource struct {
	DeploymentID string            `json:"deployment_id"`
	Scan         vulnScanResource  `json:"scan"`
	Gate         *vulnGateResource `json:"gate,omitempty"`
}

type supplyChainSettingsResource struct {
	App             string     `json:"app"`
	ScanEnabled     bool       `json:"scan_enabled"`
	ScanGate        string     `json:"scan_gate"`
	ServerEnabled   bool       `json:"server_enabled"`
	BuildAttest     bool       `json:"build_attest"`
	Scanner         string     `json:"scanner"`
	ScannerImage    string     `json:"scanner_image"`
	OverrideArmed   bool       `json:"override_armed"`
	OverrideReason  string     `json:"override_reason,omitempty"`
	OverrideExpires *time.Time `json:"override_expires_at,omitempty"`
}

func sbomDownloadURL(app, id string) string {
	return "/api/v1/apps/" + url.PathEscape(app) + "/deployments/" + url.PathEscape(id) + "/sbom?download=true"
}

func toSBOMResource(rec supplychain.Record) sbomResource {
	out := sbomResource{
		DeploymentID: rec.AttemptID, Format: rec.Summary.Format, PackageCount: rec.Summary.PackageCount,
		Types: rec.Summary.Types, Licenses: rec.Summary.Licenses, Unlicensed: rec.Summary.Unlicensed, TopPackages: rec.Summary.TopPackages,
		Provenance: rec.HasProvenance, Available: rec.HasSBOM(), Bytes: rec.SBOMBytes, GeneratedAt: rec.GeneratedAt.UTC(),
	}
	if out.Types == nil {
		out.Types = []supplychain.TypeCount{}
	}
	if out.Licenses == nil {
		out.Licenses = []supplychain.LicenseCount{}
	}
	if out.TopPackages == nil {
		out.TopPackages = []supplychain.Package{}
	}
	if out.Available {
		out.DownloadURL = sbomDownloadURL(rec.App, rec.AttemptID)
	}
	return out
}

func toVulnResource(rec supplychain.Record) vulnResource {
	out := vulnResource{DeploymentID: rec.AttemptID, Scan: vulnScanResource{Status: rec.ScanStatus, Scanner: rec.Scanner, Error: rec.ScanError, Top: []supplychain.Vuln{}}}
	if !rec.ScannedAt.IsZero() {
		t := rec.ScannedAt.UTC()
		out.Scan.ScannedAt = &t
	}
	if rec.Scan != nil {
		c := rec.Scan.Counts
		out.Scan.Counts, out.Scan.Fixable = &c, rec.Scan.Fixable
		if rec.Scan.Top != nil {
			out.Scan.Top = rec.Scan.Top
		}
	}
	if rec.GateAction != "" {
		out.Gate = &vulnGateResource{Action: rec.GateAction, Reason: rec.GateReason}
	}
	return out
}

func (rt *Router) toSupplyChainSettings(app string, cfg supplychain.Config, st supplychain.Settings) supplyChainSettingsResource {
	out := supplyChainSettingsResource{
		App: app, ScanEnabled: st.Enabled, ScanGate: string(st.Gate), ServerEnabled: cfg.Enabled, BuildAttest: cfg.BuildAttest,
		Scanner: cfg.Scanner, ScannerImage: cfg.Image,
	}
	if out.ScanGate == "" {
		out.ScanGate = string(supplychain.GateOff)
	}
	if st.OverrideReason != "" && !st.OverrideArmedAt.IsZero() {
		exp := st.OverrideArmedAt.Add(cfg.OverrideTTL).UTC()
		if exp.After(time.Now()) {
			out.OverrideArmed, out.OverrideReason, out.OverrideExpires = true, st.OverrideReason, &exp
		}
	}
	return out
}

// supplyChainRecordSummary is the additive per-deploy pair on deploy lists.
func supplyChainRecordSummary(rec supplychain.Record) (packages int, counts *supplychain.Counts) {
	if rec.Scan != nil {
		c := rec.Scan.Counts
		counts = &c
	}
	return rec.Summary.PackageCount, counts
}

func (rt *Router) supplyChainReady(w http.ResponseWriter, r *http.Request) bool {
	if rt.supplyChain == nil {
		writeError(w, http.StatusNotImplemented, "supply chain visibility is not configured on this control plane")
		return false
	}
	return rt.requireApp(w, r)
}

// supplyChainRecords maps deploy attempt IDs to records; failures yield an empty map.
func (rt *Router) supplyChainRecords(ctx context.Context, ids []string) map[string]supplychain.Record {
	if rt.supplyChain == nil || len(ids) == 0 {
		return nil
	}
	recs, err := rt.supplyChain.Lookup(ctx, ids)
	if err != nil {
		rt.logger.Warn("api: look up supply chain records failed", slog.String("error", err.Error()))
		return nil
	}
	return recs
}

func (rt *Router) writeSupplyChainSettings(w http.ResponseWriter, r *http.Request, status int) {
	app := r.PathValue("name")
	st, err := rt.supplyChain.Settings(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: load supply chain settings failed", err, slog.String("name", app))
		return
	}
	writeJSON(w, status, rt.toSupplyChainSettings(app, rt.supplyChain.Config(), st))
}

// handleGetSupplyChain handles GET /api/v1/apps/{name}/supply-chain.
func (rt *Router) handleGetSupplyChain(w http.ResponseWriter, r *http.Request) {
	if !rt.supplyChainReady(w, r) {
		return
	}
	rt.writeSupplyChainSettings(w, r, http.StatusOK)
}

type supplyChainSettingsRequest struct {
	ScanEnabled *bool   `json:"scan_enabled"`
	ScanGate    *string `json:"scan_gate"`
}

// handlePutSupplyChain handles PUT /api/v1/apps/{name}/supply-chain.
func (rt *Router) handlePutSupplyChain(w http.ResponseWriter, r *http.Request) {
	if !rt.supplyChainReady(w, r) {
		return
	}
	var req supplyChainSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, supplyChainBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	app := r.PathValue("name")
	if _, err := rt.supplyChain.SaveSettings(r.Context(), app, supplychain.SettingsPatch{Enabled: req.ScanEnabled, Gate: req.ScanGate}); err != nil {
		rt.writeSupplyChainError(w, err, "api: save supply chain settings failed", app)
		return
	}
	rt.writeSupplyChainSettings(w, r, http.StatusOK)
}

type supplyChainOverrideRequest struct {
	Reason string `json:"reason"`
}

// handleSupplyChainOverride handles POST /api/v1/apps/{name}/supply-chain/override.
func (rt *Router) handleSupplyChainOverride(w http.ResponseWriter, r *http.Request) {
	if !rt.supplyChainReady(w, r) {
		return
	}
	var req supplyChainOverrideRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, supplyChainBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	app := r.PathValue("name")
	st, err := rt.supplyChain.ArmOverride(r.Context(), app, req.Reason)
	if err != nil {
		rt.writeSupplyChainError(w, err, "api: arm scan gate override failed", app)
		return
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: app, Kind: store.AppEventScanGateOverride, Title: "Scan gate override armed", Detail: st.OverrideReason})
	rt.logger.Warn("api: scan gate override armed", slog.String("name", app), slog.String("reason", st.OverrideReason), slog.String("remote_addr", clientIP(r)))
	rt.writeSupplyChainSettings(w, r, http.StatusOK)
}

func (rt *Router) writeSupplyChainError(w http.ResponseWriter, err error, msg, app string, attrs ...slog.Attr) {
	switch {
	case errors.Is(err, supplychain.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, supplychain.ErrNotFound), errors.Is(err, supplychain.ErrNoSBOM):
		writeError(w, http.StatusNotFound, "no SBOM for this deployment")
	case errors.Is(err, supplychain.ErrDisabled), errors.Is(err, supplychain.ErrNotEnabled):
		writeError(w, http.StatusConflict, err.Error())
	default:
		rt.internalError(w, msg, err, append([]slog.Attr{slog.String("name", app)}, attrs...)...)
	}
}

// handleGetSBOM handles GET /api/v1/apps/{name}/deployments/{id}/sbom.
func (rt *Router) handleGetSBOM(w http.ResponseWriter, r *http.Request) {
	if !rt.supplyChainReady(w, r) {
		return
	}
	app, id := r.PathValue("name"), r.PathValue("id")
	if r.URL.Query().Get("download") == "true" {
		data, _, err := rt.supplyChain.OpenSBOM(r.Context(), app, id)
		if err != nil {
			rt.writeSupplyChainError(w, err, "api: open sbom failed", app, slog.String("deployment_id", id))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.sbom.json"`)
		w.Header().Set("Cache-Control", "private, max-age=3600")
		_, _ = w.Write(data)
		return
	}
	rec, err := rt.supplyChain.Get(r.Context(), app, id)
	if err != nil {
		rt.writeSupplyChainError(w, err, "api: get sbom failed", app, slog.String("deployment_id", id))
		return
	}
	writeJSON(w, http.StatusOK, toSBOMResource(rec))
}

// handleGetVulnerabilities handles GET /api/v1/apps/{name}/deployments/{id}/vulnerabilities.
func (rt *Router) handleGetVulnerabilities(w http.ResponseWriter, r *http.Request) {
	if !rt.supplyChainReady(w, r) {
		return
	}
	app, id := r.PathValue("name"), r.PathValue("id")
	rec, err := rt.supplyChain.Get(r.Context(), app, id)
	if err != nil {
		rt.writeSupplyChainError(w, err, "api: get vulnerabilities failed", app, slog.String("deployment_id", id))
		return
	}
	writeJSON(w, http.StatusOK, toVulnResource(rec))
}

// handleScanDeployment handles POST /api/v1/apps/{name}/deployments/{id}/scan.
func (rt *Router) handleScanDeployment(w http.ResponseWriter, r *http.Request) {
	if !rt.supplyChainReady(w, r) {
		return
	}
	app, id := r.PathValue("name"), r.PathValue("id")
	rec, err := rt.supplyChain.ScanNow(r.Context(), app, id)
	if err != nil {
		if rec.AttemptID != "" {
			writeJSON(w, http.StatusBadGateway, toVulnResource(rec))
			return
		}
		rt.writeSupplyChainError(w, err, "api: scan deployment failed", app, slog.String("deployment_id", id))
		return
	}
	writeJSON(w, http.StatusOK, toVulnResource(rec))
}
