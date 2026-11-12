package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// handleGetApp handles GET /api/v1/apps/{name}.
func (rt *Router) handleGetApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		rt.writeAppNotFound(r.Context(), w, name)
		return
	}
	if err != nil {
		rt.internalError(w, "api: get app failed", err, slog.String("name", name))
		return
	}
	resource := rt.appOut(r.Context(), *svc)
	tagNames, err := rt.appTagNames(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get app: load tags failed", err, slog.String("name", name))
		return
	}
	resource.Tags = tagNames
	rt.attachSecretPolicy(r.Context(), &resource, *svc)
	pendingImage := ""
	if serving, ok := rt.lastServingImage(r.Context(), *svc); ok {
		pendingImage, resource.Image = resource.Image, serving
	}
	notes := rt.templateNotesFor(r.Context(), *svc)
	writeJSON(w, http.StatusOK, appWithRequests{
		appResource:              resource,
		ManagedByStack:           rt.loadStackOwners(r.Context()).of("App", "App/"+name),
		PendingImage:             pendingImage,
		Requests:                 rt.requestSummaryFor(r, name),
		PreviousReleaseHeldUntil: rt.previousReleaseHeldUntil(r.Context(), name),
		FallbackURL:              rt.fallbackURLFor(r.Context(), *svc),
		StrategyReason:           notes.strategyReason,
		MemoryFromTemplate:       notes.memoryFromTemplate,
	})
}

// previousReleaseHeldUntil is when the held previous release is due for
// removal, nil when none is held or the hold has already lapsed.
func (rt *Router) previousReleaseHeldUntil(ctx context.Context, name string) *time.Time {
	events := rt.appEvents()
	if events == nil {
		return nil
	}
	applied, err := events.GetAppliedConfig(ctx, name)
	if err != nil || applied == nil || !applied.HeldUntil.After(time.Now()) {
		return nil
	}
	until := applied.HeldUntil.UTC()
	return &until
}

// appWithRequests is GET /api/v1/apps/{name}'s body: the app plus its recent
// ingress request summary (rate, error rate, p95), omitted without telemetry.
type appWithRequests struct {
	appResource
	ManagedByStack string                    `json:"managed_by_stack,omitempty"`
	Requests       *telemetry.RequestSummary `json:"requests,omitempty"`
	// PendingImage is the desired image while a rollout to it is in progress
	// or failed, with Image then the last image confirmed serving.
	PendingImage string `json:"pending_image,omitempty"`
	// PreviousReleaseHeldUntil is when the kept previous release, an instant
	// rollback target, is removed. Omitted when none is held.
	PreviousReleaseHeldUntil *time.Time `json:"previous_release_held_until,omitempty"`
	// FallbackURL is the automatic sslip.io URL an app with no domain is
	// routed under. Omitted when it has a domain or none can be built.
	FallbackURL string `json:"fallback_url,omitempty"`
	// StrategyReason is why a template pinned the current strategy, shown only while the strategy still matches.
	StrategyReason string `json:"strategy_reason,omitempty"`
	// MemoryFromTemplate is true while the memory limit is the one the template recommended.
	MemoryFromTemplate bool `json:"memory_from_template,omitempty"`
}

// fallbackURLFor returns the automatic https URL for svc, or "" when it has
// a configured domain, the toggle is off, or no public IP is known.
func (rt *Router) fallbackURLFor(ctx context.Context, svc store.DesiredService) string {
	if len(svc.Domains) > 0 || !ingress.ServesHTTP(svc) {
		return ""
	}
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil || s.FallbackDomainsDisabled {
		return ""
	}
	if d, ok := rt.autoHost(ctx, svc.Name); ok {
		return rt.ingressHTTPSURL(s, d)
	}
	return ""
}

// ingressHTTPSURL is "https://host" with the port clients actually use: the
// public HTTPS port setting when set (or 443 behind an upstream TLS proxy),
// else the ingress listen port, so a link reaches this ingress and not
// whatever owns 443 on the host.
func (rt *Router) ingressHTTPSURL(s store.IngressSettings, host string) string {
	if u, ok := rt.localAppURL(host); ok {
		return u
	}
	port := s.PublicLinkPort()
	if port == 0 {
		port = rt.doctorHTTPSPort
	}
	return httpsURL(host, port)
}

// publicHTTPSURL is "https://host" for a hostname served by this platform,
// carrying the explicit public port only when one is configured.
func (rt *Router) publicHTTPSURL(ctx context.Context, host string) string {
	if u, ok := rt.localAppURL(host); ok {
		return u
	}
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return httpsURL(host, 0)
	}
	return rt.ingressHTTPSURL(s, host)
}

// previewURLFor is publicHTTPSURL for a possibly empty preview domain.
func (rt *Router) previewURLFor(ctx context.Context, domain string) string {
	if domain == "" {
		return ""
	}
	return rt.publicHTTPSURL(ctx, domain)
}

// httpsURL omits the port when it is unset or the scheme default.
func httpsURL(host string, port int) string {
	if port == 0 || port == defaultDoctorHTTPSPort {
		return "https://" + host
	}
	return "https://" + host + ":" + strconv.Itoa(port)
}
