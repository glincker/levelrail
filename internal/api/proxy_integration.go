package api

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ProxyIntegrationStore is the persistence the proxy integration routes need
// beyond what the syncer reads.
type ProxyIntegrationStore interface {
	GetProxyIntegrationSettings(ctx context.Context) (store.ProxyIntegrationSettings, error)
	UpdateProxyIntegrationSettings(ctx context.Context, s store.ProxyIntegrationSettings) error
	ListProxyRouteStatus(ctx context.Context) ([]store.ProxyRouteStatus, error)
	SaveProxyRouteVerification(ctx context.Context, s store.ProxyRouteStatus) error
}

// proxyIntegrationDeps is set by SetProxyIntegration; nil syncer means the
// routes answer 501.
type proxyIntegrationDeps struct {
	syncer        *proxyroutes.Syncer
	store         ProxyIntegrationStore
	verifyTimeout time.Duration
	// detect is overridable in tests; nil uses the Docker runtime.
	detect func(ctx context.Context) proxyroutes.Detection
	// probe and routerLoaded are overridable in tests.
	probe        func(ctx context.Context, t proxyroutes.ProbeTarget) proxyroutes.ProbeResult
	routerLoaded func(ctx context.Context, base, router string, timeout time.Duration) (bool, error)
	// setupMu makes setup one at a time so two clicks cannot interleave.
	setupMu sync.Mutex
}

// SetProxyIntegration enables the managed proxy routes API. syncer must be
// the same instance the reconciler uses.
func (rt *Router) SetProxyIntegration(syncer *proxyroutes.Syncer, st ProxyIntegrationStore, verifyTimeout time.Duration) {
	rt.proxyIntegration = &proxyIntegrationDeps{
		syncer: syncer, store: st, verifyTimeout: verifyTimeout,
		probe: proxyroutes.Probe, routerLoaded: proxyroutes.RouterLoaded,
	}
}

type proxySettingsResource struct {
	Integration     string `json:"integration"`
	DynamicDir      string `json:"dynamic_dir"`
	EntrypointHTTP  string `json:"entrypoint_http"`
	EntrypointHTTPS string `json:"entrypoint_https"`
	CertResolver    string `json:"cert_resolver"`
	UpstreamHost    string `json:"upstream_host"`
}

type proxyIngressResource struct {
	HTTPPort              int    `json:"http_port"`
	HTTPSPort             int    `json:"https_port"`
	DashboardAddr         string `json:"dashboard_addr"`
	TLSTerminatedUpstream bool   `json:"tls_terminated_upstream"`
	PublicHTTPSPort       int    `json:"public_https_port"`
}

type proxyCertificateResource struct {
	Issuer   string `json:"issuer"`
	NotAfter string `json:"not_after"`
	Valid    bool   `json:"valid"`
}

type proxyDomainResource struct {
	Domain      string                    `json:"domain"`
	Target      string                    `json:"target"`
	App         string                    `json:"app"`
	File        string                    `json:"file"`
	State       string                    `json:"state"`
	ProxyLoaded *bool                     `json:"proxy_loaded"`
	Reachable   bool                      `json:"reachable"`
	Certificate *proxyCertificateResource `json:"certificate"`
	LastError   string                    `json:"last_error"`
	CheckedAt   string                    `json:"checked_at"`
}

type proxyStepResource struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

type proxyIntegrationResource struct {
	Detected proxyroutes.Detection `json:"detected"`
	Settings proxySettingsResource `json:"settings"`
	Ingress  proxyIngressResource  `json:"ingress"`
	Domains  []proxyDomainResource `json:"domains"`
	Steps    []proxyStepResource   `json:"steps"`
}

// Step ids and states of the contract.
const (
	proxyStepDetect      = "detect"
	proxyStepTLSUpstream = "tls_upstream"
	proxyStepRoutes      = "routes"
	proxyStepDNS         = "dns"
	proxyStepVerify      = "verify"

	proxyStepDone    = "done"
	proxyStepTodo    = "todo"
	proxyStepBlocked = "blocked"
	proxyStepError   = "error"
)

