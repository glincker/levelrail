package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

// Env knobs for the traffic summary.
const (
	envTrafficCertExpiryDays   = "APP_TRAFFIC_CERT_EXPIRY_DAYS"
	envTrafficSummaryCacheTTL  = "APP_TRAFFIC_SUMMARY_CACHE_TTL"
	defaultTrafficCertDays     = 30
	defaultTrafficSummaryCache = 5 * time.Second
)

// Domain classes counted by the summary, matching the dashboard taxonomy.
const (
	trafficClassLive           = "live"
	trafficClassWaiting        = "waiting"
	trafficClassPropagating    = "propagating"
	trafficClassNeedsAttention = "needs_attention"
	trafficClassNotSetUp       = "not_set_up"
	trafficClassPaused         = "paused"
	trafficClassUnknown        = "unknown"
)

// Signals the summary cannot compute yet on this control plane.
const (
	trafficSignalZones  = "zones_not_delegated"
	trafficSignalRoutes = "routes_with_errors"
)

// certSignal is a stored certificate's state for one hostname.
type certSignal struct {
	Issuer   string
	NotAfter time.Time
	Status   string
	Renewal  string
	Source   string
}

// domainSignals is the stored state classifyDomain decides from.
type domainSignals struct {
	CheckStatus string
	Maintenance bool
	Cert        *certSignal
	ACMEAction  string
	Upstream    bool
}

// classifyDomain follows the dashboard's status table, top to bottom.
func classifyDomain(s domainSignals) string {
	acmeFailing := s.ACMEAction != "" && s.ACMEAction != acmeActionWaitRetry
	switch {
	case s.Maintenance:
		return trafficClassPaused
	case s.Cert != nil && (s.Cert.Status == certStatusExpired || s.Cert.Renewal == alerting.CertRenewalStalled),
		acmeFailing,
		s.CheckStatus == domainCheckStatusResolvesElsewhere && !s.Upstream:
		return trafficClassNeedsAttention
	case s.CheckStatus == domainCheckStatusUnconfigured:
		return trafficClassNotSetUp
	case s.CheckStatus == domainCheckStatusNotResolving:
		return trafficClassWaiting
	case s.CheckStatus == domainCheckStatusPropagating:
		return trafficClassPropagating
	case s.CheckStatus == domainCheckStatusConnected, s.CheckStatus == domainCheckStatusResolvesElsewhere:
		return trafficClassLive
	}
	return trafficClassUnknown
}

const (
	certStatusExpired      = "expired"
	certStatusExpiringSoon = "expiring_soon"
)

type trafficDomainCounts struct {
	Total          int `json:"total"`
	Live           int `json:"live"`
	Waiting        int `json:"waiting"`
	Propagating    int `json:"propagating"`
	NeedsAttention int `json:"needs_attention"`
	NotSetUp       int `json:"not_set_up"`
	Paused         int `json:"paused"`
	Unknown        int `json:"unknown"`
}

type trafficCertCounts struct {
	WindowDays     int `json:"window_days"`
	Expiring       int `json:"expiring"`
	Expired        int `json:"expired"`
	RenewalFailing int `json:"renewal_failing"`
}

// trafficSummaryResource is GET /api/v1/traffic/summary's body.
type trafficSummaryResource struct {
	GeneratedAt         time.Time           `json:"generated_at"`
	Domains             trafficDomainCounts `json:"domains"`
	Certificates        trafficCertCounts   `json:"certificates"`
	DomainsNotResolving int                 `json:"domains_not_resolving"`
	ZonesNotDelegated   *int                `json:"zones_not_delegated,omitempty"`
	RoutesWithErrors    *int                `json:"routes_with_errors,omitempty"`
	Attention           int                 `json:"attention"`
	Unavailable         []string            `json:"unavailable,omitempty"`
}

// trafficDomainState is one classified domain inside a cached snapshot.
type trafficDomainState struct {
	Domain       string
	App          string
	Class        string
	CertExpiring bool
	CertExpired  bool
	CertFailing  bool
}

type trafficSnapshot struct {
	at         time.Time
	windowDays int
	domains    []trafficDomainState
}

// trafficSummaryCache keeps the last snapshot; visibility filtering runs
// per request on top of it.
type trafficSummaryCache struct {
	snap *trafficSnapshot
}

// certSignals maps every hostname a stored certificate covers to its state.
func (rt *Router) certSignals(ctx context.Context, window time.Duration, now time.Time) (map[string]certSignal, error) {
	infos, err := alerting.ListCertificates(ctx, rt.certs, window, now, rt.logger)
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}
	var obs []alerting.CertExpiryObservation
	if rt.certObservations != nil {
		obs, _ = rt.certObservations.ListAllCertExpiryObservations(ctx)
	}
	renewal := alerting.CertRenewalStates(infos, obs, rt.certRenewalStalledThreshold, now)
	custom, err := rt.customTLSCertDomains(ctx)
	if err != nil {
		custom = map[string]bool{}
	}
	out := map[string]certSignal{}
	for _, info := range infos {
		sig := certSignal{Issuer: info.Issuer, NotAfter: info.NotAfter, Status: info.Status, Renewal: renewal[info.Domain], Source: "acme"}
		if custom[strings.ToLower(info.Domain)] {
			sig.Source = "custom"
		}
		for _, name := range append([]string{info.Domain}, info.SANs...) {
			key := strings.ToLower(name)
			if prev, ok := out[key]; !ok || sig.NotAfter.After(prev.NotAfter) {
				out[key] = sig
			}
		}
	}
	return out, nil
}

