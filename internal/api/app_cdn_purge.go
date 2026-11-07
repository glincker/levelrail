package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cdn"
)

const (
	envCDNPurge        = "APP_CDN_PURGE"
	envCDNPurgeTimeout = "APP_CDN_PURGE_TIMEOUT"
	// defaultCDNPurgeBudget bounds the whole purge so a slow provider
	// cannot hold a publish or rollback response.
	defaultCDNPurgeBudget = 20 * time.Second
)

// cdnZoneClient is what a DNS target's proxy handle offers beyond the proxied
// flag: cache purge by hostname and the zone's SSL mode.
type cdnZoneClient interface {
	PurgeHosts(ctx context.Context, zone string, hosts []string) error
	SSLMode(ctx context.Context, zone string) (string, error)
}

type cdnZone struct {
	client cdnZoneClient
	hosts  []string
}

func cdnPurgeBudget() time.Duration {
	if d, err := time.ParseDuration(os.Getenv(envCDNPurgeTimeout)); err == nil && d > 0 {
		return d
	}
	return defaultCDNPurgeBudget
}

// appCDNZones groups the app's domains by the zone whose provider can purge
// and read settings. It is empty unless the app's records are proxied.
func (rt *Router) appCDNZones(ctx context.Context, app string) (map[string]*cdnZone, error) {
	if rt.domains == nil || rt.ingressSettings == nil {
		return nil, nil
	}
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return nil, err
	}
	if !rt.appDNSProxied(ctx, app, s) {
		return nil, nil
	}
	all, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return nil, err
	}
	zones := map[string]*cdnZone{}
	var errs []error
	for _, d := range all {
		if d.ServiceName != app {
			continue
		}
		t, err := rt.resolveDNSTarget(ctx, d.Domain)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", d.Domain, err))
			continue
		}
		if t == nil {
			continue
		}
		c, ok := t.Proxy.(cdnZoneClient)
		if !ok {
			continue
		}
		z := zones[t.Zone]
		if z == nil {
			z = &cdnZone{client: c}
			zones[t.Zone] = z
		}
		z.hosts = append(z.hosts, strings.TrimSuffix(d.Domain, "."))
	}
	return zones, errors.Join(errs...)
}

// purgeAppCache clears the CDN edge cache for the app's domains. It is
// best effort: callers log and show the error, and never fail a publish on it.
func (rt *Router) purgeAppCache(ctx context.Context, app string) error {
	return rt.purgeAppCacheFor(ctx, app, cdn.TriggerPublish)
}

// purgeAppCacheFor is purgeAppCache that also records the attempt in the
// purge history under trigger, so the edge status can show what cleared it.
func (rt *Router) purgeAppCacheFor(ctx context.Context, app, trigger string) error {
	if strings.EqualFold(os.Getenv(envCDNPurge), "off") {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, cdnPurgeBudget())
	defer cancel()
	zones, err := rt.appCDNZones(ctx, app)
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}
	names := make([]string, 0, len(zones))
	for name := range zones {
		names = append(names, name)
	}
	sort.Strings(names)
	rec := cdn.PurgeRecord{At: time.Now().UTC(), Trigger: trigger, Scope: cdn.ScopeHosts, Zones: []string{}, OK: true}
	for _, name := range names {
		z := zones[name]
		rec.Provider = cdnProviderName(z.client)
		rec.Zones = append(rec.Zones, strings.TrimSuffix(name, "."))
		rec.Hosts += len(z.hosts)
		if err := z.client.PurgeHosts(ctx, name, z.hosts); err != nil {
			errs = append(errs, fmt.Errorf("zone %s: %w", strings.TrimSuffix(name, "."), err))
		}
	}
	if len(names) > 0 {
		rec.OK = len(errs) == 0
		if !rec.OK {
			rec.Error = errors.Join(errs...).Error()
		}
		cdn.DefaultHistory().Add(app, rec)
	}
	if len(errs) == 0 {
		return nil
	}
	// Only provider messages reach here, which never carry the token.
	err = fmt.Errorf("CDN cache purge failed, visitors may see the old site until it expires: %w", errors.Join(errs...))
	rt.logger.Warn("api: cdn purge failed", slog.String("app", app), slog.String("error", err.Error()))
	return err
}

// purgeWarning is purgeAppCache for response bodies: "" on success.
func (rt *Router) purgeWarning(ctx context.Context, app string) string {
	return rt.purgeWarningFor(ctx, app, cdn.TriggerPublish)
}

func (rt *Router) purgeWarningFor(ctx context.Context, app, trigger string) string {
	if err := rt.purgeAppCacheFor(ctx, app, trigger); err != nil {
		return err.Error()
	}
	return ""
}

// cdnSSLCheck reads the SSL mode of the zone behind the app's first proxied
// domain and returns it with a warning when it will break proxied traffic.
func (rt *Router) cdnSSLCheck(ctx context.Context, app string) (mode, warning string) {
	ctx, cancel := context.WithTimeout(ctx, cdnPurgeBudget())
	defer cancel()
	zones, _ := rt.appCDNZones(ctx, app)
	name := firstZone(zones)
	if name == "" {
		return "", ""
	}
	m, err := zones[name].client.SSLMode(ctx, name)
	if err != nil {
		return "", "could not read the Cloudflare SSL/TLS mode; the token needs Zone Settings read"
	}
	return m, sslModeWarning(m)
}

func firstZone(zones map[string]*cdnZone) string {
	first := ""
	for name := range zones {
		if first == "" || name < first {
			first = name
		}
	}
	return first
}

// sslModeWarning explains an SSL mode that fails against an origin serving
// HTTPS. Full is accepted; strict is the recommended one.
func sslModeWarning(mode string) string {
	switch mode {
	case "off", "flexible":
		return "Cloudflare SSL/TLS mode is " + mode + "; set it to Full (strict) or visitors may see redirect loops"
	case "full":
		return "Cloudflare SSL/TLS mode is Full; Full (strict) also checks the origin certificate"
	}
	return ""
}

func cdnProviderName(c cdnZoneClient) string {
	if n, ok := c.(interface{ Name() string }); ok && n.Name() != "" {
		return n.Name()
	}
	return "unknown"
}