func toProxySettingsResource(s store.ProxyIntegrationSettings) proxySettingsResource {
	return proxySettingsResource{
		Integration: s.Mode, DynamicDir: s.DynamicDir, EntrypointHTTP: s.EntrypointHTTP,
		EntrypointHTTPS: s.EntrypointHTTPS, CertResolver: s.CertResolver, UpstreamHost: s.UpstreamHost,
	}
}

func (rt *Router) proxyIntegrationReady(w http.ResponseWriter) (*proxyIntegrationDeps, bool) {
	if rt.proxyIntegration == nil {
		writeError(w, http.StatusNotImplemented, "managed proxy routes are not available on this control plane")
		return nil, false
	}
	return rt.proxyIntegration, true
}

// handleGetProxyIntegration handles GET /api/v1/system/proxy-integration.
func (rt *Router) handleGetProxyIntegration(w http.ResponseWriter, r *http.Request) {
	deps, ok := rt.proxyIntegrationReady(w)
	if !ok {
		return
	}
	res, err := rt.buildProxyIntegration(r.Context(), deps, rt.detectProxy(r.Context()))
	if err != nil {
		rt.internalError(w, "proxy integration: build", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) buildProxyIntegration(ctx context.Context, deps *proxyIntegrationDeps, det proxyroutes.Detection) (proxyIntegrationResource, error) {
	plan, err := deps.syncer.Plan(ctx)
	if err != nil {
		return proxyIntegrationResource{}, err
	}
	ingress, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return proxyIntegrationResource{}, err
	}
	statuses, err := deps.store.ListProxyRouteStatus(ctx)
	if err != nil {
		return proxyIntegrationResource{}, err
	}
	byDomain := make(map[string]store.ProxyRouteStatus, len(statuses))
	for _, s := range statuses {
		byDomain[s.Domain] = s
	}
	res := proxyIntegrationResource{
		Detected: det,
		Settings: toProxySettingsResource(plan.Settings),
		Ingress: proxyIngressResource{
			HTTPPort: portOf(deps.syncer.Ingress.Addr), HTTPSPort: rt.ingressHTTPSPort(), DashboardAddr: deps.syncer.Dashboard.Addr,
			TLSTerminatedUpstream: ingress.TLSTerminatedUpstream, PublicHTTPSPort: ingress.PublicHTTPSPort,
		},
		Domains: []proxyDomainResource{},
	}
	for _, it := range plan.Items {
		res.Domains = append(res.Domains, proxyDomain(plan, it, byDomain[it.Domain]))
	}
	res.Steps = rt.proxySteps(ctx, det, ingress, plan, res.Domains)
	return res, nil
}

func proxyDomain(plan proxyroutes.Plan, it proxyroutes.Item, st store.ProxyRouteStatus) proxyDomainResource {
	d := proxyDomainResource{Domain: it.Domain, Target: it.Kind, App: it.App, File: it.File}
	if !plan.Settings.Enabled() {
		d.State = proxyroutes.StateMissing
	} else {
		d.State, d.LastError = plan.State(it)
	}
	if d.State != proxyroutes.StateError && !st.Written && st.LastError != "" {
		d.State, d.LastError = proxyroutes.StateError, st.LastError
	}
	if st.VerifiedAt.IsZero() {
		return d
	}
	d.CheckedAt = st.VerifiedAt.UTC().Format(time.RFC3339)
	d.Reachable = st.Reachable
	switch st.ProxyLoaded {
	case store.ProxyLoadedYes, store.ProxyLoadedNo:
		loaded := st.ProxyLoaded == store.ProxyLoadedYes
		d.ProxyLoaded = &loaded
	}
	if st.CertIssuer != "" {
		d.Certificate = &proxyCertificateResource{
			Issuer: st.CertIssuer, NotAfter: st.CertNotAfter.UTC().Format(time.RFC3339),
			Valid: st.CertTrusted && time.Now().Before(st.CertNotAfter),
		}
	}
	if d.LastError == "" {
		d.LastError = st.LastError
	}
	return d
}

func portOf(addr string) int {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	n, err := net.LookupPort("tcp", p)
	if err != nil {
		return 0
	}
	return n
}

func (rt *Router) ingressHTTPSPort() int {
	if rt.doctorHTTPSPort != 0 {
		return rt.doctorHTTPSPort
	}
	return defaultDoctorHTTPSPort
}
