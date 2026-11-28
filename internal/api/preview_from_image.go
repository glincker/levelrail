package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/diskguard"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/webhook"
)

const maxImagePreviewPRNumber = 1 << 24

var (
	previewBranchPattern = regexp.MustCompile(`^[A-Za-z0-9._/-]{0,200}$`)
	previewSHAPattern    = regexp.MustCompile(`^[0-9a-fA-F]{0,64}$`)
)

// isImagePreview marks a preview made from a CI image: its head repo is the app name, which no
// "owner/name" webhook repo can equal.
func isImagePreview(p store.PreviewEnvironment) bool {
	return p.HeadRepo != "" && p.HeadRepo == p.AppName
}

// removeImagePreviewService undoes a deploy whose row vanished. An app group this run did not
// create is left alone and only the preview's own service goes.
func (rt *Router) removeImagePreviewService(ctx context.Context, previewName string, appExisted bool) {
	if !appExisted {
		rt.teardownPreviewApp(ctx, previewName)
		return
	}
	svc, err := rt.apps.GetDesiredService(ctx, previewName)
	if err != nil {
		return
	}
	if err := rt.apps.DeleteDesiredService(ctx, previewName); err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		rt.logger.Error("api: create preview from image: remove service failed", slog.String("error", err.Error()), slog.String("service", previewName))
		return
	}
	rt.teardownServiceContainers(previewName, svc.NodeID)
}

type previewNoEvictKey struct{}

// previewNoEvict is true for an image preview, whose caller-chosen PR number must never evict a real preview.
func previewNoEvict(ctx context.Context) bool {
	v, _ := ctx.Value(previewNoEvictKey{}).(bool)
	return v
}

type createPreviewRequest struct {
	PRNumber int    `json:"pr_number"`
	Image    string `json:"image"`
	Branch   string `json:"branch,omitempty"`
	SHA      string `json:"sha,omitempty"`
}

// handleCreatePreviewFromImage handles POST /api/v1/apps/{name}/previews: a
// preview of a CI-built image, with no source fetch or build on this host.
func (rt *Router) handleCreatePreviewFromImage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	req, ok := decodeJSON[createPreviewRequest](w, r, 1<<16)
	if !ok {
		return
	}
	req.Image = strings.TrimSpace(req.Image)
	if req.PRNumber <= 0 || req.PRNumber > maxImagePreviewPRNumber {
		writeError(w, http.StatusBadRequest, "pr_number must be a positive integer")
		return
	}
	if !previewBranchPattern.MatchString(req.Branch) || !previewSHAPattern.MatchString(req.SHA) {
		writeError(w, http.StatusBadRequest, "branch or sha has invalid characters")
		return
	}
	if req.Image == "" {
		writeError(w, http.StatusBadRequest, "image is required")
		return
	}
	if err := validateImageReference(req.Image); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if rt.builder == nil {
		writeError(w, http.StatusNotImplemented, "deploys are not configured on this control plane")
		return
	}

	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: create preview from image: load git source failed", err, slog.String("name", name))
		return
	}
	if !gs.PreviewEnabled {
		writeError(w, http.StatusConflict, "preview environments are not enabled for this app")
		return
	}
	if len(gs.Services) > 0 || isStaticPreviewSource(*gs) {
		writeError(w, http.StatusConflict, "image previews support single-service apps only")
		return
	}

	if len(gs.Databases) > 0 {
		writeError(w, http.StatusConflict, "image previews do not support apps with databases")
		return
	}
	if s := rt.previewSettings(ctx, name); s.DatabaseStrategy == store.PreviewDatabaseStrategyFresh || s.DatabaseStrategy == store.PreviewDatabaseStrategySeed {
		writeError(w, http.StatusConflict, "image previews do not support the fresh or seed database strategy")
		return
	}

	// The same gates a plain image deploy of the parent runs, since the preview inherits its env.
	parent, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		rt.internalError(w, "api: create preview from image: load app failed", err, slog.String("name", name))
		return
	}
	// Recorded against the preview's own name so a refusal does not land on the production app.
	guarded := *parent
	guarded.Name = previewAppName(name, req.PRNumber)
	if rt.refuseLowDisk(w, diskguard.OpPull) || rt.refuseNodeCritical(ctx, w, guarded, req.Image, store.DeployAttemptSourceImage) {
		return
	}
	if _, ok := rt.freezeGate(w, r, name, freezeOverride{}); !ok {
		return
	}
	candidate := *parent
	candidate.Image = req.Image
	if _, ok := rt.policyGate(w, r, candidate, policyOverride{}); !ok {
		return
	}
	if env, protected, ok := rt.checkEnvironmentProtection(ctx, w, parent.EnvironmentID, false); !ok {
		return
	} else if protected {
		writeError(w, http.StatusConflict, fmt.Sprintf("environment %q is protected: image previews are not allowed, deploy through an approval", env.Name))
		return
	}

	// Detached from the request so a client disconnect cannot strand a row in deploying.
	rt.previewDeploys.Add(1)
	defer rt.previewDeploys.Done()
	preview, status, msg := rt.deployPreviewFromImage(context.WithValue(context.WithoutCancel(ctx), previewNoEvictKey{}, true), name, *gs, req)
	if preview == nil {
		if status < http.StatusBadRequest {
			status = http.StatusConflict
		}
		writeError(w, status, strings.TrimSpace(msg))
		return
	}
	writeJSON(w, http.StatusCreated, rt.toPreviewEnvironmentResource(ctx, *preview, rt.previewTTLFor(rt.previewSettings(ctx, name))))
}

