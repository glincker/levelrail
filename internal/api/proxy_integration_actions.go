package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// defaultProxyVerifyTimeout bounds each probe when SetProxyIntegration was
// given no timeout (cmd/levelrail reads APP_PROXY_VERIFY_TIMEOUT).
const defaultProxyVerifyTimeout = 8 * time.Second

// Conflict codes returned by setup.
const (
	proxyCodeNotDetected = "proxy_not_detected"
	proxyCodeUnsupported = "proxy_unsupported"
	proxyCodeIncomplete  = "detection_incomplete"
	proxyCodeDirInvalid  = "dynamic_dir_invalid"
	proxyCodeDisabled    = "integration_off"
)

type proxyConflict struct {
	Error   string   `json:"error"`
	Code    string   `json:"code"`
	Missing []string `json:"missing,omitempty"`
	Fix     string   `json:"fix,omitempty"`
}

type proxySetupRequest struct {
	Confirm    bool   `json:"confirm"`
	DynamicDir string `json:"dynamic_dir"`
}

type proxySetupResponse struct {
	proxyIntegrationResource
	DryRun  bool     `json:"dry_run"`
	Changes []string `json:"changes"`
}

func (deps *proxyIntegrationDeps) timeout() time.Duration {
	if deps.verifyTimeout > 0 {
		return deps.verifyTimeout
	}
	return defaultProxyVerifyTimeout
}

// handleUpdateProxyIntegrationSettings handles PUT /api/v1/settings/proxy-integration.
func (rt *Router) handleUpdateProxyIntegrationSettings(w http.ResponseWriter, r *http.Request) {
	deps, ok := rt.proxyIntegrationReady(w)
	if !ok {
		return
	}
	var body proxySettingsResource
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	s := store.ProxyIntegrationSettings{
		Mode: strings.TrimSpace(body.Integration), DynamicDir: strings.TrimSpace(body.DynamicDir),
		EntrypointHTTP: strings.TrimSpace(body.EntrypointHTTP), EntrypointHTTPS: strings.TrimSpace(body.EntrypointHTTPS),
		CertResolver: strings.TrimSpace(body.CertResolver), UpstreamHost: strings.TrimSpace(body.UpstreamHost),
	}
	if err := rt.validateProxySettings(s); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := deps.store.UpdateProxyIntegrationSettings(r.Context(), s); err != nil {
		rt.internalError(w, "proxy integration: update settings", err)
		return
	}
	rt.syncProxyRoutes(r.Context(), deps)
	writeJSON(w, http.StatusOK, toProxySettingsResource(s))
}

func (rt *Router) validateProxySettings(s store.ProxyIntegrationSettings) error {
	if !store.ValidProxyIntegrationMode(s.Mode) {
		return errors.New("integration must be off or traefik_file")
	}
	if s.DynamicDir != "" && !filepath.IsAbs(s.DynamicDir) {
		return fmt.Errorf("dynamic_dir %q must be an absolute path", s.DynamicDir)
	}
	if !s.Enabled() {
		return nil
	}
	if s.DynamicDir == "" {
		return errors.New("dynamic_dir is required when the integration is on")
	}
	if _, err := proxyroutes.OpenDir(rt.proxyIntegration.syncer.NS, s.DynamicDir); err != nil {
		return err
	}
	if err := proxyroutes.ValidateName("entrypoint_http", s.EntrypointHTTP); err != nil {
		return err
	}
	if err := proxyroutes.ValidateName("entrypoint_https", s.EntrypointHTTPS); err != nil {
		return err
	}
	if s.CertResolver != "" {
		if err := proxyroutes.ValidateName("cert_resolver", s.CertResolver); err != nil {
			return err
		}
	}
	return proxyroutes.ValidateUpstreamHost(s.UpstreamHost)
}

func (rt *Router) syncProxyRoutes(ctx context.Context, deps *proxyIntegrationDeps) {
	if _, err := deps.syncer.Sync(ctx); err != nil {
		rt.logger.WarnContext(ctx, "proxy integration: sync", "error", err.Error())
	}
	rt.nudgeReconciler()
}

