package api

import (
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/advisoryfeed"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// envAdvisoryRefresh is the server switch for the refresh action. It stays off
// until an operator opts in, because the action redeploys an app.
const envAdvisoryRefresh = "APP_ADVISORY_REFRESH_ACTION"

type advisoryRefreshRequest struct {
	// Confirm is required for a protected environment, like any deploy.
	Confirm bool `json:"confirm,omitempty"`
	freezeOverride
	policyOverride
}

// handleAdvisoryRefresh handles POST /api/v1/apps/{name}/advisory-refresh. It
// pins the newest registry digest of the app's tag and sends it through the
// normal deploy path, so freeze, policy, approvals and bindings all still apply.
func (rt *Router) handleAdvisoryRefresh(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if strings.ToLower(strings.TrimSpace(os.Getenv(envAdvisoryRefresh))) != "on" {
		writeError(w, http.StatusConflict, "advisory refresh is off; set "+envAdvisoryRefresh+"=on on the server")
		return
	}
	if !advisoryService().Enabled() {
		writeError(w, http.StatusConflict, "no advisory feed is configured")
		return
	}
	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	var req advisoryRefreshRequest
	if !decodeReq(w, r, &req) {
		return
	}
	res, err := rt.advisoryReport(r)
	if err != nil {
		rt.internalError(w, "api: advisory refresh: report failed", err)
		return
	}
	// An unverified feed may report, but it must never trigger a redeploy.
	if res.Signature != advisoryfeed.SigVerified {
		writeError(w, http.StatusConflict, "the advisory feed signature is "+res.Signature+"; refresh needs a verified feed")
		return
	}
	if !appHasAdvisory(res.Matches, name) {
		writeError(w, http.StatusConflict, "app "+name+" matches no known advisory")
		return
	}
	pinned, status, msg := rt.advisoryPinnedImage(r, *existing)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	if rt.verifyPromotionDigest(r.Context(), pinned) != nil {
		writeError(w, http.StatusGone, "image "+pinned+" is no longer in its registry")
		return
	}
	if status, msg := rt.enforceImageSignature(r.Context(), pinned); status != 0 {
		writeError(w, status, msg)
		return
	}
	rt.recordAdvisoryRefreshAudit(r, name)
	rt.applyDeploy(w, r, *existing, deployTriggerRequest{Image: pinned, Confirm: req.Confirm, Pull: true, freezeOverride: req.freezeOverride, policyOverride: req.policyOverride}, "")
}

func appHasAdvisory(ms []advisoryfeed.Match, app string) bool {
	for _, m := range ms {
		if m.App == app {
			return true
		}
	}
	return false
}

// advisoryPinnedImage resolves the newest digest of the app's tag and refuses
// when it is the digest already running.
func (rt *Router) advisoryPinnedImage(r *http.Request, svc store.DesiredService) (string, int, string) {
	if rt.imageResolver == nil {
		return "", http.StatusConflict, "no image resolver is available to look up a newer digest"
	}
	tagRef := docker.UnpinImageRef(svc.Image)
	got, err := rt.imageResolver.ResolveImage(r.Context(), tagRef, rt.imageTrigger(r.Context(), svc, true, "advisory refresh").Auth, true)
	if err != nil || got.Digest == "" {
		return "", http.StatusBadGateway, "could not resolve a registry digest for " + tagRef
	}
	current := docker.ImageDigestOf(svc.Image)
	if current == "" {
		current = rt.servingDigest(r.Context(), svc)
	}
	if strings.EqualFold(current, got.Digest) {
		return "", http.StatusConflict, "the registry has no newer digest of " + tagRef + "; the fix needs a new tag or a rebuild"
	}
	return docker.PinImageRef(tagRef, got.Digest), 0, ""
}

func (rt *Router) recordAdvisoryRefreshAudit(r *http.Request, app string) {
	p, err := rt.bulkPrincipal(r)
	if err != nil {
		rt.logger.Warn("api: advisory refresh audit principal failed", slog.String("error", err.Error()))
		return
	}
	rt.recordBulkAudit(r.Context(), r, p, AbilityDeploy, "advisory-refresh", app)
}
