// Package dnsinspect asks DNS questions the control plane controls itself:
// public resolvers and the zone's authoritative servers, so a stale cache on
// this host never decides whether a domain is ready.
package dnsinspect

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"golang.org/x/net/publicsuffix"
)

// EnvPublicResolvers overrides the public resolver list (comma separated
// host or host:port); "off", "false" or "0" disables public queries.
const EnvPublicResolvers = "APP_DNS_PUBLIC_RESOLVERS"

const (
	maxNameServers   = 6
	maxServerIPs     = 2
	defaultQueryWait = 3 * time.Second
)

var defaultPublic = []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}

// PublicResolvers returns the configured public resolver addresses (host:port).
func PublicResolvers() []string {
	v := strings.TrimSpace(os.Getenv(EnvPublicResolvers))
	switch strings.ToLower(v) {
	case "":
		return slices.Clone(defaultPublic)
	case "off", "false", "0":
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(p); err != nil {
			p = net.JoinHostPort(p, "53")
		}
		out = append(out, p)
	}
	return out
}

// Source is one resolver's answer for the domain.
type Source struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses,omitempty"`
	// TTL is the longest answer TTL in seconds; for an empty answer it is the
	// negative-caching TTL the server reported.
	TTL   uint32 `json:"ttl,omitempty"`
	Error string `json:"error,omitempty"`
}

// Inspection is everything one run learned.
type Inspection struct {
	Zone string `json:"zone,omitempty"`
	// DelegatedNS is what public resolvers say the zone's name servers are;
	// ZoneNS is what the zone's own servers say.
	DelegatedNS   []string `json:"delegated_ns,omitempty"`
	ZoneNS        []string `json:"zone_ns,omitempty"`
	Authoritative []Source `json:"authoritative"`
	Public        []Source `json:"public"`
	// AuthReachable is true when at least one authoritative server answered.
	AuthReachable bool `json:"auth_reachable"`
	// NegativeTTL is the SOA negative-caching time of the zone, in seconds.
	NegativeTTL uint32 `json:"negative_ttl,omitempty"`
}

// Exchanger sends one query; tests replace it.
type Exchanger func(ctx context.Context, server, name string, qtype uint16, recurse bool) (*dns.Msg, error)

// Inspector queries public resolvers and authoritative servers.
type Inspector struct {
	Public []string
	// LocalServers overrides the resolvers read from resolv.conf (tests).
	LocalServers []string
	// AuthPort is the port authoritative servers are queried on; 0 means 53.
	AuthPort int
	// AllowPrivate lets authoritative servers sit on private addresses; only
	// tests set it, since otherwise a zone could aim this host at its LAN.
	AllowPrivate bool
	Timeout      time.Duration
	Exchange     Exchanger
}

// New returns an Inspector using the configured public resolvers.
func New() *Inspector { return &Inspector{Public: PublicResolvers()} }

func (in *Inspector) timeout() time.Duration {
	if in.Timeout > 0 {
		return in.Timeout
	}
	return defaultQueryWait
}

func (in *Inspector) exchange(ctx context.Context, server, name string, qtype uint16, recurse bool) (*dns.Msg, error) {
	if in.Exchange != nil {
		return in.Exchange(ctx, server, name, qtype, recurse)
	}
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.RecursionDesired = recurse
	c := &dns.Client{Timeout: in.timeout()}
	resp, _, err := c.ExchangeContext(ctx, m, server)
	if err == nil && resp.Truncated {
		c.Net = "tcp"
		resp, _, err = c.ExchangeContext(ctx, m, server)
	}
	if err != nil {
		return nil, fmt.Errorf("query %s for %s: %w", server, name, err)
	}
	return resp, nil
}

// Inspect looks host up on every configured public resolver and on each of
// its zone's authoritative servers.
func (in *Inspector) Inspect(ctx context.Context, host string) Inspection {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	var out Inspection
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		out.Public = in.publicSources(ctx, host)
	}()
	zone, delegated := in.findZone(ctx, host)
	out.Zone, out.DelegatedNS = zone, delegated
	if zone != "" {
		in.authoritative(ctx, host, &out)
	}
	wg.Wait()
	if out.NegativeTTL == 0 {
		for _, s := range out.Public {
			if len(s.Addresses) == 0 && s.TTL > 0 {
				out.NegativeTTL = s.TTL
				break
			}
		}
	}
	return out
}

func (in *Inspector) publicSources(ctx context.Context, host string) []Source {
	out := make([]Source, len(in.Public))
	var wg sync.WaitGroup
	for i, server := range in.Public {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = in.lookup(ctx, strings.TrimSuffix(server, ":53"), server, host, true)
		}()
	}
	wg.Wait()
	return out
}

