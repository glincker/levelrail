package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Provider choices the picker offers besides the external proxy kinds.
const (
	proxyProviderEmbedded = "embedded"
	proxyProviderAuto     = "auto"

	proxyModeBuiltin = "builtin"
	proxyModeManaged = "managed"
	proxyModeManual  = "manual"
)

// Conflict codes for provider selection and a proxy refusing written files.
const (
	proxyCodeUnknownProvider  = "unknown_provider"
	proxyCodeProviderMismatch = "provider_mismatch"
	proxyCodeRejected         = "proxy_rejected"
)

type proxyProviderResource struct {
	Kind string `json:"kind"`
	// Mode is builtin (the embedded ingress), managed (files written and
	// reloaded here) or manual (a snippet to apply by hand).
	Mode     string `json:"mode"`
	Detected bool   `json:"detected"`
	Selected bool   `json:"selected"`
}

// selectedProxyProvider is the provider the saved mode turns on.
func selectedProxyProvider(s store.ProxyIntegrationSettings) string {
	if p, ok := proxyroutes.ProviderForMode(s.Mode); ok {
		return p.Kind()
	}
	return proxyProviderEmbedded
}

func proxyProviders(det proxyroutes.Detection, settings store.ProxyIntegrationSettings) []proxyProviderResource {
	selected := selectedProxyProvider(settings)
	out := []proxyProviderResource{{Kind: proxyProviderEmbedded, Mode: proxyModeBuiltin, Selected: selected == proxyProviderEmbedded}}
	for _, p := range proxyroutes.Providers() {
		mode := proxyModeManual
		if proxyroutes.Managed(p) {
			mode = proxyModeManaged
		}
		out = append(out, proxyProviderResource{Kind: p.Kind(), Mode: mode, Detected: det.Kind == p.Kind(), Selected: selected == p.Kind()})
	}
	return out
}

// resolveProxyProvider picks the provider a setup request asks for. auto
// follows detection; a managed provider must match what holds the ports.
func resolveProxyProvider(det proxyroutes.Detection, want string) (proxyroutes.Provider, *proxyConflict) {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" || want == proxyProviderAuto {
		want = det.Kind
	}
	if want == proxyroutes.KindNone {
		return nil, &proxyConflict{Code: proxyCodeNotDetected, Error: "no reverse proxy was detected on this host", Missing: det.Missing}
	}
	p, ok := proxyroutes.ProviderFor(want)
	if !ok {
		return nil, &proxyConflict{Code: proxyCodeUnknownProvider, Error: fmt.Sprintf("provider %q is not one of traefik, nginx, caddy, haproxy or auto", want)}
	}
	if proxyroutes.Managed(p) && det.Kind != p.Kind() {
		return nil, &proxyConflict{Code: proxyCodeProviderMismatch, Missing: det.Missing,
			Error: fmt.Sprintf("%s was asked for, but the proxy on this host is %s", p.Kind(), det.Kind)}
	}
	return p, nil
}

// manualProxySnippet renders every routed domain for a provider that is
// applied by hand. Nothing is written anywhere.
func (rt *Router) manualProxySnippet(ctx context.Context, deps *proxyIntegrationDeps, prov proxyroutes.Provider, det proxyroutes.Detection) (string, *proxyConflict, error) {
	plan, err := deps.syncer.PlanFor(ctx, store.ProxyIntegrationSettings{UpstreamHost: det.UpstreamHost}, prov)
	if err != nil {
		return "", nil, err
	}
	var parts, problems []string
	for _, it := range plan.Items {
		if it.Err != nil {
			problems = append(problems, it.Domain+": "+it.Err.Error())
			continue
		}
		parts = append(parts, "# "+it.Domain+"\n"+string(it.Content))
	}
	if len(parts) == 0 && len(problems) > 0 {
		return "", &proxyConflict{Code: proxyCodeIncomplete, Missing: problems,
			Error: "cannot render a snippet: " + strings.Join(problems, "; ") + " (set upstream_host)"}, nil
	}
	return strings.Join(parts, "\n"), nil, nil
}

// proxyRouteFileChanges lists the files setup would write or replace.
func (rt *Router) proxyRouteFileChanges(ctx context.Context, deps *proxyIntegrationDeps, prov proxyroutes.Provider, s store.ProxyIntegrationSettings) ([]string, error) {
	plan, err := deps.syncer.PlanFor(ctx, s, prov)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, it := range plan.Items {
		switch state, detail := plan.State(it); state {
		case proxyroutes.StateMissing:
			out = append(out, "write "+it.File)
		case proxyroutes.StateStale:
			out = append(out, "rewrite "+it.File)
		case proxyroutes.StateError:
			out = append(out, "skip "+it.Domain+": "+detail)
		}
	}
	return out, nil
}

// rejectedProxy returns the conflict for a proxy that refused written files.
func rejectedProxy(err error) *proxyConflict {
	var rej *proxyroutes.RejectedError
	if !errors.As(err, &rej) {
		return nil
	}
	return &proxyConflict{Code: proxyCodeRejected, Error: "the proxy rejected the new routes and they were rolled back: " + rej.Error()}
}
