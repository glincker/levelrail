package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/certflow"
	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/golive"
	"github.com/GLINCKER/levelrail/internal/hostfw"
	"github.com/GLINCKER/levelrail/internal/hostprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Env knobs for the go live checklist.
const (
	envGoLivePollInterval = "APP_GOLIVE_POLL_INTERVAL"
	envGoLiveStreamRounds = "APP_GOLIVE_STREAM_ROUNDS"
	envGoLiveFirewallWait = "APP_GOLIVE_FIREWALL_TIMEOUT"

	defaultGoLivePoll     = 10 * time.Second
	defaultGoLiveRounds   = 60
	defaultGoLiveFirewall = 5 * time.Second
)

// Checklist stream message types.
const (
	checklistEventSnapshot = "checklist"
	checklistEventDone     = "done"
)

// openWebPortsFixID is the typed fix that opens ports 80 and 443 in the host firewall.
const openWebPortsFixID = "host.firewall.open_web_ports"

type goLiveChecklistState struct {
	seenOnce sync.Once
	seen     *golive.Seen
	provOnce sync.Once
	provider *hostprovider.Guidance
	provDeps *hostprovider.Deps
}

// WithGoLiveProviderDeps replaces how the cloud provider is detected, for tests.
func WithGoLiveProviderDeps(d hostprovider.Deps) Option {
	return func(rt *Router) { rt.checklist.provDeps = &d }
}

func (rt *Router) goLiveSeen() *golive.Seen {
	rt.checklist.seenOnce.Do(func() { rt.checklist.seen = golive.NewSeen() })
	return rt.checklist.seen
}

// goLiveProvider detects the cloud provider once per process: the answer does not change while it runs.
func (rt *Router) goLiveProvider(ctx context.Context) *hostprovider.Guidance {
	rt.checklist.provOnce.Do(func() {
		deps := hostprovider.SystemDeps()
		if rt.checklist.provDeps != nil {
			deps = *rt.checklist.provDeps
		}
		if g, ok := hostprovider.Detect(ctx, deps); ok {
			rt.checklist.provider = &g
		}
	})
	return rt.checklist.provider
}

func (rt *Router) hostFirewallDeps() hostfw.Deps {
	return hostfw.Deps{LookPath: rt.hostFirewallLookPath, Run: hostfw.Runner(rt.hostFirewallRun)}
}

func (rt *Router) goLiveAppState(ctx context.Context, svc *store.DesiredService) golive.AppInput {
	out := golive.AppInput{DiagnosisHref: "/apps/" + url.PathEscape(svc.Name) + "/deploys"}
	conds, err := rt.deploys.GetConditionsForControllers(ctx, []string{applicationControllerName(svc.Name)})
	if err != nil {
		rt.logger.Warn("api: go live: load conditions failed", slog.String("error", err.Error()), slog.String("app", svc.Name))
		return out
	}
	f := doctorAppFacts(conds[applicationControllerName(svc.Name)])
	if svc.Suspended {
		f = domaindoctor.AppFacts{Known: true, Detail: "the app is stopped"}
	}
	out.Known, out.Running, out.Ready, out.Detail = f.Known, f.Running, f.Ready, f.Detail
	return out
}

func readinessPath(svc *store.DesiredService) string {
	if svc.Health != nil && svc.Health.Readiness != nil && svc.Health.Readiness.Path != "" {
		return svc.Health.Readiness.Path
	}
	return "/"
}

func (rt *Router) goLivePort(ctx context.Context, svc *store.DesiredService, running bool) (golive.PortInput, int) {
	res := rt.exposureState(ctx, svc)
	p := golive.PortInput{DirectAllowed: res.LocalNode && rt.publicHost != "", NetworkHref: "/apps/" + url.PathEscape(svc.Name) + "/network"}
	for _, u := range res.URLs {
		switch u.Kind {
		case "direct":
			p.DirectURL = u.URL
		case "fallback":
			if p.ZeroConfigURL == "" {
				p.ZeroConfigURL = u.URL
			}
		}
	}
	if p.ZeroConfigURL == "" {
		if ing, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil && !ing.FallbackDomainsDisabled {
			if fb, ok := rt.autoHost(ctx, svc.Name); ok {
				p.ZeroConfigURL = rt.ingressHTTPSURL(ing, fb)
			}
		}
	}
	hostPort := 0
	if running && res.LocalNode {
		var live bool
		if hostPort, live = rt.observedHostPort(ctx, svc); live && hostPort > 0 {
			t := probeTarget{URL: "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort)) + readinessPath(svc)}
			rt.probeHTTP(ctx, &t, net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort)), probeContainerNotListen)
			p.Probe = &golive.PortProbe{OK: t.OK, Status: t.Status, Failure: t.Failure}
		}
	}
	return p, hostPort
}

