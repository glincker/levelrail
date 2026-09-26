package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

const maxPreviewTTLHours = 24 * 365

type previewLimitsResource struct {
	MaxPerApp int `json:"max_per_app"`
	MaxTotal  int `json:"max_total"`
	LiveTotal int `json:"live_total"`
}

type previewsOverviewResource struct {
	Limits   previewLimitsResource        `json:"limits"`
	PerApp   map[string]int               `json:"live_per_app"`
	Previews []previewEnvironmentResource `json:"previews"`
}

func countLivePreviews(all []store.PreviewEnvironment) (total int, perApp map[string]int) {
	perApp = map[string]int{}
	for _, p := range all {
		if p.Occupies() {
			total++
			perApp[p.AppName]++
		}
	}
	return total, perApp
}

// handleListAllPreviews handles GET /api/v1/previews: every preview of the
// apps the caller can read, plus limit usage across the platform.
func (rt *Router) handleListAllPreviews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	canRead, filtered, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: list previews: resolve caller visibility", err)
		return
	}
	all, err := rt.previewEnvironments.ListPreviewEnvironments(ctx)
	if err != nil {
		rt.internalError(w, "api: list previews", err)
		return
	}
	settings, err := rt.previewEnvironments.ListPreviewAppSettings(ctx)
	if err != nil {
		rt.internalError(w, "api: list previews: settings", err)
		return
	}

	liveTotal, livePerApp := countLivePreviews(all)
	out := previewsOverviewResource{
		Limits:   previewLimitsResource{MaxPerApp: rt.previewLimits.MaxPerApp, MaxTotal: rt.previewLimits.MaxTotal, LiveTotal: liveTotal},
		PerApp:   map[string]int{},
		Previews: []previewEnvironmentResource{},
	}
	for app, n := range livePerApp {
		if !filtered || canRead(app) {
			out.PerApp[app] = n
		}
	}
	for i := len(all) - 1; i >= 0; i-- {
		p := all[i]
		if filtered && !canRead(p.AppName) {
			continue
		}
		out.Previews = append(out.Previews, rt.toPreviewEnvironmentResource(ctx, p, rt.previewTTLFor(settings[p.AppName])))
	}
	writeJSON(w, http.StatusOK, out)
}

type previewPolicyResource struct {
	OnLimit           string `json:"on_limit"`
	AllowForkPreviews bool   `json:"allow_fork_previews"`
	TTLHours          int    `json:"ttl_hours"`
	EffectiveTTLHours int    `json:"effective_ttl_hours"`
	MaxPerApp         int    `json:"max_per_app"`
	LiveCount         int    `json:"live_count"`
	MaxTotal          int    `json:"max_total"`
	LiveTotal         int    `json:"live_total"`
}

func (rt *Router) previewPolicyFor(ctx context.Context, appName string) (previewPolicyResource, error) {
	s := rt.previewSettings(ctx, appName)
	all, err := rt.previewEnvironments.ListPreviewEnvironments(ctx)
	if err != nil {
		return previewPolicyResource{}, err
	}
	liveTotal, perApp := countLivePreviews(all)
	return previewPolicyResource{
		OnLimit: s.OnLimit, AllowForkPreviews: s.AllowForkPreviews, TTLHours: s.TTLHours,
		EffectiveTTLHours: int(rt.previewTTLFor(s) / time.Hour),
		MaxPerApp:         rt.previewLimits.MaxPerApp, LiveCount: perApp[appName],
		MaxTotal: rt.previewLimits.MaxTotal, LiveTotal: liveTotal,
	}, nil
}