func (rt *Router) buildTrafficSnapshot(ctx context.Context, now time.Time) (*trafficSnapshot, error) {
	days := envInt(envTrafficCertExpiryDays, defaultTrafficCertDays)
	window := time.Duration(days) * 24 * time.Hour
	domains, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}
	certs, err := rt.certSignals(ctx, window, now)
	if err != nil {
		return nil, err
	}
	maintenance := map[string]bool{}
	if list, err := rt.domainMaintenance.ListDomainMaintenance(ctx); err == nil {
		for _, d := range list {
			maintenance[strings.ToLower(d)] = true
		}
	}
	upstream := false
	if s, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil {
		upstream = s.TLSTerminatedUpstream
	}
	snap := &trafficSnapshot{at: now, windowDays: days}
	for _, d := range domains {
		key := strings.ToLower(d.Domain)
		sig := domainSignals{Maintenance: maintenance[key], Upstream: upstream}
		if o, ok := rt.traffic.lastCheck(key); ok {
			sig.CheckStatus = o.Status
		}
		if c, ok := certs[key]; ok {
			sig.Cert = &c
		}
		if f := acmeFailureFor(key); f != nil {
			sig.ACMEAction = f.Action
		}
		st := trafficDomainState{Domain: key, App: d.ServiceName, Class: classifyDomain(sig)}
		if sig.Cert != nil {
			st.CertExpired = sig.Cert.Status == certStatusExpired
			st.CertExpiring = !st.CertExpired && sig.Cert.NotAfter.Before(now.Add(window))
			st.CertFailing = sig.Cert.Renewal == alerting.CertRenewalStalled
		}
		if sig.ACMEAction != "" && sig.ACMEAction != acmeActionWaitRetry {
			st.CertFailing = true
		}
		snap.domains = append(snap.domains, st)
	}
	return snap, nil
}

// trafficSnapshotCached rebuilds at most once per cache TTL.
func (rt *Router) trafficSnapshotCached(ctx context.Context) (*trafficSnapshot, error) {
	now := time.Now().UTC()
	ttl := envDuration(envTrafficSummaryCacheTTL, defaultTrafficSummaryCache)
	t := rt.traffic
	t.mu.Lock()
	snap := t.summary.snap
	t.mu.Unlock()
	if snap != nil && now.Sub(snap.at) < ttl {
		return snap, nil
	}
	snap, err := rt.buildTrafficSnapshot(ctx, now)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.summary.snap = snap
	t.mu.Unlock()
	return snap, nil
}

// summarize counts the snapshot's domains the caller may see.
func summarize(snap *trafficSnapshot, canSee func(string) bool) trafficSummaryResource {
	out := trafficSummaryResource{GeneratedAt: snap.at, Certificates: trafficCertCounts{WindowDays: snap.windowDays},
		Unavailable: []string{trafficSignalZones, trafficSignalRoutes}}
	for _, d := range snap.domains {
		if !canSee(d.App) {
			continue
		}
		out.Domains.Total++
		switch d.Class {
		case trafficClassLive:
			out.Domains.Live++
		case trafficClassWaiting:
			out.Domains.Waiting++
			out.DomainsNotResolving++
		case trafficClassPropagating:
			out.Domains.Propagating++
		case trafficClassNeedsAttention:
			out.Domains.NeedsAttention++
		case trafficClassNotSetUp:
			out.Domains.NotSetUp++
		case trafficClassPaused:
			out.Domains.Paused++
		default:
			out.Domains.Unknown++
		}
		if d.CertExpiring {
			out.Certificates.Expiring++
		}
		if d.CertExpired {
			out.Certificates.Expired++
		}
		if d.CertFailing {
			out.Certificates.RenewalFailing++
		}
	}
	out.Attention = out.Domains.NeedsAttention
	return out
}

// handleTrafficSummary handles GET /api/v1/traffic/summary: counts from
// stored state and the last known DNS checks, never network I/O.
func (rt *Router) handleTrafficSummary(w http.ResponseWriter, r *http.Request) {
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: traffic summary: visibility", err)
		return
	}
	snap, err := rt.trafficSnapshotCached(r.Context())
	if err != nil {
		rt.internalError(w, "api: traffic summary failed", err)
		return
	}
	writeJSON(w, http.StatusOK, summarize(snap, canSee))
}