// hostFirewallFixKind is auto only when this process can really change the host firewall.
func (rt *Router) hostFirewallFixKind() (fixit.Kind, string) {
	read := rt.fixit.host
	if read == nil {
		reader := fixit.SystemHostReader()
		read = func() fixit.HostEnv { return fixit.DetectHostEnv(reader) }
	}
	env := read()
	switch {
	case env.Mode == fixit.RunDocker:
		return fixit.KindGuided, "the control plane runs in a container and cannot change the host firewall"
	case !env.Root:
		return fixit.KindGuided, "the control plane does not run as root, which changing the firewall needs"
	}
	return fixit.KindAuto, ""
}

func (rt *Router) goLiveFirewall(ctx context.Context) golive.FirewallInput {
	fctx, cancel := context.WithTimeout(ctx, envDuration(envGoLiveFirewallWait, defaultGoLiveFirewall))
	defer cancel()
	st := hostfw.Detect(fctx, rt.hostFirewallDeps())
	kind, _ := rt.hostFirewallFixKind()
	return golive.FirewallInput{Host: st, Plan: hostfw.PlanOpen(st), FixID: openWebPortsFixID, FixKind: kind, Provider: rt.goLiveProvider(ctx)}
}

func (rt *Router) goLiveCert(ctx context.Context, domain string, pre certflow.Result) (golive.CertInput, *golive.Failure, time.Time) {
	in := golive.CertInput{Ready: pre.Ready, NextSafeAt: pre.NextSafeAt, CertAPI: "POST /api/v1/domains/" + url.PathEscape(domain) + "/cert"}
	for _, it := range pre.Items {
		if it.State == certflow.StateFail {
			in.Blocker, in.BlockerFix = it.Detail, it.Fix
			break
		}
	}
	st, err := rt.domainCertStatus(ctx, domain)
	if err != nil {
		rt.logger.Warn("api: go live: certificate status failed", slog.String("error", err.Error()), slog.String("domain", domain))
		return in, nil, time.Time{}
	}
	in.Status = golive.CertStatus{State: st.State, Staging: st.Staging, Trusted: st.State == certFlowIssued && st.Kind.Trusted() && !st.Staging,
		Issuer: st.Issuer, Error: st.Error, Explanation: st.Explanation, Hint: st.Hint, NextSafeAt: st.NextSafeAt}
	if st.NotAfter != nil {
		in.Status.NotAfter = *st.NotAfter
	}
	if st.RenewalAt != nil {
		in.Status.RenewalAt = *st.RenewalAt
	}
	if rows, err := rt.certTruthRows(ctx, time.Now()); err == nil {
		for _, row := range rows {
			if certInfoCovers(row.Info, domain) && (in.Status.NotBefore.IsZero() || row.Info.NotBefore.After(in.Status.NotBefore)) {
				in.Status.NotBefore = row.Info.NotBefore
			}
		}
	}
	var failure *golive.Failure
	if f := acmeFailureFor(domain); f != nil {
		failure = &golive.Failure{At: f.At, Error: f.Error, Explanation: domaindoctor.ExplainACME(f.Error), Renewal: f.Renewal}
	}
	return in, failure, rt.certLedger.StartedAt(domain)
}

// goLiveReference asks the app directly on its published port, so the HTTPS answer can be matched to it.
func goLiveReference(ctx context.Context, hostPort int, path string) *golive.Reference {
	if hostPort <= 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort))+path, nil)
	if err != nil {
		return nil
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	return &golive.Reference{Status: resp.StatusCode, ContentType: resp.Header.Get("Content-Type"), Server: resp.Header.Get("Server"),
		BodyHash: golive.HashBody(io.LimitReader(resp.Body, 64<<10))}
}

