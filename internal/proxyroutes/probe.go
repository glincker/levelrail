package proxyroutes

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProbeTarget is one end to end check: a TLS handshake to Address with SNI
// Domain, then GET Path with the Host header.
type ProbeTarget struct {
	Domain  string
	Address string
	Path    string
	Timeout time.Duration
	// Roots overrides the system roots, for tests.
	Roots *x509.CertPool
}

// ProbeResult is the outcome of Probe.
type ProbeResult struct {
	Reachable       bool
	StatusCode      int
	CertIssuer      string
	CertTrusted     bool
	CertLetsEncrypt bool
	Err             string
}

// traefikNotFound is the body Traefik answers with when no router matched.
const traefikNotFound = "404 page not found"

// Probe dials Address (never what DNS says) so a rebinding answer cannot
// point the probe elsewhere, and records the certificate even when it is not
// trusted yet, which is the normal state while ACME is still issuing.
func Probe(ctx context.Context, t ProbeTarget) ProbeResult {
	var res ProbeResult
	ctx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: t.Domain,
		// Verified by hand below so an untrusted certificate is reported, not fatal.
		InsecureSkipVerify: true, //nolint:gosec // VerifyConnection does the verification and records the outcome
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("no certificate presented")
			}
			leaf := cs.PeerCertificates[0]
			res.CertIssuer = issuerName(leaf)
			res.CertLetsEncrypt = isLetsEncrypt(leaf)
			inter := x509.NewCertPool()
			for _, c := range cs.PeerCertificates[1:] {
				inter.AddCert(c)
			}
			_, err := leaf.Verify(x509.VerifyOptions{DNSName: t.Domain, Intermediates: inter, Roots: t.Roots})
			res.CertTrusted = err == nil
			return nil
		},
	}
	dialer := &net.Dialer{}
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return dialer.DialContext(ctx, network, t.Address)
			},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	u := url.URL{Scheme: "https", Host: t.Domain, Path: t.Path}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		res.Err = "invalid domain"
		return res
	}
	resp, err := client.Do(req) //nolint:gosec // dialed address is fixed above, the URL only carries SNI and Host
	if err != nil {
		res.Err = "could not reach https://" + t.Domain + " through " + t.Address + ": " + err.Error()
		return res
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
	res.StatusCode = resp.StatusCode
	switch {
	case resp.StatusCode == http.StatusNotFound && strings.TrimSpace(string(body)) == traefikNotFound:
		res.Err = "the proxy has no route for this domain yet (it answered with its own 404)"
	case resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout:
		res.Err = fmt.Sprintf("the proxy could not reach this server (status %d): check the upstream host and that the listener accepts connections from the proxy", resp.StatusCode)
	case resp.StatusCode >= http.StatusInternalServerError:
		res.Err = fmt.Sprintf("the route answered with status %d", resp.StatusCode)
	default:
		res.Reachable = true
	}
	if res.Reachable && !res.CertTrusted {
		res.Err = "reachable, but the certificate (issued by " + res.CertIssuer + ") is not trusted yet: the proxy may still be requesting one from Let's Encrypt"
	}
	return res
}

func issuerName(c *x509.Certificate) string {
	if c.Issuer.CommonName != "" {
		return c.Issuer.CommonName
	}
	if len(c.Issuer.Organization) > 0 {
		return c.Issuer.Organization[0]
	}
	return "unknown"
}

func isLetsEncrypt(c *x509.Certificate) bool {
	for _, o := range c.Issuer.Organization {
		if strings.EqualFold(o, "Let's Encrypt") {
			return true
		}
	}
	return false
}

// RouterLoaded asks Traefik's API at base (http://127.0.0.1:8080) whether
// router is loaded and enabled. Only used when the API is insecure and local.
func RouterLoaded(ctx context.Context, base, router string, timeout time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+"/api/http/routers/"+url.PathEscape(router), nil)
	if err != nil {
		return false, err
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // base is a fixed loopback address
	if err != nil {
		return false, fmt.Errorf("traefik api: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("traefik api answered %d", resp.StatusCode)
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return false, fmt.Errorf("traefik api: %w", err)
	}
	return body.Status == "enabled", nil
}
