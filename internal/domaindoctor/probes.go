package domaindoctor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// ResolverAnswer is what one resolver answered for the domain.
type ResolverAnswer struct {
	Resolver string   `json:"resolver"`
	IPv4     []string `json:"ipv4,omitempty"`
	IPv6     []string `json:"ipv6,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// CAARecord is one CAA resource record and the name it was found at.
type CAARecord struct {
	Name  string
	Flag  uint8
	Tag   string
	Value string
}

// DNSProbe answers the DNS questions the doctor asks.
type DNSProbe interface {
	Answers(ctx context.Context, host string) []ResolverAnswer
	CNAME(ctx context.Context, host string) (string, error)
	// CAA returns the CAA set that governs host: the records at the closest
	// name walking up from host, or none.
	CAA(ctx context.Context, host string) ([]CAARecord, error)
}

// PeerCert is the leaf certificate a TLS handshake presented.
type PeerCert struct {
	Subject     string
	Issuer      string
	DNSNames    []string
	NotAfter    time.Time
	Verified    bool
	VerifyError string
}

// TLSProbe performs a handshake against addr with the given SNI.
type TLSProbe interface {
	Handshake(ctx context.Context, addr, sni string) (PeerCert, error)
}

// HTTPResult is one HTTP response, redirects not followed.
type HTTPResult struct {
	Status   int
	Location string
	HSTS     string
}

// HTTPProbe sends one GET for scheme://host+path but always dials addr.
type HTTPProbe interface {
	Get(ctx context.Context, addr, scheme, host, path string) (HTTPResult, error)
}

// DefaultProbes are the real network probes.
func DefaultProbes() Probes {
	return Probes{DNS: NetDNS{}, TLS: NetTLS{}, HTTP: NetHTTP{}}
}

// NetDNS queries the system resolver plus well known public resolvers.
type NetDNS struct {
	// Public lists resolver addresses (host:port); nil uses 1.1.1.1 and
	// 8.8.8.8 unless APP_DNS_PUBLIC_RESOLVERS is off.
	Public []string
}

func (d NetDNS) public() []string {
	if d.Public != nil {
		return d.Public
	}
	if v := strings.ToLower(os.Getenv("APP_DNS_PUBLIC_RESOLVERS")); v == "off" || v == "false" || v == "0" {
		return nil
	}
	return []string{"1.1.1.1:53", "8.8.8.8:53"}
}

// Answers implements DNSProbe.
func (d NetDNS) Answers(ctx context.Context, host string) []ResolverAnswer {
	out := []ResolverAnswer{systemAnswer(ctx, host)}
	for _, server := range d.public() {
		out = append(out, serverAnswer(ctx, server, host))
	}
	return out
}

func systemAnswer(ctx context.Context, host string) ResolverAnswer {
	ans := ResolverAnswer{Resolver: "this server"}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		ans.Error = dnsErrorText(err)
		return ans
	}
	for _, a := range addrs {
		if a.IP.To4() != nil {
			ans.IPv4 = append(ans.IPv4, a.IP.String())
		} else {
			ans.IPv6 = append(ans.IPv6, a.IP.String())
		}
	}
	return ans
}

func dnsErrorText(err error) string {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return "not found"
	}
	return "no answer"
}

func serverAnswer(ctx context.Context, server, host string) ResolverAnswer {
	ans := ResolverAnswer{Resolver: strings.TrimSuffix(server, ":53")}
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		msg, err := exchange(ctx, server, host, qtype)
		if err != nil {
			ans.Error = "no answer"
			continue
		}
		if msg.Rcode == dns.RcodeNameError {
			ans.Error = "not found"
		}
		for _, rr := range msg.Answer {
			switch v := rr.(type) {
			case *dns.A:
				ans.IPv4 = append(ans.IPv4, v.A.String())
			case *dns.AAAA:
				ans.IPv6 = append(ans.IPv6, v.AAAA.String())
			}
		}
	}
	if len(ans.IPv4)+len(ans.IPv6) > 0 {
		ans.Error = ""
	} else if ans.Error == "" {
		ans.Error = "not found"
	}
	return ans
}

func exchange(ctx context.Context, server, host string, qtype uint16) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(host), qtype)
	m.RecursionDesired = true
	c := &dns.Client{Timeout: 3 * time.Second}
	resp, _, err := c.ExchangeContext(ctx, m, server)
	if err != nil {
		return nil, fmt.Errorf("query %s for %s: %w", server, host, err)
	}
	return resp, nil
}

func (d NetDNS) queryServer() string {
	if pub := d.public(); len(pub) > 0 {
		return pub[0]
	}
	if cfg, err := dns.ClientConfigFromFile("/etc/resolv.conf"); err == nil && len(cfg.Servers) > 0 {
		return net.JoinHostPort(cfg.Servers[0], cfg.Port)
	}
	return ""
}

// CNAME implements DNSProbe.
func (d NetDNS) CNAME(ctx context.Context, host string) (string, error) {
	server := d.queryServer()
	if server == "" {
		return "", errors.New("no resolver available")
	}
	msg, err := exchange(ctx, server, host, dns.TypeCNAME)
	if err != nil {
		return "", err
	}
	for _, rr := range msg.Answer {
		if c, ok := rr.(*dns.CNAME); ok && strings.EqualFold(strings.TrimSuffix(c.Hdr.Name, "."), host) {
			return strings.TrimSuffix(c.Target, "."), nil
		}
	}
	return "", nil
}

// CAA implements DNSProbe, walking up to (not including) the TLD.
func (d NetDNS) CAA(ctx context.Context, host string) ([]CAARecord, error) {
	server := d.queryServer()
	if server == "" {
		return nil, errors.New("no resolver available")
	}
	labels := strings.Split(strings.TrimSuffix(host, "."), ".")
	for i := 0; i < len(labels)-1; i++ {
		name := strings.Join(labels[i:], ".")
		msg, err := exchange(ctx, server, name, dns.TypeCAA)
		if err != nil {
			return nil, err
		}
		var out []CAARecord
		for _, rr := range msg.Answer {
			if c, ok := rr.(*dns.CAA); ok {
				out = append(out, CAARecord{Name: name, Flag: c.Flag, Tag: strings.ToLower(c.Tag), Value: c.Value})
			}
		}
		if len(out) > 0 {
			return out, nil
		}
	}
	return nil, nil
}

// NetTLS dials and handshakes for real.
type NetTLS struct{}

// Handshake implements TLSProbe. Verification runs separately so an
// untrusted certificate is still described rather than just rejected.
func (NetTLS) Handshake(ctx context.Context, addr, sni string) (PeerCert, error) {
	d := &tls.Dialer{Config: &tls.Config{ServerName: sni, InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}} //nolint:gosec // the chain is verified below so an untrusted certificate can be reported, not hidden
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return PeerCert{}, fmt.Errorf("tls handshake with %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	tc, ok := conn.(*tls.Conn)
	if !ok {
		return PeerCert{}, errors.New("tls handshake: not a TLS connection")
	}
	certs := tc.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return PeerCert{}, errors.New("tls handshake: no certificate presented")
	}
	leaf := certs[0]
	pc := PeerCert{Subject: leaf.Subject.String(), Issuer: leaf.Issuer.String(), DNSNames: leaf.DNSNames, NotAfter: leaf.NotAfter}
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	if _, verr := leaf.Verify(x509.VerifyOptions{DNSName: sni, Intermediates: inter}); verr != nil {
		pc.VerifyError = verr.Error()
	} else {
		pc.Verified = true
	}
	return pc, nil
}

// NetHTTP sends single requests that never follow redirects.
type NetHTTP struct{}

// Get implements HTTPProbe.
func (NetHTTP) Get(ctx context.Context, addr, scheme, host, path string) (HTTPResult, error) {
	dialer := &net.Dialer{}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		TLSClientConfig:   &tls.Config{ServerName: host, InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, //nolint:gosec // only the status and headers are read; trust is judged by the TLS check
		DisableKeepAlives: true,
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, scheme+"://"+host+path, nil)
	if err != nil {
		return HTTPResult{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "domain-doctor")
	resp, err := client.Do(req)
	if err != nil {
		return HTTPResult{}, fmt.Errorf("GET %s://%s%s: %w", scheme, host, path, err)
	}
	_ = resp.Body.Close()
	return HTTPResult{Status: resp.StatusCode, Location: resp.Header.Get("Location"), HSTS: resp.Header.Get("Strict-Transport-Security")}, nil
}