// handleApplyProxyIntegration handles POST /api/v1/system/proxy-integration/apply.
func (rt *Router) handleApplyProxyIntegration(w http.ResponseWriter, r *http.Request) {
	deps, ok := rt.proxyIntegrationReady(w)
	if !ok {
		return
	}
	settings, err := deps.store.GetProxyIntegrationSettings(r.Context())
	if err != nil {
		rt.internalError(w, "proxy integration: settings", err)
		return
	}
	if !settings.Enabled() {
		writeJSON(w, http.StatusConflict, proxyConflict{Code: proxyCodeDisabled, Error: "managed proxy routes are off: run setup first"})
		return
	}
	rt.syncProxyRoutes(r.Context(), deps)
	rt.respondProxyIntegration(r.Context(), w, deps, rt.detectProxy(r.Context()))
}

// handleVerifyProxyIntegration handles POST /api/v1/system/proxy-integration/verify[?domain=].
func (rt *Router) handleVerifyProxyIntegration(w http.ResponseWriter, r *http.Request) {
	deps, ok := rt.proxyIntegrationReady(w)
	if !ok {
		return
	}
	det := rt.detectProxy(r.Context())
	domain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	found, err := rt.verifyProxyRoutes(r.Context(), deps, det, domain)
	if err != nil {
		rt.internalError(w, "proxy integration: verify", err)
		return
	}
	if domain != "" && !found {
		writeError(w, http.StatusNotFound, "domain "+domain+" is not routed by this instance")
		return
	}
	rt.respondProxyIntegration(r.Context(), w, deps, det)
}

func (rt *Router) respondProxyIntegration(ctx context.Context, w http.ResponseWriter, deps *proxyIntegrationDeps, det proxyroutes.Detection) {
	res, err := rt.buildProxyIntegration(ctx, deps, det)
	if err != nil {
		rt.internalError(w, "proxy integration: build", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// verifyProxyRoutes probes every written route, or only domain, through the
// public address and records the outcome. found reports whether domain is routed.
func (rt *Router) verifyProxyRoutes(ctx context.Context, deps *proxyIntegrationDeps, det proxyroutes.Detection, domain string) (bool, error) {
	plan, err := deps.syncer.Plan(ctx)
	if err != nil {
		return false, err
	}
	ingress, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return false, err
	}
	host := rt.publicHost
	if host == "" {
		host = "127.0.0.1"
	}
	port := ingress.PublicLinkPort()
	if port == 0 {
		port = det.PublicHTTPSPort
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	found := false
	var wg sync.WaitGroup
	for _, it := range plan.Items {
		if domain != "" && it.Domain != domain {
			continue
		}
		found = true
		if state, _ := plan.State(it); state != proxyroutes.StateWritten || port == 0 {
			continue
		}
		wg.Add(1)
		go func(it proxyroutes.Item) {
			defer wg.Done()
			rt.verifyProxyRoute(ctx, deps, det, it, addr)
		}(it)
	}
	wg.Wait()
	return found, nil
}

func (rt *Router) verifyProxyRoute(ctx context.Context, deps *proxyIntegrationDeps, det proxyroutes.Detection, it proxyroutes.Item, addr string) {
	path := "/"
	if it.Kind == proxyroutes.TargetDashboard {
		path = "/healthz"
	}
	res := deps.probe(ctx, proxyroutes.ProbeTarget{Domain: it.Domain, Address: addr, Path: path, Timeout: deps.timeout()})
	st := store.ProxyRouteStatus{
		Domain: it.Domain, ProxyLoaded: store.ProxyLoadedUnknown, Reachable: res.Reachable, StatusCode: res.StatusCode,
		CertIssuer: res.CertIssuer, CertNotAfter: res.CertNotAfter, CertTrusted: res.CertTrusted, LastError: res.Err, VerifiedAt: time.Now(),
	}
	if res.Reachable && det.APIPort > 0 {
		base := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(det.APIPort))
		if loaded, err := deps.routerLoaded(ctx, base, deps.syncer.NS.RouterName(it.Domain), deps.timeout()); err == nil {
			st.ProxyLoaded = store.ProxyLoadedNo
			if loaded {
				st.ProxyLoaded = store.ProxyLoadedYes
			}
		}
	}
	if err := deps.store.SaveProxyRouteVerification(ctx, st); err != nil {
		rt.logger.WarnContext(ctx, "proxy integration: save verification", "domain", it.Domain, "error", err.Error())
	}
}
