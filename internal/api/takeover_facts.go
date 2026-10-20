package api

import (
	"context"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/proxycoexist"
	"github.com/GLINCKER/levelrail/internal/serverready"
	"github.com/GLINCKER/levelrail/internal/takeover"
)

// takeoverDomains lists every domain this server serves with its app and health.
func (rt *Router) takeoverDomains(ctx context.Context) ([]takeover.DomainFact, error) {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}
	var out []takeover.DomainFact
	seen := map[string]bool{}
	for _, s := range svcs {
		for _, d := range s.Domains {
			key := strings.ToLower(d)
			if seen[key] {
				continue
			}
			seen[key] = true
			f := takeover.DomainFact{Domain: key, App: s.Name}
			switch {
			case s.Suspended:
				f.HealthDetail = "stopped"
			default:
				f.Healthy, f.HealthDetail = rt.appReadiness(ctx, s)
			}
			out = append(out, f)
		}
	}
	if ing, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil && ing.PrimaryDomain != "" && !seen[strings.ToLower(ing.PrimaryDomain)] {
		out = append(out, takeover.DomainFact{Domain: strings.ToLower(ing.PrimaryDomain), App: "dashboard", Healthy: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out, nil
}

func (rt *Router) takeoverDomainNames(ctx context.Context) ([]string, error) {
	doms, err := rt.takeoverDomains(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(doms))
	for i, d := range doms {
		names[i] = d.Domain
	}
	return names, nil
}

func (rt *Router) takeoverFacts(ctx context.Context, acceptUnmanaged bool) (takeover.Facts, error) {
	cfg := rt.takeoverConfig()
	f := takeover.Facts{
		CurrentHTTP: rt.ingressHTTPListenAddr(), CurrentHTTPS: ":" + strconv.Itoa(rt.ingressHTTPSPort()),
		Window: cfg.Window, AcceptUnmanaged: acceptUnmanaged,
		MinDiskGB: float64(envInt(envTakeoverMinDiskGB, defaultTakeoverMinGB)), MinMemoryMB: envInt(envTakeoverMinMemMB, defaultTakeoverMinMB),
		RestartSupported: rt.restartResource().Supported,
		SocketActivation: rt.doctorEdge != nil && rt.doctorEdge.SocketActivation,
	}
	doms, err := rt.takeoverDomains(ctx)
	if err != nil {
		return f, err
	}
	f.Domains = doms
	rt.fillTakeoverDocker(ctx, &f)
	rt.fillTakeoverNetwork(ctx, &f)
	rt.fillTakeoverTLS(ctx, &f)
	rt.fillTakeoverHost(&f)
	return f, nil
}

func (rt *Router) fillTakeoverDocker(ctx context.Context, f *takeover.Facts) {
	rtm, ok := rt.localDockerRuntime()
	if !ok {
		return
	}
	lister, ok := rtm.(LocalContainerLister)
	if !ok {
		return
	}
	all, err := lister.ListLocalContainers(ctx)
	if err != nil {
		return
	}
	f.DockerAvailable = true
	inspector, _ := rtm.(proxyContainerInspector)
	policies := map[string]string{}
	hosts := map[string]bool{}
	for _, c := range all {
		if !c.Running {
			continue
		}
		for _, h := range takeover.TraefikHosts(c.Labels) {
			f.LabelRouters = append(f.LabelRouters, takeover.LabelRouter{Domain: h, Container: c.Name})
			hosts[h] = true
		}
		for _, port := range c.Published {
			if port != 80 && port != 443 {
				continue
			}
			kind, _ := proxycoexist.KindFromImage(c.Image)
			policy, seen := policies[c.Name]
			if !seen && inspector != nil {
				if lc, err := inspector.InspectLocalContainer(ctx, c.Name); err == nil {
					policy = lc.RestartPolicy
				}
				policies[c.Name] = policy
			}
			f.Holders = append(f.Holders, takeover.Holder{Port: port, Container: c.Name, Image: c.Image, Kind: string(kind), RestartPolicy: policy})
		}
	}
	for h := range hosts {
		f.OldDomains = append(f.OldDomains, h)
	}
	sort.Strings(f.OldDomains)
}

func (rt *Router) fillTakeoverNetwork(ctx context.Context, f *takeover.Facts) {
	if rt.publicHost != "" {
		f.ServerIPs = []string{rt.publicHost}
	}
	lookup := rt.lookupHost
	if lookup == nil {
		lookup = func(ctx context.Context, host string) ([]string, error) {
			return net.DefaultResolver.LookupHost(ctx, host)
		}
	}
	rt.parallel(len(f.Domains), func(i int) {
		dctx, cancel := context.WithTimeout(ctx, takeoverDNSTimeout)
		defer cancel()
		ips, err := lookup(dctx, f.Domains[i].Domain)
		f.Domains[i].ResolvesTo = ips
		if err != nil {
			f.Domains[i].DNSErr = err.Error()
		}
	})
}

func (rt *Router) fillTakeoverTLS(ctx context.Context, f *takeover.Facts) {
	if ing, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil {
		f.ACMEEnabled = ing.ACMEEnabled
	}
	custom, err := rt.customTLSCertDomains(ctx)
	if err != nil {
		return
	}
	for i := range f.Domains {
		f.Domains[i].CustomCert = custom[f.Domains[i].Domain]
	}
}

func (rt *Router) fillTakeoverHost(f *takeover.Facts) {
	ctx, cancel := context.WithTimeout(context.Background(), takeoverDNSTimeout)
	defer cancel()
	ss := rt.takeover.hostSS
	if ss == nil {
		ss = serverready.HostProbes().SS
	}
	listening := rt.takeover.portListening
	if listening == nil {
		listening = dialLocal
	}
	process := map[int]string{}
	if out, err := ss(ctx); err == nil {
		for _, l := range serverready.ParseSS(out) {
			if (l.Port == 80 || l.Port == 443) && l.Process != "docker-proxy" {
				process[l.Port] = l.Process
			}
		}
	}
	for _, port := range []int{80, 443} {
		held := slices.ContainsFunc(f.Holders, func(h takeover.Holder) bool { return h.Port == port })
		if held {
			continue
		}
		if _, ok := process[port]; ok || listening(port) {
			f.HostListeners = append(f.HostListeners, port)
		}
	}
	f.HostProcess = process
	if free, ok := rt.takeoverDiskFree(); ok {
		f.DiskFreeGB, f.DiskKnown = free, true
	}
	f.MemoryMB = rt.takeoverMemoryMB()
}

func dialLocal(port int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	c, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

func (rt *Router) takeoverDiskFree() (float64, bool) {
	if rt.takeover.diskFreeGB != nil {
		return rt.takeover.diskFreeGB()
	}
	gb, err := serverready.HostProbes().DiskFreeGB("/")
	return gb, err == nil
}

func (rt *Router) takeoverMemoryMB() int {
	if rt.takeover.memoryMB != nil {
		return rt.takeover.memoryMB()
	}
	mb, err := serverready.HostProbes().MemoryMB()
	if err != nil {
		return 0
	}
	return mb
}
