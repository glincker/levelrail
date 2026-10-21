package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
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
	f := domaindoctor.Facts{Domain: domain, App: svc.Name, HeldBack: held[domain], ServerIPs: rt.serverPublicIPs(ctx),
		ServerHost: rt.publicHost, CLIName: rt.cliName()}
	if mgr, provider, zone, err := rt.dnsRecordManager(ctx, domain); err == nil && mgr != nil {
		f.DNSProvider, f.DNSZone = provider, zone
	}
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
	tls := rt.tlsSnapshot(ctx)
	if tls.ProxyManaged(domain) {
		f.TLSTerminatedUpstream = true
		f.ACMEEnabled = false
		f.Cert = nil
		if pc, ok := tls.ProxyCert(domain); ok {
			f.Cert = &domaindoctor.CertFacts{
				Issuer: pc.Issuer, NotAfter: pc.NotAfter, Source: certSourceProxy,
				Status: alerting.CertExpiryStatus(pc.NotAfter, time.Now(), window),
			}
		}
	}
	if a := acmeFailureIn(tls, domain); a != nil {
		f.ACMEFailure = &domaindoctor.ACMEFailureFacts{Error: a.Error, Action: a.Action, Renewal: a.Renewal}
	}
	return f
}

// runDomainDoctor loads the stored facts for domain on app name and runs
// every check, calling emit (when set) as each finishes. A non-zero status
// is the HTTP error the caller should send with msg.
func (rt *Router) runDomainDoctor(ctx context.Context, name, domain string, emit func(domaindoctor.Check)) (rep domaindoctor.Report, status int, msg string) {
	return rt.runDomainDoctorWith(ctx, name, domain, emit, nil)
}

// runDomainDoctorWith is runDomainDoctor with a hook to adjust the facts, so
// the certificate preflight can run the CAA and port checks as if public
// certificates were already on.
func (rt *Router) runDomainDoctorWith(ctx context.Context, name, domain string, emit func(domaindoctor.Check), adjust func(*domaindoctor.Facts)) (rep domaindoctor.Report, status int, msg string) {
	svc, target, err := rt.doctorSubject(ctx, name, domain)
	if errors.Is(err, store.ErrServiceNotFound) {
		return rep, http.StatusNotFound, "app not found"
	}
	if err != nil {
		rt.logger.Error("api: domain doctor: load app", "error", err.Error(), "app", name, "domain", domain)
		return rep, http.StatusInternalServerError, "internal error"
	}
	held := rt.heldBackDomains(ctx)
	if domain == "" || (target == nil && !rt.domainBelongsTo(ctx, svc, domain, held)) {
		return rep, http.StatusNotFound, "domain not found on this app"
	}
	facts := rt.doctorFacts(ctx, svc, domain, held)
	facts.Target = target
	if adjust != nil {
		adjust(&facts)
	}
	probes := domaindoctor.DefaultProbes()
	if rt.traffic != nil && rt.traffic.doctorProbes != nil {
		probes = *rt.traffic.doctorProbes
	}
	rep = domaindoctor.RunStream(ctx, facts, probes, domaindoctor.Options{
		Timeout:        envDuration(envDomainDoctorTimeout, defaultDomainDoctorTimeout),
		ProbeTimeout:   envDuration(envDomainDoctorProbeTimeout, defaultDomainDoctorProbe),
		ExpiryWarnDays: envInt(envTrafficCertExpiryDays, defaultTrafficCertDays),
	}, emit)
	if target == nil {
		rep.GoLive = "/apps/" + url.PathEscape(svc.Name) + "/go-live?domain=" + url.QueryEscape(domain)
	}
	return rep, 0, ""
}

// handleDomainDoctor handles POST /api/v1/apps/{name}/domains/{domain}/doctor.
func (rt *Router) handleDomainDoctor(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.PathValue("domain")), "."))
	rep, status, msg := rt.runDomainDoctor(r.Context(), r.PathValue("name"), domain, nil)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