// goLiveSelfCheck runs only when the domain resolves here and a trusted certificate is in place.
func (rt *Router) goLiveSelfCheck(ctx context.Context, svc *store.DesiredService, domain string, rep domaindoctor.Report, hostPort int) *golive.SelfCheckResult {
	if !rep.Probed {
		return nil
	}
	lctx, cancel := context.WithTimeout(ctx, domainCheckLookupTimeout)
	resolved, err := rt.lookupHost(lctx, domain)
	cancel()
	if err != nil {
		return nil
	}
	here := intersectStrings(resolved, rt.serverPublicIPs(ctx))
	if len(here) == 0 {
		return nil
	}
	port := 443
	if s, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil {
		port = s.PublicLinkPort()
	}
	path := readinessPath(svc)
	res := golive.SelfCheck(ctx, golive.SelfCheckInput{Domain: domain, Addr: net.JoinHostPort(here[0], strconv.Itoa(port)), Path: path,
		Reference: goLiveReference(ctx, hostPort, path), Timeout: envDuration(envDomainDoctorProbeTimeout, defaultDomainDoctorProbe)})
	return &res
}

func intersectStrings(a, b []string) []string {
	var out []string
	for _, x := range a {
		for _, y := range b {
			if x == y {
				out = append(out, x)
				break
			}
		}
	}
	return out
}

// buildGoLiveChecklist gathers every fact once and hands them to the pure builder.
func (rt *Router) buildGoLiveChecklist(r *http.Request, svc *store.DesiredService, domain string) (golive.Checklist, int, string) {
	ctx := r.Context()
	in := golive.Input{App: svc.Name, Domain: domain, Now: time.Now(), Seen: rt.goLiveSeen(), PollEvery: envDuration(envGoLivePollInterval, defaultGoLivePoll), CLIName: rt.cliName()}
	in.AppState = rt.goLiveAppState(ctx, svc)
	var hostPort int
	in.Port, hostPort = rt.goLivePort(ctx, svc, in.AppState.Running)
	in.Firewall = rt.goLiveFirewall(ctx)
	if domain == "" {
		return golive.Build(in), 0, ""
	}
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		rt.logger.Error("api: go live: load settings", slog.String("error", err.Error()), slog.String("app", svc.Name))
		return golive.Checklist{}, http.StatusInternalServerError, "internal error"
	}
	rep, status, msg := rt.runDomainDoctorWith(ctx, svc.Name, domain, nil, publicCertDoctorFacts(s))
	if status != 0 {
		return golive.Checklist{}, status, msg
	}
	in.Report = &rep
	pre, status, msg := rt.certPreflightFromReport(r, domain, "", false, s, rep)
	if status != 0 {
		return golive.Checklist{}, status, msg
	}
	in.Cert, in.Failure, in.ChallengeStartedAt = rt.goLiveCert(ctx, domain, pre)
	if in.AppState.Ready && in.Cert.Status.Trusted {
		in.Live = rt.goLiveSelfCheck(ctx, svc, domain, rep, hostPort)
	}
	return golive.Build(in), 0, ""
}

// goLiveTarget loads the app and picks the domain: the query value, else the
// app's only domain, else none (the checklist then covers the first steps).
func (rt *Router) goLiveTarget(w http.ResponseWriter, r *http.Request) (*store.DesiredService, string, bool) {
	svc, ok := rt.loadExposureApp(w, r)
	if !ok {
		return nil, "", false
	}
	domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.URL.Query().Get("domain")), "."))
	if domain == "" && len(svc.Domains) == 1 {
		domain = strings.ToLower(svc.Domains[0])
	}
	return svc, domain, true
}

// handleGoLiveChecklist handles GET /api/v1/apps/{name}/go-live/checklist.
func (rt *Router) handleGoLiveChecklist(w http.ResponseWriter, r *http.Request) {
	svc, domain, ok := rt.goLiveTarget(w, r)
	if !ok {
		return
	}
	cl, status, msg := rt.buildGoLiveChecklist(r, svc, domain)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, cl)
}

