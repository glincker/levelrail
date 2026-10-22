package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/branchdeploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxBranchDeployPatterns   = 50
	maxBranchDeployPatternLen = 200
)

type branchDeploySettingsResource struct {
	Mode             string   `json:"mode"`
	Patterns         []string `json:"patterns"`
	ProductionBranch string   `json:"production_branch"`
	IdleTTLHours     int      `json:"idle_ttl_hours"`
}

type branchDeployResource struct {
	Branch       string `json:"branch"`
	Slug         string `json:"slug"`
	DeployAppID  string `json:"deploy_app_id"`
	URL          string `json:"url,omitempty"`
	Domain       string `json:"domain,omitempty"`
	HeadSHA      string `json:"head_sha"`
	HeadMessage  string `json:"head_message,omitempty"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

type branchDeploysResponse struct {
	Settings branchDeploySettingsResource `json:"settings"`
	Deploys  []branchDeployResource       `json:"deploys"`
}

type setBranchDeploySettingsRequest struct {
	Mode     string   `json:"mode"`
	Patterns []string `json:"patterns"`
}

func (rt *Router) branchDeploysResponseFor(r *http.Request, name string) (branchDeploysResponse, int, string) {
	ctx := r.Context()
	cfg, err := rt.branchDeploys.GetBranchDeploySettings(ctx, name)
	if err != nil {
		rt.logger.Error("api: branch deploys: load settings failed", slog.String("error", err.Error()), slog.String("name", name))
		return branchDeploysResponse{}, http.StatusInternalServerError, "internal error"
	}
	rows, err := rt.branchDeploys.ListBranchDeploysByApp(ctx, name)
	if err != nil {
		rt.logger.Error("api: branch deploys: list failed", slog.String("error", err.Error()), slog.String("name", name))
		return branchDeploysResponse{}, http.StatusInternalServerError, "internal error"
	}
	prod := ""
	if gs, gerr := rt.gitSources.GetGitSource(ctx, name); gerr == nil && gs != nil {
		prod = productionBranch(*gs)
	}
	ttl := branchDeployTTL()
	out := branchDeploysResponse{
		Settings: branchDeploySettingsResource{Mode: cfg.Mode, Patterns: cfg.Patterns, ProductionBranch: prod, IdleTTLHours: int(ttl / time.Hour)},
		Deploys:  make([]branchDeployResource, 0, len(rows)),
	}
	for _, b := range rows {
		res := branchDeployResource{
			Branch: b.Branch, Slug: b.Slug, DeployAppID: b.DeployAppID, Domain: b.Domain, HeadSHA: b.HeadSHA, HeadMessage: b.HeadMessage,
			Status: b.Status, StatusReason: b.StatusReason, CreatedAt: b.CreatedAt, UpdatedAt: b.UpdatedAt,
			URL: rt.previewURLFor(ctx, b.Domain),
		}
		if updated, perr := time.Parse(time.RFC3339Nano, b.UpdatedAt); perr == nil {
			res.ExpiresAt = updated.Add(ttl).UTC().Format(time.RFC3339)
		}
		out.Deploys = append(out.Deploys, res)
	}
	return out, 0, ""
}

// handleListBranchDeploys handles GET /api/v1/apps/{name}/branch-deploys.
func (rt *Router) handleListBranchDeploys(w http.ResponseWriter, r *http.Request) {
	out, status, msg := rt.branchDeploysResponseFor(r, r.PathValue("name"))
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// cleanBranchPatterns trims and validates patterns; the returned message is non-empty on a bad entry.
func cleanBranchPatterns(in []string) ([]string, string) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		if len(p) > maxBranchDeployPatternLen || strings.IndexFunc(p, unicode.IsSpace) >= 0 || strings.IndexFunc(p, unicode.IsControl) >= 0 {
			return nil, "a branch pattern is too long or contains whitespace"
		}
		seen[p] = true
		out = append(out, p)
	}
	if len(out) > maxBranchDeployPatterns {
		return nil, "too many branch patterns"
	}
	return out, ""
}

// handleSetBranchDeploySettings handles PUT /api/v1/apps/{name}/branch-deploys.
func (rt *Router) handleSetBranchDeploySettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()
	var req setBranchDeploySettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !branchdeploy.ValidMode(req.Mode) {
		writeError(w, http.StatusBadRequest, "mode must be off, all or patterns")
		return
	}
	patterns, bad := cleanBranchPatterns(req.Patterns)
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad)
		return
	}
	if req.Mode == branchdeploy.ModePatterns && len(patterns) == 0 {
		writeError(w, http.StatusBadRequest, "patterns mode needs at least one branch or pattern")
		return
	}
	if _, err := rt.gitSources.GetGitSource(ctx, name); errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	} else if err != nil {
		rt.internalError(w, "api: branch deploys: load git source failed", err, slog.String("name", name))
		return
	}
	cfg := store.BranchDeploySettings{AppName: name, Mode: req.Mode, Patterns: patterns, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := rt.branchDeploys.SaveBranchDeploySettings(ctx, cfg); err != nil {
		rt.internalError(w, "api: branch deploys: save settings failed", err, slog.String("name", name))
		return
	}
	out, status, msg := rt.branchDeploysResponseFor(r, name)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteBranchDeploy handles DELETE /api/v1/apps/{name}/branch-deploys/{slug}.
func (rt *Router) handleDeleteBranchDeploy(w http.ResponseWriter, r *http.Request) {
	name, slug := r.PathValue("name"), r.PathValue("slug")
	row, err := rt.branchDeploys.GetBranchDeployBySlug(r.Context(), name, slug)
	if errors.Is(err, store.ErrBranchDeployNotFound) {
		writeError(w, http.StatusNotFound, "branch deploy not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: branch deploys: load for delete failed", err, slog.String("name", name))
		return
	}
	status, msg := rt.teardownBranchDeploy(r.Context(), *row)
	if status >= http.StatusInternalServerError {
		writeError(w, status, "internal error")
		return
	}
	if status == http.StatusMultiStatus {
		writeJSON(w, status, map[string]string{"message": strings.TrimSpace(msg)})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
