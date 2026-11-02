package cutoververify

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"time"
)

// NetProber probes over the public name with the standard library.
type NetProber struct {
	// Timeout bounds each request; it must be set by the caller.
	Timeout time.Duration
	// HTTPSPort is the port the TLS probes use, 443 when zero.
	HTTPSPort int
}

// HTTP makes one GET and does not follow redirects.
func (p NetProber) HTTP(ctx context.Context, rawURL string) (HTTPResult, error) {
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}} //nolint:gosec // certificate trust is its own line
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, Timeout: p.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	if u, perr := url.Parse(rawURL); perr == nil && u.Scheme == "https" && p.HTTPSPort != 0 && p.HTTPSPort != 443 && u.Port() == "" {
		d := net.Dialer{Timeout: p.Timeout}
		tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return d.DialContext(ctx, network, net.JoinHostPort(u.Hostname(), strconv.Itoa(p.HTTPSPort)))
		}
	}
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

// Cert reads the served leaf certificate and checks it against the system roots.
func (p NetProber) Cert(ctx context.Context, host string) (CertInfo, error) {
	port := p.HTTPSPort
	if port == 0 {
		port = 443
	}
	d := net.Dialer{Timeout: p.Timeout}
	raw, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
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
