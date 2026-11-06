package proxyroutes

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Runtime setting key and env seed; the catch-all is opt in and off by default.
const (
	KeyUnroutedToLevelrail = "proxy.unrouted_to_levelrail"
	EnvUnroutedToLevelrail = "LEVELRAIL_PROXY_UNROUTED_TO_LEVELRAIL"
	// DefaultUnroutedPriority sits above the front proxy's own noop catch-all
	// and below every host router, so it never overrides another router.
	DefaultUnroutedPriority = -500
	unroutedName            = "unrouted"
)

var unroutedWarnOnce sync.Once

// UnroutedFileName is the managed catch-all file for the provider extension.
func (n Namespace) UnroutedFileName(ext string) string {
	return n.FilePrefix() + unroutedName + ext
}

// RenderUnrouted returns a Traefik file that sends any request no other router
// claimed to Levelrail's ingress, so it can show the diagnostic page.
func (n Namespace) RenderUnrouted(r Route) ([]byte, error) {
	for kind, name := range map[string]string{"HTTP entrypoint": r.EntrypointHTTP, "HTTPS entrypoint": r.EntrypointHTTPS} {
		if err := ValidateName(kind, name); err != nil {
			return nil, err
		}
	}
	if err := ValidateUpstream(r.Upstream); err != nil {
		return nil, err
	}
	if r.Priority >= 0 || r.Priority < -999 {
		return nil, fmt.Errorf("unrouted priority %d must be between -999 and -1", r.Priority)
	}
	obj := string(n) + "-" + unroutedName
	var b bytes.Buffer
	fmt.Fprintf(&b, "%s\n# Do not edit: rewritten on every reconcile. Turn off the unrouted setting to remove it.\n", n.Header())
	b.WriteString("http:\n  routers:\n")
	fmt.Fprintf(&b, "    %s:\n      rule: PathPrefix(`/`)\n      entryPoints: [%s]\n      service: %s\n      priority: %d\n      tls: {}\n", obj, r.EntrypointHTTPS, obj, r.Priority)
	fmt.Fprintf(&b, "    %s-http:\n      rule: PathPrefix(`/`)\n      entryPoints: [%s]\n      service: %s\n      priority: %d\n", obj, r.EntrypointHTTP, obj, r.Priority)
	fmt.Fprintf(&b, "  services:\n    %s:\n      loadBalancer:\n        passHostHeader: true\n        servers:\n          - url: http://%s\n", obj, r.Upstream)
	return b.Bytes(), nil
}

// unroutedFile is the opt in catch-all for this pass, or false when it is off,
// not Traefik, or cannot be rendered.
func (s *Syncer) unroutedFile(ctx context.Context, p Plan) (File, bool) {
	if s.Unrouted == nil || !s.Unrouted(ctx) {
		return File{}, false
	}
	if p.Provider.Kind() != KindTraefik {
		unroutedWarnOnce.Do(func() {
			if s.Logger != nil {
				s.Logger.Warn("proxyroutes: unrouted catch-all supports Traefik only, skipping", slog.String("provider", p.Provider.Kind()))
			}
		})
		return File{}, false
	}
	up, err := PathFromSettings(p.Settings.UpstreamHost, s.Unit).Resolve(s.Ingress)
	if err != nil {
		s.warn(ctx, "unrouted upstream", unroutedName, err)
		return File{}, false
	}
	prio := s.UnroutedPriority
	if prio == 0 {
		prio = DefaultUnroutedPriority
	}
	content, err := s.NS.RenderUnrouted(Route{
		EntrypointHTTP: p.Settings.EntrypointHTTP, EntrypointHTTPS: p.Settings.EntrypointHTTPS,
		Upstream: up, Priority: prio,
	})
	if err != nil {
		s.warn(ctx, "unrouted render", unroutedName, err)
		return File{}, false
	}
	return File{Domain: unroutedName, Name: s.NS.UnroutedFileName(p.Provider.Ext()), Content: content}, true
}
