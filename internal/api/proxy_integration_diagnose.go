package api

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	diagnoseLookupTimeout = 3 * time.Second
	proxyLogTailLines     = 500
)

type proxyLogTailer interface {
	TailLogs(ctx context.Context, name string, lines int) (string, error)
}

// lookupDomainIPs resolves only the managed domain with the control plane resolver.
func lookupDomainIPs(ctx context.Context, host string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, diagnoseLookupTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.IP.String())
	}
	return out, nil
}

// dialIsOpen only ever dials the address the probe itself uses.
func dialIsOpen(ctx context.Context, addr string) bool {
	c, err := (&net.Dialer{Timeout: diagnoseLookupTimeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// applyProxyDiagnosis gathers evidence only when the certificate is a proxy
// fallback, then stores the diagnosis and replaces the generic probe error.
func (rt *Router) applyProxyDiagnosis(ctx context.Context, deps *proxyIntegrationDeps, det proxyroutes.Detection, settings store.ProxyIntegrationSettings,
	it proxyroutes.Item, addr string, res proxyroutes.ProbeResult, st *store.ProxyRouteStatus) {
	if !res.Reachable || res.CertClass == "" {
		return
	}
	in := proxyroutes.DiagnoseInput{
		Domain: it.Domain, Class: res.CertClass, Issuer: res.CertIssuer, NotAfter: res.CertNotAfter, Now: time.Now(),
		Budget: rt.runtimeSettings.ProxyCertWait(ctx), RouterName: deps.syncer.NS.RouterName(it.Domain), ConfiguredResolver: settings.CertResolver,
	}
	if sts, err := deps.store.ListProxyRouteStatus(ctx); err == nil {
		for _, s := range sts {
			if s.Domain == it.Domain {
				in.FirstSeen = s.WrittenAt
			}
		}
	}
	if res.CertClass == proxyroutes.ClassProxyDefault {
		rt.collectFallbackEvidence(ctx, deps, det, addr, &in)
	}
	d := proxyroutes.Diagnose(in)
	st.Diagnosis, st.Hint, st.Snippet, st.Evidence = d.Code, d.Hint, d.Snippet, d.Evidence
	if d.Summary != "" && !res.CertTrusted {
		st.LastError = d.Summary
	}
}

func (rt *Router) collectFallbackEvidence(ctx context.Context, deps *proxyIntegrationDeps, det proxyroutes.Detection, addr string, in *proxyroutes.DiagnoseInput) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return
	}
	if det.APIPort > 0 && deps.routerTLS != nil {
		base := "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(det.APIPort))
		if info, err := deps.routerTLS(ctx, base, in.RouterName, deps.timeout()); err == nil && info.Found {
			in.Router = &info
		}
		if deps.knownResolvers != nil {
			in.Resolvers, _ = deps.knownResolvers(ctx, base, deps.timeout())
		}
	}
	// A loopback probe address says nothing about where the domain should point.
	if ip := net.ParseIP(host); deps.lookupIPs != nil && (ip == nil || !ip.IsLoopback()) && host != "localhost" {
		facts := &proxyroutes.DNSFacts{}
		if ips, err := deps.lookupIPs(ctx, in.Domain); err != nil {
			facts.Err = err.Error()
		} else {
			facts.IPs = ips
		}
		if ip != nil {
			facts.ServerIPs = []string{ip.String()}
		}
		in.DNS = facts
	}
	if line := proxyroutes.ACMEFailureSince(rt.proxyContainerLogs(ctx, deps, det.Container), in.Domain, latest(in.FirstSeen, proxyroutes.RetriedAt(in.Domain))); line != "" {
		in.ACMELog = secretmask.Mask(line)
	}
	if deps.dialOpen != nil {
		open := deps.dialOpen(ctx, net.JoinHostPort(host, "80"))
		in.Port80Open = &open
	}
}

// proxyContainerLogs reads recent proxy output through the Docker API.
func (rt *Router) proxyContainerLogs(ctx context.Context, deps *proxyIntegrationDeps, container string) string {
	if container == "" {
		return ""
	}
	read := deps.proxyLogs
	if read == nil {
		if rt.execRuntime == nil {
			return ""
		}
		runtime, err := rt.execRuntime("")
		if err != nil {
			return ""
		}
		t, ok := runtime.(proxyLogTailer)
		if !ok {
			return ""
		}
		read = func(ctx context.Context, name string) (string, error) {
			return t.TailLogs(ctx, name, proxyLogTailLines)
		}
	}
	logs, err := read(ctx, container)
	if err != nil {
		return ""
	}
	return logs
}

// retryProxyCert asks the syncer to nudge the proxy; verification follows.
func (rt *Router) retryProxyCert(ctx context.Context, deps *proxyIntegrationDeps, domain string) error {
	if deps.retryCert == nil {
		return proxyroutes.ErrRetryUnsupported
	}
	return deps.retryCert(ctx, domain)
}

// latest returns the later of two times.
func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
