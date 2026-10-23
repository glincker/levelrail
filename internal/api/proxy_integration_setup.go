package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// proxySetupPlan is everything setup would change, computed without writing.
type proxySetupPlan struct {
	det      proxyroutes.Detection
	prov     proxyroutes.Provider
	current  store.ProxyIntegrationSettings
	settings store.ProxyIntegrationSettings
	ingress  store.IngressSettings
	changes  []string
	snippet  string
}

// handleSetupProxyIntegration handles POST /api/v1/system/proxy-integration/setup:
// detect, then (with confirm) save the detected settings, write the routes,
// switch TLS upstream on and verify them. Repeating it changes nothing.
// Caddy and HAProxy only ever return a snippet.
func (rt *Router) handleSetupProxyIntegration(w http.ResponseWriter, r *http.Request) {
	deps, ok := rt.proxyIntegrationReady(w)
	if !ok {
		return
	}
	var body proxySetupRequest
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	body.DynamicDir = strings.TrimSpace(body.DynamicDir)
	if body.DynamicDir != "" && !filepath.IsAbs(body.DynamicDir) {
		writeError(w, http.StatusBadRequest, "dynamic_dir must be an absolute path")
		return
	}
	body.CertDir = strings.TrimSpace(body.CertDir)
	if body.CertDir != "" {
		if err := proxyroutes.ValidateNginxPath("cert_dir", body.CertDir); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	deps.setupMu.Lock()
	defer deps.setupMu.Unlock()
	ctx := r.Context()
	plan, conflict, err := rt.planProxySetup(ctx, deps, body)
	if err != nil {
		rt.internalError(w, "proxy integration: setup plan", err)
		return
	}
	if conflict != nil {
		writeJSON(w, http.StatusConflict, conflict)
		return
	}
	manual := !proxyroutes.Managed(plan.prov)
	if body.Confirm && !manual {
		if c := rt.applyProxySetup(ctx, deps, plan); c != nil {
			writeJSON(w, http.StatusConflict, c)
			return
		}
	}
	res, err := rt.buildProxyIntegration(ctx, deps, plan.det)
	if err != nil {
		rt.internalError(w, "proxy integration: build", err)
		return
	}
	writeJSON(w, http.StatusOK, proxySetupResponse{
		proxyIntegrationResource: res, DryRun: !body.Confirm || manual, Changes: plan.changes,
		Manual: manual, Snippet: plan.snippet,
	})
}

// applyProxySetup saves the proxy settings and writes the routes first, so a
// proxy that rejects them leaves TLS handling and settings as they were.
func (rt *Router) applyProxySetup(ctx context.Context, deps *proxyIntegrationDeps, plan proxySetupPlan) *proxyConflict {
	if err := deps.store.UpdateProxyIntegrationSettings(ctx, plan.settings); err != nil {
		return &proxyConflict{Code: proxyCodeDirInvalid, Error: "save proxy settings: " + err.Error()}
	}
	if c := rt.syncOrRevertProxyRoutes(ctx, deps, plan.current); c != nil {
		return c
	}
	if err := rt.ingressSettings.UpdateIngressSettings(ctx, plan.ingress); err != nil {
		return &proxyConflict{Code: proxyCodeDisabled, Error: "save ingress settings: " + err.Error()}
	}
	ingressdriver.DefaultACMEFailures().ClearAll()
	if _, err := rt.verifyProxyRoutes(ctx, deps, plan.det, ""); err != nil {
		rt.logger.WarnContext(ctx, "proxy integration: verify after setup", "error", err.Error())
	}
	return nil
}

func (rt *Router) planProxySetup(ctx context.Context, deps *proxyIntegrationDeps, req proxySetupRequest) (proxySetupPlan, *proxyConflict, error) {
	det := rt.detectProxy(ctx)
	prov, conflict := resolveProxyProvider(det, req.Provider)
	if conflict != nil {
		return proxySetupPlan{}, conflict, nil
	}
	if !proxyroutes.Managed(prov) {
		snippet, c, err := rt.manualProxySnippet(ctx, deps, prov, det)
		return proxySetupPlan{det: det, prov: prov, snippet: snippet, changes: []string{}}, c, err
	}
	if req.DynamicDir != "" {
		det.OverrideDir(req.DynamicDir)
	}
	if req.CertDir != "" && prov.Kind() == proxyroutes.KindNginx {
		det.OverrideCertDir(req.CertDir)
	}
	if prov.Kind() == proxyroutes.KindNginx && det.Complete && det.CertDir == "" {
		det.Complete, det.Missing = false, append(det.Missing, "no certificate directory for nginx (cert_dir)")
	}
	if !det.Complete {
		return proxySetupPlan{}, &proxyConflict{Code: proxyCodeIncomplete, Missing: det.Missing,
			Error: "detection is incomplete: " + strings.Join(det.Missing, "; ")}, nil
	}
	ingress, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return proxySetupPlan{}, nil, err
	}
	if c := rt.checkSetupUpstreams(deps, det, ingress.PrimaryDomain != ""); c != nil {
		return proxySetupPlan{}, c, nil
	}
	dir, err := proxyroutes.OpenDirExt(deps.syncer.NS, det.DynamicDir, prov.Ext())
	if err != nil {
		return proxySetupPlan{}, &proxyConflict{Code: proxyCodeDirInvalid, Error: err.Error()}, nil
	}
	if err := dir.CheckWritable(); err != nil {
		var nw *proxyroutes.NotWritableError
		if errors.As(err, &nw) {
			return proxySetupPlan{}, &proxyConflict{Code: proxyroutes.NotWritableCode, Error: nw.Reason}, nil
		}
		return proxySetupPlan{}, &proxyConflict{Code: proxyCodeDirInvalid, Error: err.Error()}, nil
	}
	current, err := deps.store.GetProxyIntegrationSettings(ctx)
	if err != nil {
		return proxySetupPlan{}, nil, err
	}
	p := proxySetupPlan{det: det, prov: prov, current: current, ingress: ingress, settings: setupSettings(prov, det)}
	p.ingress.TLSTerminatedUpstream = true
	p.ingress.PublicHTTPSPort = det.PublicHTTPSPort
	p.changes = setupChanges(current, p.settings, ingress, p.ingress)
	files, err := rt.proxyRouteFileChanges(ctx, deps, prov, p.settings)
	if err != nil {
		return proxySetupPlan{}, nil, err
	}
	p.changes = append(p.changes, files...)
	return p, nil, nil
}

