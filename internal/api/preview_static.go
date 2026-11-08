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

type domainOwnerLookup interface {
	DomainOwnerOf(ctx context.Context, domain string) (owner string, found bool, err error)
}

// preGateStaticPreview writes the basic auth gate before the site exists, so its route is never served ungated.
// It returns whether the gate row is new (and so safe to remove on failure); a domain owned by someone else is left alone.
func (rt *Router) preGateStaticPreview(ctx context.Context, appName, previewName, domain string, s store.PreviewAppSettings) (created bool, err error) {
	if !s.GateBasicAuth || domain == "" {
		return false, nil
	}
	lookup, ok := rt.apps.(domainOwnerLookup)
	if !ok {
		return false, nil
	}
	owner, found, err := lookup.DomainOwnerOf(ctx, domain)
	if err != nil {
		return false, fmt.Errorf("check owner of %q: %w", domain, err)
	}
	if found && owner != previewName {
		return false, nil
	}
	if err := rt.gatePreviewDomain(ctx, appName, domain, store.PreviewAppSettings{GateUsername: s.GateUsername}); err != nil {
		if !found {
			rt.removePreviewGates(ctx, []string{domain})
		}
		return false, fmt.Errorf("apply basic auth gate: %w", err)
	}
	return !found, nil
}

// deployPreviewStatic publishes the PR's head commit as its own static site
// under previewName, so the live site, its route and its release history are
// never touched. A taken domain retries once without one, like the container
// path.
func (rt *Router) deployPreviewStatic(ctx context.Context, appName, previewName string, gs store.GitSource, sourceDir, headSHA, wantDomain string, policy store.PreviewAppSettings) (usedDomain string, domainConflict bool, err error) {
	prod, err := rt.apps.GetDesiredService(ctx, appName)
	if err != nil {
		return "", false, fmt.Errorf("load production service %q: %w", appName, err)
	}
	gitBuild, err := gs.SpecBuild()
	if err != nil {
		return "", false, fmt.Errorf("resolve build settings for %q: %w", appName, err)
	}
	svcSpec := specServiceFromDesired(*prod, gitBuild)
	svcSpec.Static = gs.StaticOptions()
	// A static build reads build args only, so app env never needs to reach it.
	svcSpec.Port, svcSpec.Env = 0, nil
	svcSpec.Domains = domainSlice(wantDomain)
	req := deploy.Request{ServiceName: previewName, Service: svcSpec, SourceDir: sourceDir, CommitSHA: headSHA, ImageRepo: previewName, Preview: true}

	newGate, err := rt.preGateStaticPreview(ctx, appName, previewName, wantDomain, policy)
	if err != nil {
		return "", false, err
	}
	_, deployErr := rt.builder.Deploy(ctx, req, build.SlogProgress(rt.logger))
	if deployErr != nil && newGate {
		rt.removePreviewGates(ctx, []string{wantDomain})
	}
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
// the DNS record, search hiding and the basic auth gate, without idle sleep
// (there is no container). A requested gate that cannot be applied, or a
// forward-auth gate the static route cannot enforce, is an error so the
// caller does not publish an unprotected preview.
func (rt *Router) finishStaticPreviewExposure(ctx context.Context, appName, previewName, domain string, s store.PreviewAppSettings) ([]string, error) {
	var notes []string
	if domain != "" {
		if res := rt.previewDNS(ctx, previewName, domain); res.DNS == dnsResultError {
			rt.logger.Warn("api: preview dns record failed", slog.String("domain", domain), slog.String("message", res.Message))
			notes = append(notes, "the DNS record could not be created: "+res.Message)
		}
		if g, found, err := rt.domainAuthGate.GetDomainAuthGate(ctx, domain); err != nil {
			return notes, fmt.Errorf("check forward-auth gate for %q: %w", domain, err)
		} else if found && g.Enabled {
			return notes, fmt.Errorf("forward-auth gates are not enforced on static previews, remove the gate on %q", domain)
		}
	}
	gateRequested := s.GateBasicAuth && domain != ""
	s.GateBasicAuth = false
	notes = append(notes, rt.secureReachablePreview(ctx, appName, domain, s)...)
	if gateRequested {
		if err := rt.gatePreviewDomain(ctx, appName, domain, store.PreviewAppSettings{GateUsername: s.GateUsername}); err != nil {
			return notes, fmt.Errorf("apply basic auth gate: %w", err)
		}
	}
	return notes, nil
}

// teardownPreviewStatic removes a preview's static site row, route and files.
// It is a no-op for a preview that published none, and never matches a site
// whose name is not a preview name.
func (rt *Router) teardownPreviewStatic(ctx context.Context, previewName string) []string {
	if !strings.Contains(previewName, previewSiteMarker) {
		return nil
	}
	// Domains are read first: the site row (and its domain claims) is gone after removal.
	var domains []string
	if g, ok := rt.staticSites.(staticSiteGetter); ok {
		if site, err := g.GetStaticSite(ctx, previewName); err == nil {
			domains = site.Domains
		}
	}
	if err := rt.removeStaticSite(ctx, previewName); err != nil {
		rt.logger.Error("api: teardown preview static site failed", slog.String("error", err.Error()), slog.String("preview_app_id", previewName))
		return []string{"static:" + previewName}
	}
	failed := rt.removePreviewGates(ctx, domains)
	rt.nudgeReconciler()
	return failed
}

// removePreviewGates deletes the basic auth row and stored password of each
// released preview domain so a later owner of the host starts ungated.
func (rt *Router) removePreviewGates(ctx context.Context, domains []string) []string {
	var failed []string
	for _, d := range domains {
		if rt.domainBasicAuth != nil {
			if err := rt.domainBasicAuth.DeleteDomainBasicAuth(ctx, d); err != nil {
				rt.logger.Error("api: teardown preview gate row failed", slog.String("error", err.Error()), slog.String("domain", d))
				failed = append(failed, "gate:"+d)
			}
		}
		if rt.domainBasicAuthSecrets != nil {
			if err := rt.domainBasicAuthSecrets.DeleteAll(ctx, store.DomainBasicAuthSecretsKey(d)); err != nil {
				rt.logger.Error("api: teardown preview gate secret failed", slog.String("error", err.Error()), slog.String("domain", d))
				failed = append(failed, "gate-secret:"+d)
			}
		}
	}
	return failed
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
