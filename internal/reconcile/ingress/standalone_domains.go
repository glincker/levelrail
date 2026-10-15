package ingress

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/standalone"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

const standaloneRouteOwner = store.StandaloneDomainOwner

// StandaloneDomainStore is optional on ServiceStore, like TrafficPolicyStore:
// a store without it routes no standalone domains.
type StandaloneDomainStore interface {
	ListStandaloneDomains(ctx context.Context) ([]store.StandaloneDomain, error)
}

// domainFeatures are the per-domain settings every routed hostname honours,
// app owned or standalone.
type domainFeatures struct {
	auth        map[string]store.DomainBasicAuth
	maintenance map[string]bool
	redirects   map[string]store.DomainRedirect
	waf         map[string]store.DomainWAF
	errorPages  map[string][]store.DomainErrorPage
	hidden      map[string]bool
	// maintenanceSettings carries each maintenance host's page settings.
	maintenanceSettings map[string]store.DomainMaintenance
}

type standalonePlan struct {
	routes      []ingress.ProxyRoute
	maintenance []ingress.MaintenanceRoute
	redirects   []ingress.RedirectRoute
	// unrouted maps a domain to why it has no route this pass.
	unrouted map[string]string
}

// standaloneEgress allows loopback and private targets on purpose; metadata
// and link-local stay refused, re-checked every pass so a DNS change cannot
// slip one in after the API accepted the name.
var standaloneEgress = trafficpolicy.EgressGuard{AllowPrivate: true, AllowLoopback: true}

func (c *Controller) standaloneRoutes(ctx context.Context, services []store.DesiredService, ready map[string]bool, claimed map[string]string, f domainFeatures) (standalonePlan, error) {
	plan := standalonePlan{unrouted: map[string]string{}}
	src, ok := c.store.(StandaloneDomainStore)
	if !ok {
		return plan, nil
	}
	rows, err := src.ListStandaloneDomains(ctx)
	if err != nil {
		return plan, err
	}
	byName := make(map[string]*store.DesiredService, len(services))
	for i := range services {
		byName[services[i].Name] = &services[i]
	}
	for _, d := range rows {
		host := d.Domain
		if owner, _, dup := firstDuplicateHost(standaloneRouteOwner, []string{host}, claimed); dup {
			c.logger.WarnContext(ctx, "ingress: standalone domain is already routed to another resource, skipping",
				slog.String("domain", host), slog.String("already_routed_to", owner))
			plan.unrouted[host] = "already routed to " + owner
			continue
		}
		switch {
		case f.maintenance[host]:
			claimed[host] = standaloneRouteOwner
			plan.maintenance = append(plan.maintenance, ingress.MaintenanceRoute{Hosts: []string{host}, Page: maintenancePage(f.maintenanceSettings[host])})
			continue
		case f.redirects[host].TargetURL != "":
			r := f.redirects[host]
			claimed[host] = standaloneRouteOwner
			plan.redirects = append(plan.redirects, ingress.RedirectRoute{Hosts: []string{host}, TargetURL: r.TargetURL, StatusCode: r.StatusCode})
			continue
		}
		dial, up, err := c.standaloneUpstream(ctx, d, byName, ready)
		if err != nil {
			c.logger.WarnContext(ctx, "ingress: standalone domain target unavailable, not routed this pass",
				slog.String("domain", host), slog.String("target", d.Target), slog.String("error", err.Error()))
			plan.unrouted[host] = err.Error()
			continue
		}
		routes := c.routesForService(ctx, []string{host}, dial, f.auth, f.waf, f.errorPages, f.hidden)
		if len(routes) == 0 {
			plan.unrouted[host] = "basic auth password could not be resolved"
			continue
		}
		claimed[host] = standaloneRouteOwner
		for i := range routes {
			opts := up
			routes[i].Upstream = &opts
		}
		plan.routes = append(plan.routes, routes...)
	}
	return plan, nil
}

