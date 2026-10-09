package datamigrate

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Prober is how PostSwitch reaches the network, replaceable in tests.
type Prober struct {
	Resolver Resolver
	// Dial opens a TCP connection, used to reach this node directly.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
	// RootCAs overrides the system roots, for a private CA.
	RootCAs *x509.CertPool
	Timeout time.Duration
}

func (p Prober) timeout() time.Duration {
	if p.Timeout > 0 {
		return p.Timeout
	}
	return 8 * time.Second
}

func (p Prober) dial() func(ctx context.Context, network, addr string) (net.Conn, error) {
	if p.Dial != nil {
		return p.Dial
	}
	d := &net.Dialer{Timeout: p.timeout()}
	return d.DialContext
}

// PostSwitch checks domain is served from this node: DNS resolves here, a
// trusted certificate is presented, and healthPath answers 200. TLS and HTTP
// go to this node's addresses with the domain as SNI, so they pass or fail
// independently of how far DNS has propagated.
func (p Prober) PostSwitch(ctx context.Context, domain, healthPath string, targets []string) []Check {
	st := p.Resolver.Lookup(ctx, domain)
	checks := []Check{dnsResolvesHere(domain, st, targets)}

	cert := p.probeTLS(ctx, domain, targets)
	checks = append(checks, cert)
	if cert.Status != CheckPass {
		return append(checks, Check{ID: "health", Status: CheckFail, Detail: "not tried, TLS failed first"})
	}
	return append(checks, p.probeHealth(ctx, domain, healthPath, targets))
}

func dnsResolvesHere(domain string, st DomainState, targets []string) Check {
	switch {
	case st.Error != "" && len(st.Addrs) == 0:
		return Check{ID: "resolves", Status: CheckFail, Detail: "could not resolve " + domain + ": " + st.Error}
	case pointsAtTarget(st.Addrs, targets):
		return Check{ID: "resolves", Status: CheckPass, Detail: domain + " resolves to this node"}
	}
	return Check{ID: "resolves", Status: CheckFail,
		Detail: fmt.Sprintf("%s resolves to %s, not this node (%s)", domain, strings.Join(st.Addrs, ", "), strings.Join(targets, ", ")),
		Fix:    "update the record, or wait out its TTL if you already changed it"}
}

func (p Prober) candidates(targets []string, port string) []string {
	out := make([]string, 0, len(targets)+1)
	for _, t := range targets {
		out = append(out, net.JoinHostPort(t, port))
	}
	return append(out, net.JoinHostPort("127.0.0.1", port))
}

func (p Prober) probeTLS(ctx context.Context, domain string, targets []string) Check {
	var lastErr error
	for _, addr := range p.candidates(targets, "443") {
		dctx, cancel := context.WithTimeout(ctx, p.timeout())
		raw, err := p.dial()(dctx, "tcp", addr)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		conn := tls.Client(raw, &tls.Config{ServerName: domain, MinVersion: tls.VersionTLS12, RootCAs: p.RootCAs})
		_ = conn.SetDeadline(time.Now().Add(p.timeout()))
		err = conn.HandshakeContext(ctx)
		if err != nil {
			_ = raw.Close()
			return Check{ID: "certificate", Status: CheckFail, Detail: "no trusted certificate for " + domain + ": " + err.Error(),
				Fix: "make sure ports 80 and 443 reach this node and the domain points here, then retry, certificates are issued on first use"}
		}
		certs := conn.ConnectionState().PeerCertificates
		_ = conn.Close()
		d := "a trusted certificate is served"
		if len(certs) > 0 {
			d = fmt.Sprintf("trusted certificate from %s, expires %s", certs[0].Issuer.CommonName, certs[0].NotAfter.UTC().Format("2006-01-02"))
		}
		return Check{ID: "certificate", Status: CheckPass, Detail: d}
	}
	return Check{ID: "certificate", Status: CheckFail, Detail: "could not connect to port 443 on this node: " + errString(lastErr),
		Fix: "open port 443 on this node and make sure the ingress is running"}
}

func (p Prober) probeHealth(ctx context.Context, domain, path string, targets []string) Check {
	if path == "" || !strings.HasPrefix(path, "/") {
		path = "/"
	}
	var lastErr error
	for _, addr := range p.candidates(targets, "443") {
		tr := &http.Transport{
			DialContext:     func(c context.Context, n, _ string) (net.Conn, error) { return p.dial()(c, n, addr) },
			TLSClientConfig: &tls.Config{ServerName: domain, MinVersion: tls.VersionTLS12, RootCAs: p.RootCAs},
		}
		client := &http.Client{Transport: tr, Timeout: p.timeout(),
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+domain+path, nil)
		if err != nil {
			return Check{ID: "health", Status: CheckFail, Detail: err.Error()}
		}
		resp, err := client.Do(req)
		tr.CloseIdleConnections()
		if err != nil {
			lastErr = err
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return Check{ID: "health", Status: CheckPass, Detail: fmt.Sprintf("GET %s answered 200", path)}
		}
		return Check{ID: "health", Status: CheckFail, Detail: fmt.Sprintf("GET %s answered %d", path, resp.StatusCode),
			Fix: "check the app's readiness path and logs"}
	}
	return Check{ID: "health", Status: CheckFail, Detail: "request failed: " + errString(lastErr)}
}

func errString(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}
