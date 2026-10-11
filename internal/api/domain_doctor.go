package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Env knobs bounding a doctor run.
const (
	envDomainDoctorTimeout      = "APP_DOMAIN_DOCTOR_TIMEOUT"
	envDomainDoctorProbeTimeout = "APP_DOMAIN_DOCTOR_PROBE_TIMEOUT"
	defaultDomainDoctorTimeout  = 20 * time.Second
	defaultDomainDoctorProbe    = 4 * time.Second
)

// serverPublicIPs is the set the doctor may connect to: the configured
// APP_PUBLIC_HOST and this host's detected public addresses. The request's
// Host header is never used, so a caller cannot widen it.
func (rt *Router) serverPublicIPs(ctx context.Context) []string {
	var out []string
	if rt.publicHost != "" {
		lctx, cancel := context.WithTimeout(ctx, domainCheckLookupTimeout)
		out = expectedIPs(lctx, rt.lookupHost, rt.publicHost)
		cancel()
	}
	for _, ip := range rt.detectedPublicIPs(ctx) {
		if !slices.Contains(out, ip) {
			out = append(out, ip)
		}
	}
	return out
}

func doctorAppFacts(conds []reconcile.Condition) domaindoctor.AppFacts {
	label := summarizeAppConditions(conds).Label
	f := domaindoctor.AppFacts{Known: label != "No status yet", Detail: label}
	f.Running = f.Known && label != "Stopped"
	f.Ready = label == "Healthy"
	for _, c := range conds {
		if c.Status == reconcile.ConditionFalse && c.Message != "" {
			f.Detail = c.Message
			break
		}
	}
	return f
}

// domainBelongsTo reports whether domain is configured on svc in any
// environment, or staged for it by an import.
func (rt *Router) domainBelongsTo(ctx context.Context, svc *store.DesiredService, domain string, held map[string]bool) bool {
	for _, d := range svc.Domains {
		if strings.EqualFold(d, domain) {
			return true
		}
	}
	if lister, ok := rt.apps.(serviceEnvironmentDomainStore); ok {
		if sets, err := lister.ListServiceEnvironmentDomains(ctx, svc.Name); err == nil {
			for _, set := range sets {
				for _, d := range set {
					if strings.EqualFold(d, domain) {
						return true
					}
				}
			}
		}
	}
	return held[domain]
}

// doctorFacts gathers stored state; it performs no probe of the domain.
func (rt *Router) doctorFacts(ctx context.Context, svc *store.DesiredService, domain string, held map[string]bool) domaindoctor.Facts {
	f := domaindoctor.Facts{Domain: domain, App: svc.Name, HeldBack: held[domain], ServerIPs: rt.serverPublicIPs(ctx)}
	if s, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil {
		f.TLSTerminatedUpstream = s.TLSTerminatedUpstream
		f.ACMEEnabled = s.EffectiveACMEEnabled()
		f.HTTPSPort = s.PublicLinkPort()
	}
	if conds, err := rt.deploys.GetConditionsForControllers(ctx, []string{applicationControllerName(svc.Name)}); err == nil {
		f.AppState = doctorAppFacts(conds[applicationControllerName(svc.Name)])
	}
	if svc.Suspended {
		f.AppState = domaindoctor.AppFacts{Known: true, Detail: "the app is stopped"}
	}
	if rt.execRuntime != nil {
		f.PortHoldersKnown = true
		for _, h := range rt.portHolders(ctx) {
			f.PortHolders = append(f.PortHolders, domaindoctor.PortHolder{Port: h.Port, Container: h.Container, Kind: string(h.Kind)})
		}
	}
	window := time.Duration(envInt(envTrafficCertExpiryDays, defaultTrafficCertDays)) * 24 * time.Hour
	if certs, err := rt.certSignals(ctx, window, time.Now()); err == nil {
		if c, ok := certs[domain]; ok {
			f.Cert = &domaindoctor.CertFacts{Issuer: c.Issuer, NotAfter: c.NotAfter, Status: c.Status, Renewal: c.Renewal, Source: c.Source}
		}
	}
	if a := acmeFailureFor(domain); a != nil {
		f.ACMEFailure = &domaindoctor.ACMEFailureFacts{Error: a.Error, Action: a.Action, Renewal: a.Renewal}
	}
	return f
}

// handleDomainDoctor handles POST /api/v1/apps/{name}/domains/{domain}/doctor.
func (rt *Router) handleDomainDoctor(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.PathValue("domain")), "."))
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: domain doctor: load app", err)
		return
	}
	held := rt.heldBackDomains(r.Context())
	if domain == "" || !rt.domainBelongsTo(r.Context(), svc, domain, held) {
		writeError(w, http.StatusNotFound, "domain not found on this app")
		return
	}
	probes := domaindoctor.DefaultProbes()
	if rt.traffic != nil && rt.traffic.doctorProbes != nil {
		probes = *rt.traffic.doctorProbes
	}
	rep := domaindoctor.Run(r.Context(), rt.doctorFacts(r.Context(), svc, domain, held), probes, domaindoctor.Options{
		Timeout:        envDuration(envDomainDoctorTimeout, defaultDomainDoctorTimeout),
		ProbeTimeout:   envDuration(envDomainDoctorProbeTimeout, defaultDomainDoctorProbe),
		ExpiryWarnDays: envInt(envTrafficCertExpiryDays, defaultTrafficCertDays),
	})
	writeJSON(w, http.StatusOK, rep)
}