// standaloneUpstream resolves where d's traffic goes this pass.
func (c *Controller) standaloneUpstream(ctx context.Context, d store.StandaloneDomain, services map[string]*store.DesiredService, ready map[string]bool) (string, ingress.UpstreamOptions, error) {
	t, err := standalone.ParseTarget(d.Target)
	if err != nil {
		return "", ingress.UpstreamOptions{}, err
	}
	opts, err := standalone.Options{
		UpstreamTLS: d.UpstreamTLS, TLSSkipVerify: d.TLSSkipVerify, TLSServerName: d.TLSServerName,
		HostHeader: d.HostHeader, WebSocket: d.WebSocket, DialTimeoutSeconds: d.DialTimeoutSeconds,
		ReadTimeoutSeconds: d.ReadTimeoutSeconds, StripPrefix: d.StripPrefix,
	}.Validate(t)
	if err != nil {
		return "", ingress.UpstreamOptions{}, err
	}
	up := ingress.UpstreamOptions{
		TLS:                   t.TLS(opts.UpstreamTLS),
		InsecureSkipVerify:    opts.TLSSkipVerify,
		ServerName:            opts.TLSServerName,
		DisableWebSocket:      !opts.WebSocket,
		DialTimeout:           time.Duration(opts.DialTimeoutSeconds) * time.Second,
		ResponseHeaderTimeout: time.Duration(opts.ReadTimeoutSeconds) * time.Second,
		StripPrefix:           opts.StripPrefix,
		BasePath:              t.BasePath(),
	}
	dial, upstreamHost, err := c.standaloneDial(ctx, t, services, ready)
	if err != nil {
		return "", up, err
	}
	if up.TLS && up.ServerName == "" && upstreamHost != "" {
		if _, err := netip.ParseAddr(upstreamHost); err != nil {
			up.ServerName = upstreamHost
		}
	}
	switch mode := opts.EffectiveHostMode(t); mode {
	case standalone.HostPreserve:
	case standalone.HostUpstream:
		if t.Kind == standalone.KindURL {
			up.HostHeader = t.URL.Host
		} else {
			up.HostFromUpstream = true
		}
	default:
		up.HostHeader = mode
	}
	return dial, up, nil
}

// standaloneDial returns the address to dial and, for named targets, the
// name the upstream expects for SNI.
func (c *Controller) standaloneDial(ctx context.Context, t standalone.Target, services map[string]*store.DesiredService, ready map[string]bool) (dial, name string, err error) {
	switch t.Kind {
	case standalone.KindPort:
		return net.JoinHostPort(t.Host, strconv.Itoa(t.Port)), "", nil
	case standalone.KindAddress, standalone.KindURL:
		raw := "http://" + net.JoinHostPort(t.Host, strconv.Itoa(t.Port))
		resolved, err := c.policyTargets.resolve(ctx, standaloneEgress, net.DefaultResolver, raw, time.Now())
		if err != nil {
			return "", "", err
		}
		return resolved.Dial(), t.Host, nil
	case standalone.KindApp:
		svc, ok := services[t.App]
		if !ok {
			return "", "", fmt.Errorf("app %s does not exist", t.App)
		}
		if !c.isLocalNode(svc.NodeID) {
			return "", "", fmt.Errorf("app %s runs on another node; standalone domains reach apps on this node only", t.App)
		}
		if t.Port > 0 {
			if d, ok := c.dialForStreamPort(ctx, svc, t.Port); ok {
				return d, "", nil
			}
			return "", "", fmt.Errorf("app %s has no running container publishing port %d", t.App, t.Port)
		}
		if d, ok := c.dialForService(ctx, *svc, ready[svc.Name]); ok {
			return d, "", nil
		}
		return "", "", fmt.Errorf("app %s has no ready container", t.App)
	case standalone.KindContainer:
		return c.containerDial(ctx, t)
	}
	return "", "", errors.New("unknown target kind")
}

const (
	conditionStandaloneDomains = "StandaloneDomains"
	reasonTargetUnavailable    = "TargetUnavailable"
)

// condition reports standalone domains with no route this pass, nil when all route.
func (p standalonePlan) condition() *reconcile.Condition {
	if len(p.unrouted) == 0 {
		return nil
	}
	hosts := make([]string, 0, len(p.unrouted))
	for h := range p.unrouted {
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	parts := make([]string, 0, len(hosts))
	for _, h := range hosts {
		parts = append(parts, h+": "+p.unrouted[h])
	}
	return &reconcile.Condition{
		Type:    conditionStandaloneDomains,
		Status:  reconcile.ConditionFalse,
		Reason:  reasonTargetUnavailable,
		Message: fmt.Sprintf("%d standalone domain(s) are not routed: %s", len(hosts), strings.Join(parts, "; ")),
	}
}

func (c *Controller) containerDial(ctx context.Context, t standalone.Target) (string, string, error) {
	state, err := c.runtime.InspectByName(ctx, t.Container)
	if err != nil {
		return "", "", fmt.Errorf("inspect container %s: %w", t.Container, err)
	}
	if state == nil || !state.Running {
		return "", "", fmt.Errorf("container %s is not running", t.Container)
	}
	for _, p := range state.Ports {
		if p.ContainerPort == t.Port && p.HostPort > 0 {
			return "127.0.0.1:" + strconv.Itoa(p.HostPort), "", nil
		}
	}
	for _, n := range state.Networks {
		addr, err := netip.ParseAddr(n.IPAddress)
		if err != nil {
			continue
		}
		if err := standalone.CheckAddr(addr); err != nil {
			return "", "", err
		}
		return net.JoinHostPort(addr.String(), strconv.Itoa(t.Port)), "", nil
	}
	return "", "", fmt.Errorf("container %s has no published port %d and no network address", t.Container, t.Port)
}
