package api

import (
	"context"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DNS-01 provider names reported to the dashboard and CLI.
const (
	dnsProviderCloudflare = "cloudflare"
	dnsProviderRoute53    = "route53"
	dnsProviderNone       = "none"
)

// Certificate challenge verdicts for a domain.
const (
	challengeHTTP01        = "http-01"
	challengeDNS01Required = "dns-01-required"
)

// Next actions for an ACME failure, one per failure reason.
const (
	acmeActionOpenPort80 = "open_port_80"
	acmeActionFixDNS     = "fix_dns"
	acmeActionWaitRetry  = "wait_rate_limit"
	acmeActionFixCAA     = "fix_caa"
	acmeActionCheckLogs  = "check_logs"
)

const connectivityProbeLimit = 3 * time.Second

// acmeFailureResource is the last failed certificate attempt for a hostname,
// with the classified reason and the concrete next step.
type acmeFailureResource struct {
	Error   string    `json:"error"`
	Renewal bool      `json:"renewal"`
	At      time.Time `json:"at"`
	Reason  string    `json:"reason"`
	Action  string    `json:"action"`
}

// acmeActionForReason maps a classifyACMEError code to its next action.
func acmeActionForReason(reason string) string {
	switch reason {
	case httpsHintUnreachable:
		return acmeActionOpenPort80
	case httpsHintDNS:
		return acmeActionFixDNS
	case httpsHintRateLimited:
		return acmeActionWaitRetry
	case httpsHintCAA:
		return acmeActionFixCAA
	}
	return acmeActionCheckLogs
}

func newACMEFailureResource(f ingress.ACMEFailure) *acmeFailureResource {
	reason := classifyACMEError(f.Error)
	return &acmeFailureResource{Error: f.Error, Renewal: f.Renewal, At: f.At, Reason: reason, Action: acmeActionForReason(reason)}
}

// acmeFailureFor returns the recorded failure for domain, or nil.
func acmeFailureFor(domain string) *acmeFailureResource {
	f, ok := ingress.DefaultACMEFailures().Get(strings.ToLower(domain))
	if !ok {
		return nil
	}
	return newACMEFailureResource(f)
}

// activeDNSProvider reports which DNS-01 provider the ingress would use:
// Cloudflare wins when both are enabled, matching the reconciler.
func (rt *Router) activeDNSProvider(ctx context.Context) string {
	if rt.cloudflareDNS != nil && rt.cloudflareDNSSecrets != nil {
		if s, err := rt.cloudflareDNS.GetCloudflareDNSSettings(ctx); err == nil && s.Enabled {
			if ok, err := rt.cloudflareDNSSecrets.Exists(ctx, store.CloudflareDNSSecretsKey(), store.CloudflareDNSTokenEnvKey); err == nil && ok {
				return dnsProviderCloudflare
			}
		}
	}
	if rt.route53DNS != nil && rt.route53DNSSecrets != nil {
		if s, err := rt.route53DNS.GetRoute53DNSSettings(ctx); err == nil && s.Enabled {
			key := store.Route53DNSSecretsKey()
			id, e1 := rt.route53DNSSecrets.Exists(ctx, key, store.Route53DNSAccessKeyIDEnvKey)
			secret, e2 := rt.route53DNSSecrets.Exists(ctx, key, store.Route53DNSSecretAccessKeyEnvKey)
			if e1 == nil && e2 == nil && id && secret {
				return dnsProviderRoute53
			}
		}
	}
	return dnsProviderNone
}

// allPrivate reports whether addrs is non-empty and every parseable address
// is not publicly routable (RFC 1918, ULA, CGNAT, loopback, link-local).
func allPrivate(addrs []string) bool {
	seen := false
	for _, a := range addrs {
		ip := net.ParseIP(a)
		if ip == nil {
			continue
		}
		seen = true
		if ingress.IsPubliclyRoutable(ip) {
			return false
		}
	}
	return seen
}

// challengeFor decides which ACME challenge can validate a domain: HTTP-01
// needs the Let's Encrypt validators to reach this node, impossible when the
// address the domain must point at is private. A wildcard always needs DNS-01.
func challengeFor(domain string, expectedPrivate bool) string {
	if expectedPrivate || strings.HasPrefix(domain, "*.") {
		return challengeDNS01Required
	}
	return challengeHTTP01
}

// enrichDomainCheck adds the wizard's guidance fields to a DNS check result.
func (rt *Router) enrichDomainCheck(ctx context.Context, resp *domainCheckResponse) {
	expected := append(append([]string{}, resp.ExpectedIPv4...), resp.ExpectedIPv6...)
	if len(expected) == 0 && net.ParseIP(resp.ExpectedHost) != nil {
		expected = []string{resp.ExpectedHost}
	}
	resp.ExpectedPrivate = allPrivate(expected)
	resp.Challenge = challengeFor(resp.Domain, resp.ExpectedPrivate)
	resp.DNSProvider = rt.activeDNSProvider(ctx)
	resp.ACMEFailure = acmeFailureFor(resp.Domain)
}

// connectivityPort is one TCP probe of this node's ingress ports.
type connectivityPort struct {
	Port      int    `json:"port"`
	Address   string `json:"address"`
	Reachable bool   `json:"reachable"`
}

// Guidance codes for the connectivity verdict.
const (
	guidanceOK               = "ok"
	guidanceNoHost           = "no_host"
	guidancePrivateAddress   = "private_address"
	guidancePortsUnreachable = "ports_unreachable"
)

// ingressConnectivityResource is GET /api/v1/ingress/connectivity.
type ingressConnectivityResource struct {
	Host         string             `json:"host,omitempty"`
	HostInferred bool               `json:"host_inferred,omitempty"`
	Addresses    []string           `json:"addresses"`
	Private      bool               `json:"private"`
	Ports        []connectivityPort `json:"ports"`
	// HTTP01Possible is false when the node address is private, so no
	// public CA validator can ever reach port 80.
	HTTP01Possible bool   `json:"http01_possible"`
	DNSProvider    string `json:"dns_provider"`
	Guidance       string `json:"guidance"`
}

// connectivityGuidance is the pure verdict from probe results.
func connectivityGuidance(host string, private bool, ports []connectivityPort) string {
	switch {
	case host == "":
		return guidanceNoHost
	case private:
		return guidancePrivateAddress
	}
	for _, p := range ports {
		if !p.Reachable {
			return guidancePortsUnreachable
		}
	}
	return guidanceOK
}

func (rt *Router) probeIngressPorts(ctx context.Context, addrs []string) []connectivityPort {
	ports := []int{80, 443}
	out := make([]connectivityPort, len(ports))
	ctx, cancel := context.WithTimeout(ctx, connectivityProbeLimit)
	defer cancel()
	var wg sync.WaitGroup
	for i, port := range ports {
		out[i] = connectivityPort{Port: port}
		if len(addrs) == 0 {
			continue
		}
		out[i].Address = net.JoinHostPort(addrs[0], strconv.Itoa(port))
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := rt.doctorDialContextOrDefault()(ctx, "tcp", out[i].Address)
			if err == nil {
				_ = conn.Close()
				out[i].Reachable = true
			}
		}()
	}
	wg.Wait()
	return out
}

// handleIngressConnectivity handles GET /api/v1/ingress/connectivity: dials
// this node's advertised address on 80 and 443 from the control plane and
// says whether HTTP-01 can work at all. It cannot prove reachability from the
// internet, only from here; a private address is decisive.
func (rt *Router) handleIngressConnectivity(w http.ResponseWriter, r *http.Request) {
	host, inferred := advertisedHost(r, rt.publicHost)
	out := ingressConnectivityResource{Host: host, HostInferred: inferred, Addresses: []string{}, Ports: []connectivityPort{}}
	if host != "" {
		addrs := expectedIPs(r.Context(), rt.lookupHost, host)
		sort.Strings(addrs)
		out.Addresses = append(out.Addresses, addrs...)
		out.Private = allPrivate(addrs)
		out.Ports = rt.probeIngressPorts(r.Context(), addrs)
	}
	out.HTTP01Possible = host != "" && !out.Private
	out.DNSProvider = rt.activeDNSProvider(r.Context())
	out.Guidance = connectivityGuidance(host, out.Private, out.Ports)
	writeJSON(w, http.StatusOK, out)
}