type checklistEvent struct {
	Type      string            `json:"type"`
	Checklist *golive.Checklist `json:"checklist,omitempty"`
	Reason    string            `json:"reason,omitempty"`
}

// handleGoLiveChecklistStream handles GET /api/v1/apps/{name}/go-live/stream:
// a fresh checklist every poll interval until the domain is live twice in a
// row, the client leaves, or the round budget is spent.
func (rt *Router) handleGoLiveChecklistStream(w http.ResponseWriter, r *http.Request) {
	svc, domain, ok := rt.goLiveTarget(w, r)
	if !ok {
		return
	}
	flusher, ok := startSSE(w)
	if !ok {
		return
	}
	send := func(ev checklistEvent) {
		data, err := json.Marshal(ev)
		if err != nil {
			rt.logger.Error("api: go live stream: marshal", slog.String("error", err.Error()), slog.String("app", svc.Name))
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	rounds, liveRuns := envIntDefault(envGoLiveStreamRounds, defaultGoLiveRounds), 0
	every := envDuration(envGoLivePollInterval, defaultGoLivePoll)
	for i := 0; i < rounds; i++ {
		cl, status, msg := rt.buildGoLiveChecklist(r, svc, domain)
		if status != 0 {
			rt.logger.Warn("api: go live stream failed", slog.Int("status", status), slog.String("app", svc.Name), slog.String("reason", msg))
			send(checklistEvent{Type: checklistEventDone, Reason: msg})
			return
		}
		send(checklistEvent{Type: checklistEventSnapshot, Checklist: &cl})
		if liveRuns = nextLiveRuns(liveRuns, cl.Live); liveRuns >= 2 {
			send(checklistEvent{Type: checklistEventDone, Reason: "live"})
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-time.After(every):
		}
	}
	send(checklistEvent{Type: checklistEventDone, Reason: "budget"})
}

func nextLiveRuns(n int, live bool) int {
	if live {
		return n + 1
	}
	return 0
}

type liveStatusEntry struct {
	App    string       `json:"app"`
	Domain string       `json:"domain"`
	State  golive.State `json:"state"`
	Basis  string       `json:"basis"`
}

// handleLiveBadges handles GET /api/v1/go-live/status: one badge per domain
// from stored state only (app status, stored certificate, last CA error), so
// lists stay fast. The checklist runs the real probes.
func (rt *Router) handleLiveBadges(w http.ResponseWriter, r *http.Request) {
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: go live status: visibility", err)
		return
	}
	owners, err := rt.domainOwners(r.Context())
	if err != nil {
		rt.internalError(w, "api: go live status: owners", err)
		return
	}
	rows, err := rt.certTruthRows(r.Context(), time.Now())
	if err != nil {
		rt.internalError(w, "api: go live status: certificates", err)
		return
	}
	running := rt.appsRunning(r.Context(), nil)
	out := []liveStatusEntry{}
	for domain, apps := range owners {
		for _, app := range apps {
			if canSee(app) {
				out = append(out, liveStatusEntry{App: app, Domain: domain, State: rt.storedLiveState(domain, running[app], rows), Basis: "stored"})
				break
			}
		}
	}
	sortLiveEntries(out)
	writeJSON(w, http.StatusOK, map[string]any{"domains": out})
}

func (rt *Router) storedLiveState(domain string, running bool, rows []certTruthRow) golive.State {
	var best *certTruthRow
	for i := range rows {
		if certInfoCovers(rows[i].Info, domain) && (best == nil || rows[i].Info.NotAfter.After(best.Info.NotAfter)) {
			best = &rows[i]
		}
	}
	trusted := best != nil && best.Kind.Trusted() && best.Info.NotAfter.After(time.Now())
	switch {
	case !running:
		return golive.StateNotLive
	case trusted:
		return golive.StateLive
	case acmeFailureFor(domain) != nil:
		return golive.StateNeedsAttention
	case !rt.certLedger.StartedAt(domain).IsZero():
		return golive.StateWaitingCertificate
	}
	return golive.StateWaitingDNS
}

func sortLiveEntries(e []liveStatusEntry) {
	for i := 1; i < len(e); i++ {
		for j := i; j > 0 && e[j].Domain < e[j-1].Domain; j-- {
			e[j], e[j-1] = e[j-1], e[j]
		}
	}
}