// handleGetPreviewPolicy handles GET /api/v1/apps/{name}/preview-policy.
func (rt *Router) handleGetPreviewPolicy(w http.ResponseWriter, r *http.Request) {
	res, err := rt.previewPolicyFor(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.internalError(w, "api: get preview policy", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type setPreviewPolicyRequest struct {
	OnLimit           *string `json:"on_limit,omitempty"`
	AllowForkPreviews *bool   `json:"allow_fork_previews,omitempty"`
	TTLHours          *int    `json:"ttl_hours,omitempty"`
}

// handleSetPreviewPolicy handles PUT /api/v1/apps/{name}/preview-policy.
// Omitted fields keep their stored value.
func (rt *Router) handleSetPreviewPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	var req setPreviewPolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.OnLimit == nil && req.AllowForkPreviews == nil && req.TTLHours == nil {
		writeError(w, http.StatusBadRequest, "on_limit, allow_fork_previews or ttl_hours is required")
		return
	}
	if _, err := rt.gitSources.GetGitSource(ctx, name); errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	} else if err != nil {
		rt.internalError(w, "api: set preview policy: load git source", err)
		return
	}

	s := rt.previewSettings(ctx, name)
	if req.OnLimit != nil {
		if *req.OnLimit != store.PreviewOnLimitEvictOldest && *req.OnLimit != store.PreviewOnLimitReject {
			writeError(w, http.StatusBadRequest, "on_limit must be evict_oldest or reject")
			return
		}
		s.OnLimit = *req.OnLimit
	}
	if req.AllowForkPreviews != nil {
		s.AllowForkPreviews = *req.AllowForkPreviews
	}
	if req.TTLHours != nil {
		if *req.TTLHours < 0 || *req.TTLHours > maxPreviewTTLHours {
			writeError(w, http.StatusBadRequest, "ttl_hours must be between 0 and "+strconv.Itoa(maxPreviewTTLHours)+" (0 uses the platform default)")
			return
		}
		s.TTLHours = *req.TTLHours
	}
	if err := rt.previewEnvironments.SavePreviewAppSettings(ctx, s); err != nil {
		rt.internalError(w, "api: set preview policy: save", err)
		return
	}
	rt.logger.Info("api: preview policy updated", slog.String("name", name), slog.String("on_limit", s.OnLimit),
		slog.Bool("allow_fork_previews", s.AllowForkPreviews), slog.Int("ttl_hours", s.TTLHours))

	res, err := rt.previewPolicyFor(ctx, name)
	if err != nil {
		rt.internalError(w, "api: set preview policy: reload", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type approvePreviewRequest struct {
	Confirm bool `json:"confirm"`
}

type approvePreviewResponse struct {
	Status string `json:"status"`
}

// handleApprovePreviewEnvironment handles POST
// /api/v1/apps/{name}/previews/{number}/approve: deploys a held fork pull
// request once. It requires an explicit confirm because the deployed code
// runs with the app's environment variables and secrets.
func (rt *Router) handleApprovePreviewEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()
	prNumber, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "pr number must be an integer")
		return
	}
	var req approvePreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !req.Confirm {
		writeError(w, http.StatusBadRequest, "confirm must be true: approving runs the fork's code with this app's environment variables and secrets")
		return
	}
	if rt.builder == nil {
		writeError(w, http.StatusNotImplemented, "git push deploys are not configured on this control plane")
		return
	}

	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: approve preview: load git source", err)
		return
	}

	preview, status, msg := rt.claimApprovedPreview(ctx, name, prNumber)
	if preview == nil {
		writeError(w, status, msg)
		return
	}

	baseRef := gs.Branch
	if baseRef == "" {
		baseRef = webhook.DefaultBranch
	}
	ev := webhook.PullRequestEvent{
		Action: webhook.PullRequestSynchronize, Number: prNumber, HeadRef: preview.Branch, HeadSHA: preview.HeadSHA,
		BaseRef: baseRef, HeadRepoFullName: preview.HeadRepo,
	}
	rt.previewDeploys.Add(1)
	go func() { //nolint:gosec // deliberately outlives the request: a build can take minutes, same as the webhook path
		defer rt.previewDeploys.Done()
		rt.deployPreviewEnvironment(context.WithoutCancel(ctx), name, *gs, ev, true)
	}()
	writeJSON(w, http.StatusAccepted, approvePreviewResponse{Status: store.PreviewStatusDeploying})
}

// claimApprovedPreview moves a held preview to deploying under the admission
// lock, so a double click cannot start two deploys and the cap still applies.
func (rt *Router) claimApprovedPreview(ctx context.Context, name string, prNumber int) (*store.PreviewEnvironment, int, string) {
	rt.previewAdmitMu.Lock()
	defer rt.previewAdmitMu.Unlock()

	preview, err := rt.previewEnvironments.GetPreviewEnvironmentByAppAndPR(ctx, name, prNumber)
	if errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
		return nil, http.StatusNotFound, "no preview environment found for this pull request"
	}
	if err != nil {
		rt.logger.Error("api: approve preview: load failed", slog.String("error", err.Error()), slog.String("name", name))
		return nil, http.StatusInternalServerError, "internal error"
	}
	if preview.Status != store.PreviewStatusAwaitingApproval {
		return nil, http.StatusConflict, "this preview is not waiting for approval"
	}
	reason, err := rt.admitPreview(ctx, name, preview, rt.previewSettings(ctx, name))
	if err != nil {
		rt.logger.Error("api: approve preview: limit check failed", slog.String("error", err.Error()), slog.String("name", name))
		return nil, http.StatusInternalServerError, "internal error"
	}
	if reason != "" {
		return nil, http.StatusConflict, reason
	}
	preview.Status, preview.StatusReason = store.PreviewStatusDeploying, ""
	preview.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := rt.previewEnvironments.UpdatePreviewEnvironment(ctx, *preview); err != nil {
		rt.logger.Error("api: approve preview: mark deploying failed", slog.String("error", err.Error()), slog.String("name", name))
		return nil, http.StatusInternalServerError, "internal error"
	}
	return preview, 0, ""
}
