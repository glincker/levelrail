package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// proxySetupPlan is everything setup would change, computed without writing.
type proxySetupPlan struct {
	det      proxyroutes.Detection
	settings store.ProxyIntegrationSettings
	ingress  store.IngressSettings
	changes  []string
}

// handleSetupProxyIntegration handles POST /api/v1/system/proxy-integration/setup:
// detect, then (with confirm) switch TLS upstream on, save the detected
// settings, write the routes and verify them. Repeating it changes nothing.
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
	deps.setupMu.Lock()
	defer deps.setupMu.Unlock()
	ctx := r.Context()
	plan, conflict, err := rt.planProxySetup(ctx, deps, body.DynamicDir)
	if err != nil {
		rt.internalError(w, "proxy integration: setup plan", err)
		return
	}
	if conflict != nil {
		writeJSON(w, http.StatusConflict, conflict)
		return
	}
	if body.Confirm {
		if err := rt.ingressSettings.UpdateIngressSettings(ctx, plan.ingress); err != nil {
			rt.internalError(w, "proxy integration: ingress settings", err)
			return
		}
		if err := deps.store.UpdateProxyIntegrationSettings(ctx, plan.settings); err != nil {
			rt.internalError(w, "proxy integration: settings", err)
			return
		}
		rt.syncProxyRoutes(ctx, deps)
		if _, err := rt.verifyProxyRoutes(ctx, deps, plan.det, ""); err != nil {
			rt.internalError(w, "proxy integration: verify", err)
			return
		}
	}
	res, err := rt.buildProxyIntegration(ctx, deps, plan.det)
	if err != nil {
		rt.internalError(w, "proxy integration: build", err)
		return
	}
	writeJSON(w, http.StatusOK, proxySetupResponse{proxyIntegrationResource: res, DryRun: !body.Confirm, Changes: plan.changes})
}

func (rt *Router) planProxySetup(ctx context.Context, deps *proxyIntegrationDeps, dirOverride string) (proxySetupPlan, *proxyConflict, error) {
	det := rt.detectProxy(ctx)
	if dirOverride != "" && det.Kind == proxyroutes.KindTraefik {
		det.OverrideDir(dirOverride)
	}
	switch {
	case det.Kind == proxyroutes.KindNone:
		return proxySetupPlan{}, &proxyConflict{Code: proxyCodeNotDetected, Error: "no reverse proxy was detected on this host", Missing: det.Missing}, nil
	case det.Kind != proxyroutes.KindTraefik:
		return proxySetupPlan{}, &proxyConflict{Code: proxyCodeUnsupported, Missing: det.Missing,
			Error: det.Kind + " in container " + det.Container + " owns the web ports; managed routes support Traefik only, use the proxy guide for a snippet"}, nil
	case !det.Complete:
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
	dir, err := proxyroutes.OpenDir(deps.syncer.NS, det.DynamicDir)
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
	p := proxySetupPlan{det: det, ingress: ingress, settings: store.ProxyIntegrationSettings{
		Mode: store.ProxyIntegrationTraefikFile, DynamicDir: det.DynamicDir, EntrypointHTTP: det.EntrypointHTTP,
		EntrypointHTTPS: det.EntrypointHTTPS, CertResolver: det.CertResolver, UpstreamHost: det.UpstreamHost,
	}}
	p.ingress.TLSTerminatedUpstream = true
	p.ingress.PublicHTTPSPort = det.PublicHTTPSPort
	p.changes = setupChanges(current, p.settings, ingress, p.ingress)
	return p, nil, nil
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
	return out
}
