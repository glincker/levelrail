package datamigrate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// Resolver reads what public DNS says about a domain.
type Resolver interface {
	Lookup(ctx context.Context, domain string) DomainState
}

// DNSResolver asks the domain's authoritative servers for the real TTL and
// falls back to the system resolver, which can only show a decayed one.
type DNSResolver struct {
	Timeout time.Duration
}

const fallbackResolver = "1.1.1.1:53"

func (r DNSResolver) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return 4 * time.Second
}

// Lookup implements Resolver.
func (r DNSResolver) Lookup(ctx context.Context, domain string) DomainState {
	fqdn := dns.Fqdn(strings.TrimSpace(domain))
	st := DomainState{}
	if servers := r.authoritative(ctx, fqdn); len(servers) > 0 {
		for _, s := range servers {
			if ttl, cname, addrs, err := r.query(ctx, s, fqdn, false); err == nil && (ttl > 0 || cname != "" || len(addrs) > 0) {
				st.TTL, st.CNAME, st.Addrs, st.Authoritative = ttl, cname, addrs, true
				break
			}
		}
	}
	if !st.Authoritative {
		ttl, cname, addrs, err := r.query(ctx, systemServer(), fqdn, true)
		if err != nil {
			st.Error = err.Error()
			return st
		}
		st.TTL, st.CNAME, st.Addrs = ttl, cname, addrs
	}
	if len(st.Addrs) == 0 {
		hosts, err := net.DefaultResolver.LookupHost(ctx, strings.TrimSuffix(fqdn, "."))
		if err != nil {
			st.Error = err.Error()
			return st
		}
		st.Addrs = hosts
	}
	return st
}

func systemServer() string {
	if cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf"); err == nil && len(cfg.Servers) > 0 {
		return net.JoinHostPort(cfg.Servers[0], cfg.Port)
	}
	return fallbackResolver
}

// authoritative walks up from fqdn to the first zone that has NS records and
// returns the addresses of its name servers.
func (r DNSResolver) authoritative(ctx context.Context, fqdn string) []string {
	labels := dns.SplitDomainName(fqdn)
	sys := systemServer()
	for i := 0; i < len(labels)-1; i++ {
		zone := dns.Fqdn(strings.Join(labels[i:], "."))
		msg, err := r.exchange(ctx, sys, zone, dns.TypeNS, true)
		if err != nil {
			continue
		}
		var out []string
		for _, rr := range msg.Answer {
			ns, ok := rr.(*dns.NS)
			if !ok {
				continue
			}
			hosts, err := net.DefaultResolver.LookupHost(ctx, strings.TrimSuffix(ns.Ns, "."))
			if err != nil || len(hosts) == 0 {
				continue
			}
			out = append(out, net.JoinHostPort(hosts[0], "53"))
		}
		if len(out) > 0 {
			return out
		}
	}
	return nil
}

func (r DNSResolver) query(ctx context.Context, server, fqdn string, recurse bool) (uint32, string, []string, error) {
	var ttl uint32
	var cname string
	var addrs []string
	for _, qt := range []uint16{dns.TypeA, dns.TypeAAAA} {
		msg, err := r.exchange(ctx, server, fqdn, qt, recurse)
		if err != nil {
			if qt == dns.TypeA {
				return 0, "", nil, err
			}
			continue
		}
		for i, rr := range msg.Answer {
			if i == 0 && ttl == 0 {
				ttl = rr.Header().Ttl
			}
			switch v := rr.(type) {
			case *dns.CNAME:
				if cname == "" {
					cname = strings.TrimSuffix(v.Target, ".")
				}
			case *dns.A:
				addrs = append(addrs, v.A.String())
			case *dns.AAAA:
				addrs = append(addrs, v.AAAA.String())
			}
		}
	}
	return ttl, cname, addrs, nil
}

func (r DNSResolver) exchange(ctx context.Context, server, name string, qtype uint16, recurse bool) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(name, qtype)
	m.RecursionDesired = recurse
	c := &dns.Client{Timeout: r.timeout()}
	resp, _, err := c.ExchangeContext(ctx, m, server)
	if err != nil {
		return nil, fmt.Errorf("dns query %s %s: %w", name, dns.TypeToString[qtype], err)
	}
	if resp.Rcode != dns.RcodeSuccess {
		return nil, errors.New("dns " + dns.RcodeToString[resp.Rcode])
	}
	return resp, nil
}
