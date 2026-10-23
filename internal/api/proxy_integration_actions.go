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

// proxyNotDeliveredMessage explains a route that answers without reaching the
// built-in ingress, which is what a competing router on the proxy looks like.
const proxyNotDeliveredMessage = "the proxy answers for this domain, but not through this server's ingress: another router on the proxy probably claims the same host with a higher priority, so traffic controls are not enforced"

// listenerProbeTimeout bounds the local dial that explains a 5xx from the proxy.
const listenerProbeTimeout = time.Second

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
	// Provider is traefik, nginx, caddy, haproxy or auto (the default).
	Provider string `json:"provider"`
	// CertDir overrides the nginx certificate directory (in the container).
	CertDir string `json:"cert_dir"`
}

type proxySetupResponse struct {
	proxyIntegrationResource
	DryRun  bool     `json:"dry_run"`
	Changes []string `json:"changes"`
	// Manual and Snippet are set for caddy and haproxy: configuration to
	// apply by hand, never applied here.
	Manual  bool   `json:"manual,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

const defaultProxyRouteRetry = 500 * time.Millisecond

func (deps *proxyIntegrationDeps) routeRetry() time.Duration {
	if deps.routeRetryInterval > 0 {
		return deps.routeRetryInterval
	}
	return defaultProxyRouteRetry
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
		Container: strings.TrimSpace(body.Container), CertDir: strings.TrimSpace(body.CertDir), Webroot: strings.TrimSpace(body.Webroot),
	}
	if err := rt.validateProxySettings(s); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	previous, err := deps.store.GetProxyIntegrationSettings(r.Context())
	if err != nil {
		rt.internalError(w, "proxy integration: settings", err)
		return
	}
	if err := deps.store.UpdateProxyIntegrationSettings(r.Context(), s); err != nil {
		rt.internalError(w, "proxy integration: update settings", err)
		return
	}
	if c := rt.syncOrRevertProxyRoutes(r.Context(), deps, previous); c != nil {
		writeJSON(w, http.StatusConflict, c)
		return
	}
	writeJSON(w, http.StatusOK, toProxySettingsResource(s))
}

func (rt *Router) validateProxySettings(s store.ProxyIntegrationSettings) error {
	if !store.ValidProxyIntegrationMode(s.Mode) {
		return errors.New("integration must be off, traefik_file or nginx_file")
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
	if s.Mode == store.ProxyIntegrationNginxFile {
		return validateNginxSettings(s)
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

func validateNginxSettings(s store.ProxyIntegrationSettings) error {
	if s.Container == "" {
		return errors.New("container is required for nginx: it is the container that is validated and reloaded")
	}
	if s.CertDir == "" {
		return errors.New("cert_dir is required for nginx")
	}
	if err := proxyroutes.ValidateNginxPath("cert_dir", s.CertDir); err != nil {
		return err
	}
	if s.Webroot != "" {
		if err := proxyroutes.ValidateNginxPath("webroot", s.Webroot); err != nil {
			return err
		}
	}
	return proxyroutes.ValidateUpstreamHost(s.UpstreamHost)
}

// syncProxyRoutes runs one sync pass. A failure other than the proxy
// rejecting the files is logged and left to the next pass.
func (rt *Router) syncProxyRoutes(ctx context.Context, deps *proxyIntegrationDeps) error {
	_, err := deps.syncer.Sync(ctx)
	if err != nil {
		rt.logger.WarnContext(ctx, "proxy integration: sync", "error", err.Error())
	}
	rt.nudgeReconciler()
	var rej *proxyroutes.RejectedError
	if errors.As(err, &rej) {
		return err
	}
	return nil
}

// syncOrRevertProxyRoutes syncs and, when the proxy rejects the files,
// puts previous back so a bad change never stays saved.
func (rt *Router) syncOrRevertProxyRoutes(ctx context.Context, deps *proxyIntegrationDeps, previous store.ProxyIntegrationSettings) *proxyConflict {
	err := rt.syncProxyRoutes(ctx, deps)
	if err == nil {
		return nil
	}
	if uerr := deps.store.UpdateProxyIntegrationSettings(ctx, previous); uerr != nil {
		rt.logger.ErrorContext(ctx, "proxy integration: restore settings", "error", uerr.Error())
	}
	return rejectedProxy(err)
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
	if err := rt.syncProxyRoutes(r.Context(), deps); err != nil {
		writeJSON(w, http.StatusConflict, rejectedProxy(err))
		return
	}
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
	target := proxyroutes.ProbeTarget{Domain: it.Domain, Address: addr, Path: path, Timeout: deps.timeout()}
	res := deps.probe(ctx, target)
	// The proxy loads a written file asynchronously, so its "no router" 404
	// right after a write is a wait, not a failure.
	retry := deps.routeRetry()
	deadline := time.Now().Add(deps.timeout())
	for res.NoRoute && time.Now().Add(retry).Before(deadline) {
		select {
		case <-ctx.Done():
		case <-time.After(retry):
		}
		if ctx.Err() != nil {
			break
		}
		res = deps.probe(ctx, target)
	}
	if res.NoRoute {
		res.Err = proxyroutes.WaitingForRoute
	}
	if !res.NoRoute && (!res.Reachable || res.StatusCode >= http.StatusInternalServerError) {
		l := deps.syncer.Ingress
		if it.Kind == proxyroutes.TargetDashboard {
			l = deps.syncer.Dashboard
		}
		if msg := listenerDownMessage(l); msg != "" {
			res.Err = msg
		}
	}
	st := store.ProxyRouteStatus{
		Domain: it.Domain, ProxyLoaded: store.ProxyLoadedUnknown, Reachable: res.Reachable, StatusCode: res.StatusCode,
		CertIssuer: res.CertIssuer, CertNotAfter: res.CertNotAfter, CertTrusted: res.CertTrusted, LastError: res.Err, VerifiedAt: time.Now(),
	}
	if res.Reachable && it.Kind != proxyroutes.TargetDashboard && deps.marker != nil {
		marked := target
		marked.Marker = deps.marker
		if st.Delivered = deps.probe(ctx, marked).Delivered; !st.Delivered && st.LastError == "" {
			st.LastError = proxyNotDeliveredMessage
		}
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

// listenerDownMessage names the missing local listener when the proxy got a
// 5xx, so the operator sees the cause instead of a bare upstream status.
func listenerDownMessage(l proxyroutes.Listener) string {
	addr := l.Addr
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	}
	if addr == "" {
		return ""
	}
	c, err := (&net.Dialer{Timeout: listenerProbeTimeout}).DialContext(context.Background(), "tcp", addr)
	if err == nil {
		_ = c.Close()
		return ""
	}
	return fmt.Sprintf("nothing is listening on the %s port %s: wait for the ingress to reconcile, or check %s", l.Label, l.Addr, l.EnvVar)
}
