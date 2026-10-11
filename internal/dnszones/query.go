package dnszones

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// ServerSystem names the host's own configured resolver.
const ServerSystem = "system"

// Answer is what one server returned for one question.
type Answer struct {
	Server        string   `json:"server"`
	Values        []string `json:"values,omitempty"`
	TTL           uint32   `json:"ttl"`
	Rcode         string   `json:"rcode,omitempty"`
	Authoritative bool     `json:"authoritative,omitempty"`
	CNAME         string   `json:"cname,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// Querier asks one server one question. server is "host:port" or ServerSystem.
type Querier interface {
	Query(ctx context.Context, server, name string, qtype uint16) Answer
}

// DNSQuerier is the real Querier, over UDP with a TCP retry on truncation.
type DNSQuerier struct {
	Timeout    time.Duration
	ResolvConf string
}

// NewDNSQuerier returns a querier with a per query timeout.
func NewDNSQuerier(timeout time.Duration) *DNSQuerier {
	return &DNSQuerier{Timeout: timeout, ResolvConf: "/etc/resolv.conf"}
}

func (q *DNSQuerier) systemServer() string {
	cfg, err := dns.ClientConfigFromFile(q.ResolvConf)
	if err != nil || len(cfg.Servers) == 0 {
		return ""
	}
	return net.JoinHostPort(cfg.Servers[0], cfg.Port)
}

// Query implements Querier.
func (q *DNSQuerier) Query(ctx context.Context, server, name string, qtype uint16) Answer {
	label := server
	if server == ServerSystem {
		server = q.systemServer()
		if server == "" {
			return Answer{Server: label, Error: "no system resolver configured"}
		}
	}
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(NormalizeDomain(name)), qtype)
	msg.RecursionDesired = true
	c := &dns.Client{Timeout: q.Timeout}
	resp, _, err := c.ExchangeContext(ctx, msg, server)
	if err == nil && resp != nil && resp.Truncated {
		c.Net = "tcp"
		resp, _, err = c.ExchangeContext(ctx, msg, server)
	}
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return Answer{Server: label, Error: "timeout"}
		}
		return Answer{Server: label, Error: "no answer"}
	}
	return answerFrom(label, resp, qtype)
}

func answerFrom(label string, resp *dns.Msg, qtype uint16) Answer {
	a := Answer{Server: label, Rcode: strings.ToLower(dns.RcodeToString[resp.Rcode]), Authoritative: resp.Authoritative}
	first := true
	for _, rr := range resp.Answer {
		h := rr.Header()
		if c, ok := rr.(*dns.CNAME); ok && qtype != dns.TypeCNAME {
			a.CNAME = NormalizeDomain(c.Target)
			continue
		}
		if h.Rrtype != qtype {
			continue
		}
		if v, ok := FormatRR(rr); ok {
			a.Values = append(a.Values, v)
		}
		if first || h.Ttl < a.TTL {
			a.TTL = h.Ttl
			first = false
		}
	}
	return a
}

// QType maps a record type name to its wire value.
func QType(t string) (uint16, error) {
	v, ok := dns.StringToType[strings.ToUpper(strings.TrimSpace(t))]
	if !ok || !supportedType(strings.ToUpper(t)) {
		return 0, fmt.Errorf("dnszones: unsupported record type %q", t)
	}
	return v, nil
}

// withPort adds :53 to a bare host or IP.
func withPort(s string) string {
	if s == ServerSystem {
		return s
	}
	if _, _, err := net.SplitHostPort(s); err == nil {
		return s
	}
	return net.JoinHostPort(s, "53")
}