func (rt *Router) deployPreviewFromImage(ctx context.Context, appName string, gs store.GitSource, req createPreviewRequest) (*store.PreviewEnvironment, int, string) {
	// The caller holds the deploy ability and supplies the image, so it is not treated as a fork.
	ev := webhook.PullRequestEvent{Number: req.PRNumber, HeadRef: req.Branch, HeadSHA: req.SHA, HeadRepoFullName: appName, BaseRepoFullName: appName}
	previewName := previewAppName(appName, req.PRNumber)
	defer rt.previewDeployLocks.lock(previewName)()

	// Checked under the same lock the webhook path holds, so a fork hold cannot be erased in between.
	existing, err := rt.previewEnvironments.GetPreviewEnvironmentByAppAndPR(ctx, appName, req.PRNumber)
	if err != nil && !errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
		rt.logger.Error("api: create preview from image: load preview failed", slog.String("error", err.Error()), slog.String("app_name", appName))
		return nil, http.StatusInternalServerError, "internal error"
	}
	if err == nil && (existing.IsFork || existing.Status == store.PreviewStatusAwaitingApproval) {
		return nil, http.StatusConflict, "this pull request preview is a fork awaiting approval"
	}
	if err == nil && !isImagePreview(*existing) {
		// Whatever its status, so a webhook row and its comment id are never taken over.
		return nil, http.StatusConflict, "a pull request preview from the git host already uses this number"
	}
	_, appErr := rt.appGroups.GetAppByName(ctx, previewName)
	appExisted := appErr == nil
	if err != nil || !existing.Occupies() {
		// A number must not land on an app that already has that name.
		if _, getErr := rt.apps.GetDesiredService(ctx, previewName); getErr == nil {
			return nil, http.StatusConflict, fmt.Sprintf("%q is already an app", previewName)
		} else if !errors.Is(getErr, store.ErrServiceNotFound) {
			rt.logger.Error("api: create preview from image: name check failed", slog.String("error", getErr.Error()), slog.String("app_name", appName))
			return nil, http.StatusInternalServerError, "internal error"
		}
	}

	preview, done, status, msg := rt.beginPreview(ctx, appName, gs, ev, false)
	if done {
		// A held row (limit reached) is not a created preview.
		return nil, status, msg
	}

	policy := rt.previewSettings(ctx, appName)
	// A literal image has no placeholders, so the tag template yields it unchanged.
	policy.ImageTagTemplate = req.Image
	// The branch is caller-supplied and unverified, so it never selects branch env overrides or the host.
	vars := previewEnvVars{PRNumber: req.PRNumber, Policy: policy}
	wantDomain := rt.previewDomainForBranch(ctx, appName, req.PRNumber, "")

	usedDomain, domainConflict, err := rt.deployPreviewSingle(ctx, appName, previewName, gs, "", req.SHA, wantDomain, vars)
	if err != nil {
		rt.logger.Error("api: create preview from image: deploy failed", slog.String("error", err.Error()), slog.String("app_name", appName), slog.Int("pr_number", req.PRNumber))
		// No git host notification: the SHA and pull request are unverified and the error text can name registries.
		preview.Status, preview.StatusReason = store.PreviewStatusFailed, "image deploy failed"
		preview.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if updErr := rt.previewEnvironments.UpdatePreviewEnvironment(ctx, *preview); updErr != nil {
			rt.logger.Error("api: create preview from image: mark failed failed", slog.String("error", updErr.Error()), slog.String("preview_id", preview.ID))
		}
		return nil, http.StatusInternalServerError, "deploy failed"
	}

	if _, environmentID, envErr := rt.ensurePreviewEnvironmentTier(ctx, appName); envErr != nil {
		rt.logger.Error("api: create preview from image: ensure environment tier failed", slog.String("error", envErr.Error()), slog.String("app_name", appName))
	} else if tagErr := rt.tagPreviewWithEnvironment(ctx, previewName, gs, environmentID); tagErr != nil {
		rt.logger.Error("api: create preview from image: tag environment failed", slog.String("error", tagErr.Error()), slog.String("app_name", appName))
	} else {
		preview.EnvironmentID = environmentID
	}

	notes := rt.finishPreviewExposure(ctx, appName, previewName, usedDomain, policy)
	preview.Status, preview.Domain, preview.StatusReason = store.PreviewStatusActive, usedDomain, strings.Join(notes, "; ")
	if domainConflict {
		preview.StatusReason = fmt.Sprintf("domain %q was already in use, preview deployed without a domain", wantDomain)
	}
	preview.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := rt.previewEnvironments.UpdatePreviewEnvironment(ctx, *preview); err != nil {
		rt.logger.Error("api: create preview from image: finalize failed", slog.String("error", err.Error()), slog.String("app_name", appName))
		if errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
			// The row was torn down meanwhile: do not leave a container with the parent's env behind.
			rt.removeImagePreviewService(ctx, previewName, appExisted)
			return nil, http.StatusConflict, "the preview was removed while it deployed"
		}
		return nil, http.StatusInternalServerError, "internal error"
	}
	return preview, http.StatusCreated, ""
}
