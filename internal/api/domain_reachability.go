package api

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Why a domain has no reachable_url.
const (
	reachableReasonStopped      = "stopped"
	reachableReasonNotResolving = "not_resolving"
	reachableReasonNotRouted    = "not_routed"
	reachableReasonHeldBack     = "held_back"
)

// trafficState is the per-router memory behind reachable_url, the traffic
// summary and the domain doctor.
type trafficState struct {
	mu         sync.Mutex
	lastStatus map[string]domainStatusObservation
	summary    trafficSummaryCache
	// doctorProbes replaces the real network probes in tests.
	doctorProbes *domaindoctor.Probes
}

// domainStatusObservation is the last DNS check status seen for a domain.
type domainStatusObservation struct {
	Status string
	At     time.Time
}

func newTrafficState() *trafficState {
	return &trafficState{lastStatus: map[string]domainStatusObservation{}}
}

// recordCheck remembers status so summaries never need network I/O.
func (t *trafficState) recordCheck(domain, status string) {
	if t == nil || status == "" {
		return
	}
	t.mu.Lock()
	t.lastStatus[strings.ToLower(domain)] = domainStatusObservation{Status: status, At: time.Now().UTC()}
	t.mu.Unlock()
}

func (t *trafficState) lastCheck(domain string) (domainStatusObservation, bool) {
	if t == nil {
		return domainStatusObservation{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	o, ok := t.lastStatus[strings.ToLower(domain)]
	return o, ok
}

// reachabilityInput is everything computeReachability decides from.
type reachabilityInput struct {
	Domain      string
	AppRunning  bool
	HeldBack    bool
	Routed      bool
	CheckStatus string
	Upstream    bool
	LinkPort    int
}

// computeReachability returns the URL a visitor can open, or the reason
// there is none. An unknown DNS status does not block the link.
func computeReachability(in reachabilityInput) (url, reason string) {
	switch {
	case in.HeldBack:
		return "", reachableReasonHeldBack
	case !in.AppRunning:
		return "", reachableReasonStopped
	case !in.Routed:
		return "", reachableReasonNotRouted
	case in.CheckStatus == domainCheckStatusNotResolving,
		in.CheckStatus == domainCheckStatusResolvesElsewhere && !in.Upstream:
		return "", reachableReasonNotResolving
	}
	return httpsURL(in.Domain, in.LinkPort), ""
}

// reachabilityContext is loaded once per request and answers per domain.
type reachabilityContext struct {
	settings store.IngressSettings
	linkPort int
	routed   bool
	running  map[string]bool
	heldBack map[string]bool
	traffic  *trafficState
}

// publicLinkPort is the port a public link carries and whether a standard
// public link exists at all: an ingress stuck on a non-standard listen
// port with no public port configured has none.
func (rt *Router) publicLinkPort(s store.IngressSettings) (port int, routed bool) {
	if p := s.PublicLinkPort(); p != 0 {
		return p, true
	}
	listen := rt.doctorHTTPSPort
	return 0, listen == 0 || listen == defaultDoctorHTTPSPort
}

// loadReachability reads ingress settings, app run state and held back
// import domains. apps limits the condition lookup; nil means every app.
func (rt *Router) loadReachability(ctx context.Context, apps []string) reachabilityContext {
	rc := reachabilityContext{running: map[string]bool{}, heldBack: rt.heldBackDomains(ctx), traffic: rt.traffic}
	if s, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil {
		rc.settings = s
	} else {
		rt.logger.Warn("api: reachability: ingress settings unavailable", slog.String("error", err.Error()))
	}
	rc.linkPort, rc.routed = rt.publicLinkPort(rc.settings)
	rc.running = rt.appsRunning(ctx, apps)
	return rc
}

// appsRunning maps app name to whether it is serving; nil apps means all.
func (rt *Router) appsRunning(ctx context.Context, apps []string) map[string]bool {
	out := map[string]bool{}
	var svcs []store.DesiredService
	if apps == nil {
		all, err := rt.apps.ListDesiredServices(ctx)
		if err != nil {
			rt.logger.Warn("api: reachability: list apps failed", slog.String("error", err.Error()))
			return nil
		}
		svcs = all
	} else {
		for _, name := range apps {
			if svc, err := rt.apps.GetDesiredService(ctx, name); err == nil && svc != nil {
				svcs = append(svcs, *svc)
			}
		}
	}
	controllers := make([]string, 0, len(svcs))
	for _, s := range svcs {
		controllers = append(controllers, applicationControllerName(s.Name))
	}
	conds, err := rt.deploys.GetConditionsForControllers(ctx, controllers)
	if err != nil {
		rt.logger.Warn("api: reachability: load conditions failed", slog.String("error", err.Error()))
		return nil
	}
	for _, s := range svcs {
		label := summarizeAppConditions(conds[applicationControllerName(s.Name)]).Label
		out[s.Name] = !s.Suspended && label != "Stopped" && label != "No status yet"
	}
	return out
}

// heldBackDomains lists domains an app import staged without routing.
func (rt *Router) heldBackDomains(ctx context.Context) map[string]bool {
	out := map[string]bool{}
	if rt.appImports == nil {
		return out
	}
	sessions, err := rt.appImports.ListAppImportSessions(ctx)
	if err != nil {
		return out
	}
	for _, sess := range sessions {
		items, err := rt.appImports.ListAppImportItems(ctx, sess.ID)
		if err != nil {
			continue
		}
		for _, it := range items {
			if !it.Selected || it.State == store.AppImportRouted {
				continue
			}
			for _, d := range it.Domains {
				out[strings.ToLower(d)] = true
			}
		}
	}
	return out
}

func (rc reachabilityContext) isRunning(app string) bool {
	if rc.running == nil {
		return true
	}
	return rc.running[app]
}

func (rc reachabilityContext) forDomain(domain, app string) (url, reason string) {
	status := ""
	if o, ok := rc.traffic.lastCheck(domain); ok {
		status = o.Status
	}
	return computeReachability(reachabilityInput{
		Domain:      domain,
		AppRunning:  rc.isRunning(app),
		HeldBack:    rc.heldBack[strings.ToLower(domain)],
		Routed:      rc.routed,
		CheckStatus: status,
		Upstream:    rc.settings.TLSTerminatedUpstream,
		LinkPort:    rc.linkPort,
	})
}

// annotateDomainReachability fills reachable_url / reachable_reason.
func (rt *Router) annotateDomainReachability(ctx context.Context, rows []domainResource) {
	if len(rows) == 0 {
		return
	}
	rc := rt.loadReachability(ctx, nil)
	for i := range rows {
		rows[i].ReachableURL, rows[i].ReachableReason = rc.forDomain(rows[i].Domain, rows[i].ServiceName)
	}
}

// domainReachability is one domain's entry in an app's reachability map.
type domainReachability struct {
	URL    string `json:"reachable_url,omitempty"`
	Reason string `json:"reachable_reason,omitempty"`
}

// appDomainReachability computes reachability for one app's domains;
// domains not currently routed for the app report not_routed.
func (rt *Router) appDomainReachability(ctx context.Context, app string, routed, all []string) map[string]domainReachability {
	rc := rt.loadReachability(ctx, []string{app})
	out := map[string]domainReachability{}
	for _, d := range all {
		out[d] = domainReachability{Reason: reachableReasonNotRouted}
	}
	for _, d := range routed {
		u, reason := rc.forDomain(d, app)
		out[d] = domainReachability{URL: u, Reason: reason}
	}
	return out
}
