package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// previewSiteMarker is what previewAppName puts between the app and the PR
// number; teardown only removes site names that carry it.
const previewSiteMarker = "-pr-"

// isStaticPreviewSource reports whether previews of gs publish files instead of
// starting a container: a single-service source whose build type is static.
func isStaticPreviewSource(gs store.GitSource) bool {
	if len(gs.Services) > 0 {
		return false
	}
	b, err := gs.SpecBuild()
	return err == nil && b.Type == spec.BuildStatic
}

// deployPreviewStatic publishes the PR's head commit as its own static site
// under previewName, so the live site, its route and its release history are
// never touched. A taken domain retries once without one, like the container
// path.
func (rt *Router) deployPreviewStatic(ctx context.Context, appName, previewName string, gs store.GitSource, sourceDir, headSHA, wantDomain string) (usedDomain string, domainConflict bool, err error) {
	prod, err := rt.apps.GetDesiredService(ctx, appName)
	if err != nil {
		return "", false, fmt.Errorf("load production service %q: %w", appName, err)
	}
	gitBuild, err := gs.SpecBuild()
	if err != nil {
		return "", false, fmt.Errorf("resolve build settings for %q: %w", appName, err)
	}
	svcSpec := specServiceFromDesired(*prod, gitBuild)
	// A static build reads build args only, so app env never needs to reach it.
	svcSpec.Port, svcSpec.Env = 0, nil
	svcSpec.Domains = domainSlice(wantDomain)
	req := deploy.Request{ServiceName: previewName, Service: svcSpec, SourceDir: sourceDir, CommitSHA: headSHA, ImageRepo: previewName, Preview: true}

	_, deployErr := rt.builder.Deploy(ctx, req, build.SlogProgress(rt.logger))
	usedDomain = wantDomain
	var domainTaken *store.ErrDomainTaken
	if deployErr != nil && wantDomain != "" && errors.As(deployErr, &domainTaken) {
		req.Service.Domains = nil
		if _, retryErr := rt.builder.Deploy(ctx, req, build.SlogProgress(rt.logger)); retryErr != nil {
			return "", false, fmt.Errorf("deploy without domain after domain conflict: %w", retryErr)
		}
		usedDomain, domainConflict, deployErr = "", true, nil
	}
	if deployErr != nil {
		return "", false, deployErr
	}
	rt.nudgeReconciler()
	return usedDomain, domainConflict, nil
}

// finishStaticPreviewExposure is finishPreviewExposure for a static preview:
// the DNS record and search hiding, without idle sleep (there is no container).
// The basic auth gate is not applied by the static route, so it is reported.
func (rt *Router) finishStaticPreviewExposure(ctx context.Context, appName, previewName, domain string, s store.PreviewAppSettings) []string {
	var notes []string
	if domain != "" {
		if res := rt.previewDNS(ctx, previewName, domain); res.DNS == dnsResultError {
			rt.logger.Warn("api: preview dns record failed", slog.String("domain", domain), slog.String("message", res.Message))
			notes = append(notes, "the DNS record could not be created: "+res.Message)
		}
	}
	gateRequested := s.GateBasicAuth
	s.GateBasicAuth = false
	notes = append(notes, rt.secureReachablePreview(ctx, appName, domain, s)...)
	if gateRequested && domain != "" {
		notes = append(notes, "the basic auth gate is not enforced on static previews")
	}
	return notes
}

// teardownPreviewStatic removes a preview's static site row, route and files.
// It is a no-op for a preview that published none, and never matches a site
// whose name is not a preview name.
func (rt *Router) teardownPreviewStatic(ctx context.Context, previewName string) []string {
	if !strings.Contains(previewName, previewSiteMarker) {
		return nil
	}
	if err := rt.removeStaticSite(ctx, previewName); err != nil {
		rt.logger.Error("api: teardown preview static site failed", slog.String("error", err.Error()), slog.String("preview_app_id", previewName))
		return []string{"static:" + previewName}
	}
	rt.nudgeReconciler()
	return nil
}

// staticPreviewNames lists the site names that belong to a live preview row,
// so the orphan sweep never removes a preview that has no service row.
func (rt *Router) staticPreviewNames(ctx context.Context) (map[string]bool, error) {
	out := map[string]bool{}
	if rt.previewEnvironments == nil {
		return out, nil
	}
	rows, err := rt.previewEnvironments.ListPreviewEnvironments(ctx)
	if err != nil {
		return nil, fmt.Errorf("list preview environments: %w", err)
	}
	for _, p := range rows {
		out[p.PreviewAppID] = true
	}
	return out, nil
}
