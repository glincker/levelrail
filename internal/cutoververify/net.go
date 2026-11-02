package cutoververify

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptrace"
	"strconv"
	"syscall"
	"time"
)

// NetProber probes over the public name with the standard library. Every
// connection goes to one of the addresses the caller passes in, so the address
// that was checked is the address that is dialed.
type NetProber struct {
	// Timeout bounds each request; it must be set by the caller.
	Timeout time.Duration
	// HTTPSPort is the port the TLS probes use, 443 when zero.
	HTTPSPort int
	// dial replaces the network dial in tests; allowNonPublic lets tests use loopback servers.
	dial           func(ctx context.Context, network, addr string) (net.Conn, error)
	allowNonPublic bool
}

// dialPinned returns a dial function that ignores the name in the request and
// connects to addrs instead, so a second DNS answer cannot redirect the probe.
func (p NetProber) dialPinned(addrs []string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		_, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		if p.HTTPSPort != 0 && p.HTTPSPort != 443 && port == "443" {
			port = strconv.Itoa(p.HTTPSPort)
		}
		if len(addrs) == 0 {
			return nil, &net.DNSError{Err: "no address to connect to", IsNotFound: true}
		}
		d := net.Dialer{Timeout: p.Timeout, Control: p.control}
		dial := d.DialContext
		if p.dial != nil {
			dial = p.dial
		}
		for _, a := range addrs {
			if !p.allowNonPublic && !IsPublicIP(a) {
				return nil, ErrNonPublicAddress
			}
		}
		var last error
		for _, a := range addrs {
			c, derr := dial(ctx, network, net.JoinHostPort(a, port))
			if derr == nil {
				return c, nil
			}
			last = derr
		}
		return nil, last
	}
}

// control re-checks the address the socket is about to connect to, in case
// anything between the check and the dial changed it.
func (p NetProber) control(_, address string, _ syscall.RawConn) error {
	if p.allowNonPublic {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || !IsPublicIP(host) {
		return ErrNonPublicAddress
	}
	return nil
}

// HTTP makes one GET to addrs and does not follow redirects.
func (p NetProber) HTTP(ctx context.Context, rawURL string, addrs []string) (HTTPResult, error) {
	tr := &http.Transport{
		DialContext:     p.dialPinned(addrs),
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}, //nolint:gosec // certificate trust is its own line
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: p.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var first time.Time
	start := time.Now()
	trace := &httptrace.ClientTrace{GotFirstResponseByte: func() { first = time.Now() }}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodGet, rawURL, nil)
	if err != nil {
		return HTTPResult{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return HTTPResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	out := HTTPResult{Status: resp.StatusCode, Location: resp.Header.Get("Location"), Header: map[string]string{}}
	for k := range resp.Header {
		out.Header[k] = resp.Header.Get(k)
	}
	if !first.IsZero() {
		out.TTFB = first.Sub(start)
	}
	return out, nil
}

// Cert reads the leaf certificate served by addrs and checks it against the system roots.
func (p NetProber) Cert(ctx context.Context, host string, addrs []string) (CertInfo, error) {
	raw, err := p.dialPinned(addrs)(ctx, "tcp", net.JoinHostPort(host, "443"))
	if err != nil {
		return CertInfo{}, err
	}
	defer func() { _ = raw.Close() }()
	conn := tls.Client(raw, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}) //nolint:gosec // trust is evaluated below
	hctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	if err := conn.HandshakeContext(hctx); err != nil {
		return CertInfo{}, err
	}
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return CertInfo{}, net.ErrClosed
	}
	pool := x509.NewCertPool()
	for _, c := range certs[1:] {
		pool.AddCert(c)
	}
	_, verr := certs[0].Verify(x509.VerifyOptions{DNSName: host, Intermediates: pool})
	issuer := certs[0].Issuer.CommonName
	if len(certs[0].Issuer.Organization) > 0 {
		issuer = certs[0].Issuer.Organization[0] + " " + issuer
	}
	return CertInfo{Issuer: issuer, NotAfter: certs[0].NotAfter, Trusted: verr == nil}, nil
}