// lookup asks one server for A and AAAA and merges the answers.
func (in *Inspector) lookup(ctx context.Context, name, server, host string, recurse bool) Source {
	src := Source{Name: name}
	qtypes := []uint16{dns.TypeA, dns.TypeAAAA}
	msgs := make([]*dns.Msg, len(qtypes))
	errs := make([]error, len(qtypes))
	var wg sync.WaitGroup
	for i, qt := range qtypes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msgs[i], errs[i] = in.exchange(ctx, server, host, qt, recurse)
		}()
	}
	wg.Wait()
	var nxdomain, anyOK bool
	var neg uint32
	for i, msg := range msgs {
		if errs[i] != nil {
			continue
		}
		anyOK = true
		nxdomain = nxdomain || msg.Rcode == dns.RcodeNameError
		for _, rr := range msg.Answer {
			switch v := rr.(type) {
			case *dns.A:
				src.Addresses = append(src.Addresses, v.A.String())
				src.TTL = max(src.TTL, v.Hdr.Ttl)
			case *dns.AAAA:
				src.Addresses = append(src.Addresses, v.AAAA.String())
				src.TTL = max(src.TTL, v.Hdr.Ttl)
			}
		}
		for _, rr := range msg.Ns {
			if soa, ok := rr.(*dns.SOA); ok {
				neg = max(neg, min(soa.Hdr.Ttl, soa.Minttl))
			}
		}
	}
	switch {
	case len(src.Addresses) > 0:
	case !anyOK:
		src.Error = "no answer"
	case nxdomain:
		src.Error, src.TTL = "not found", neg
	default:
		src.Error, src.TTL = "no records", neg
	}
	return src
}

// findZone walks up from host to the closest name that has NS records.
func (in *Inspector) findZone(ctx context.Context, host string) (string, []string) {
	if len(in.Public) == 0 {
		return "", nil
	}
	name := strings.TrimPrefix(host, "*.")
	for name != "" {
		if suffix, _ := publicsuffix.PublicSuffix(name); suffix == name {
			return "", nil
		}
		for _, server := range in.Public {
			msg, err := in.exchange(ctx, server, name, dns.TypeNS, true)
			if err != nil {
				continue
			}
			if ns := nsTargets(msg.Answer); len(ns) > 0 {
				return name, ns
			}
			break
		}
		_, rest, ok := strings.Cut(name, ".")
		if !ok {
			break
		}
		name = rest
	}
	return "", nil
}

func nsTargets(rrs []dns.RR) []string {
	var out []string
	for _, rr := range rrs {
		if ns, ok := rr.(*dns.NS); ok {
			t := strings.ToLower(strings.TrimSuffix(ns.Ns, "."))
			if !slices.Contains(out, t) {
				out = append(out, t)
			}
		}
	}
	slices.Sort(out)
	return out
}

func (in *Inspector) authoritative(ctx context.Context, host string, out *Inspection) {
	names := out.DelegatedNS
	if len(names) > maxNameServers {
		names = names[:maxNameServers]
	}
	res := make([]Source, len(names))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var once sync.Once
	for i, ns := range names {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res[i] = Source{Name: ns, Error: "no answer"}
			ips := in.serverIPs(ctx, ns)
			if len(ips) == 0 {
				res[i].Error = "no usable address"
				return
			}
			server := net.JoinHostPort(ips[0], portString(in.AuthPort))
			src := in.lookup(ctx, ns, server, host, false)
			res[i] = src
			if src.Error == "no answer" {
				return
			}
			mu.Lock()
			out.AuthReachable = true
			mu.Unlock()
			once.Do(func() { in.zoneView(ctx, server, out, &mu) })
		}()
	}
	wg.Wait()
	out.Authoritative = res
}

// zoneView records the NS set and negative TTL the zone itself publishes.
func (in *Inspector) zoneView(ctx context.Context, server string, out *Inspection, mu *sync.Mutex) {
	msg, err := in.exchange(ctx, server, out.Zone, dns.TypeNS, false)
	if err != nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	out.ZoneNS = nsTargets(msg.Answer)
	if q, err := in.exchange(ctx, server, out.Zone, dns.TypeSOA, false); err == nil {
		for _, rr := range q.Answer {
			if soa, ok := rr.(*dns.SOA); ok {
				out.NegativeTTL = max(out.NegativeTTL, min(soa.Hdr.Ttl, soa.Minttl))
			}
		}
	}
}

func portString(p int) string {
	if p <= 0 {
		return "53"
	}
	return fmt.Sprint(p)
}

// serverIPs resolves a name server through the public resolvers and keeps
// only addresses this host may query.
func (in *Inspector) serverIPs(ctx context.Context, ns string) []string {
	var ips []string
	for _, qt := range []uint16{dns.TypeA, dns.TypeAAAA} {
		for _, server := range in.Public {
			msg, err := in.exchange(ctx, server, ns, qt, true)
			if err != nil {
				continue
			}
			for _, rr := range msg.Answer {
				var ip net.IP
				switch v := rr.(type) {
				case *dns.A:
					ip = v.A
				case *dns.AAAA:
					ip = v.AAAA
				}
				if ip != nil && in.allowed(ip) && !slices.Contains(ips, ip.String()) {
					ips = append(ips, ip.String())
				}
			}
			break
		}
	}
	if len(ips) > maxServerIPs {
		ips = ips[:maxServerIPs]
	}
	return ips
}

func (in *Inspector) allowed(ip net.IP) bool {
	if in.AllowPrivate {
		return true
	}
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	return a.IsGlobalUnicast() && !a.IsPrivate() && !a.IsLoopback() && !a.IsLinkLocalUnicast()
}