func setupSettings(prov proxyroutes.Provider, det proxyroutes.Detection) store.ProxyIntegrationSettings {
	s := store.ProxyIntegrationSettings{Mode: prov.Mode(), DynamicDir: det.DynamicDir, UpstreamHost: det.UpstreamHost}
	switch prov.Kind() {
	case proxyroutes.KindTraefik:
		s.EntrypointHTTP, s.EntrypointHTTPS, s.CertResolver = det.EntrypointHTTP, det.EntrypointHTTPS, det.CertResolver
	case proxyroutes.KindNginx:
		s.Container, s.CertDir, s.Webroot = det.Container, det.CertDir, det.Webroot
	}
	return s
}

func (rt *Router) checkSetupUpstreams(deps *proxyIntegrationDeps, det proxyroutes.Detection, dashboard bool) *proxyConflict {
	path := proxyroutes.UpstreamPath{Host: det.UpstreamHost, IP: det.UpstreamIP, HostNetwork: det.HostNetwork, Unit: deps.syncer.Unit}
	listeners := []proxyroutes.Listener{deps.syncer.Ingress}
	if dashboard {
		listeners = append(listeners, deps.syncer.Dashboard)
	}
	for _, l := range listeners {
		if _, err := path.Resolve(l); err != nil {
			var ue *proxyroutes.UnreachableError
			if errors.As(err, &ue) {
				msg := ue.Reason
				if ue.Fix != "" {
					msg += ". Fix: " + ue.Fix
				}
				return &proxyConflict{Code: proxyroutes.UpstreamUnreachableCode, Error: msg, Fix: ue.Fix}
			}
			return &proxyConflict{Code: proxyroutes.UpstreamUnreachableCode, Error: err.Error()}
		}
	}
	return nil
}

func setupChanges(cur, next store.ProxyIntegrationSettings, curIngress, nextIngress store.IngressSettings) []string {
	out := []string{}
	add := func(field string, from, to any) {
		if fmt.Sprint(from) != fmt.Sprint(to) {
			out = append(out, fmt.Sprintf("%s: %v -> %v", field, from, to))
		}
	}
	add("tls_terminated_upstream", curIngress.TLSTerminatedUpstream, nextIngress.TLSTerminatedUpstream)
	add("public_https_port", curIngress.PublicHTTPSPort, nextIngress.PublicHTTPSPort)
	add("integration", cur.Mode, next.Mode)
	add("dynamic_dir", cur.DynamicDir, next.DynamicDir)
	add("entrypoint_http", cur.EntrypointHTTP, next.EntrypointHTTP)
	add("entrypoint_https", cur.EntrypointHTTPS, next.EntrypointHTTPS)
	add("cert_resolver", cur.CertResolver, next.CertResolver)
	add("upstream_host", cur.UpstreamHost, next.UpstreamHost)
	add("container", cur.Container, next.Container)
	add("cert_dir", cur.CertDir, next.CertDir)
	add("webroot", cur.Webroot, next.Webroot)
	return out
}
