package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	proxyIntegrationPath = "/api/v1/system/proxy-integration"
	probeDialTimeout     = 4 * time.Second
	probeHTTPTimeout     = 5 * time.Second
	proxyCertMaxText     = 120
)

// tlsProbeResult is what a TLS handshake with SNI showed.
type tlsProbeResult struct {
	Issuer   string
	NotAfter time.Time
	Trusted  bool
}

// domainAutoSeams are the external touchpoints of the go-live flow; a zero
// value uses the real ones, tests replace single funcs.
type domainAutoSeams struct {
	resolveTarget func(ctx context.Context, domain string) (*dnsTarget, error)
	tlsProbe      func(ctx context.Context, dialAddr, sni string) (tlsProbeResult, error)
	httpProbe     func(ctx context.Context, rawURL, dialAddr string) (int, error)
	proxy         func(ctx context.Context, r *http.Request) *proxyIntegrationView

	handlerMu sync.Mutex
	handler   http.Handler
}

// proxyDomainState is one domain's row of the managed proxy contract.
type proxyDomainState struct {
	State       string
	Certificate string
	Reachable   bool
}

// proxyIntegrationView is the parsed GET /api/v1/system/proxy-integration.
type proxyIntegrationView struct {
	Enabled bool
	Domains map[string]proxyDomainState
}

type proxyIntegrationWire struct {
	Settings struct {
		Integration string `json:"integration"`
	} `json:"settings"`
	Domains []struct {
		Domain      string          `json:"domain"`
		State       string          `json:"state"`
		Certificate json.RawMessage `json:"certificate"`
		Reachable   bool            `json:"reachable"`
	} `json:"domains"`
}

func certificateText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	text := string(raw)
	if len(text) > proxyCertMaxText {
		text = text[:proxyCertMaxText]
	}
	return text
}

// goLiveProxyView calls the managed proxy contract in process and returns
// nil when the endpoint is absent or the integration is off.
func (rt *Router) goLiveProxyView(ctx context.Context, r *http.Request) *proxyIntegrationView {
	if rt.domainAuto.proxy != nil {
		return rt.domainAuto.proxy(ctx, r)
	}
	if r == nil {
		return nil
	}
	rt.domainAuto.handlerMu.Lock()
	if rt.domainAuto.handler == nil {
		rt.domainAuto.handler = rt.Handler()
	}
	h := rt.domainAuto.handler
	rt.domainAuto.handlerMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, proxyIntegrationPath, nil)
	if err != nil {
		return nil
	}
	for _, k := range []string{"Authorization", "Cookie"} {
		if v := r.Header.Get(k); v != "" {
			req.Header.Set(k, v)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Header().Get("Content-Type"), "json") {
		return nil
	}
	var wire proxyIntegrationWire
	if err := json.Unmarshal(rec.Body.Bytes(), &wire); err != nil {
		return nil
	}
	mode := strings.ToLower(strings.TrimSpace(wire.Settings.Integration))
	if mode == "" || mode == "off" {
		return nil
	}
	view := &proxyIntegrationView{Enabled: true, Domains: map[string]proxyDomainState{}}
	for _, d := range wire.Domains {
		view.Domains[strings.ToLower(d.Domain)] = proxyDomainState{State: d.State, Certificate: certificateText(d.Certificate), Reachable: d.Reachable}
	}
	return view
}

func defaultTLSProbe(ctx context.Context, dialAddr, sni string) (tlsProbeResult, error) {
	d := net.Dialer{Timeout: probeDialTimeout}
	raw, err := d.DialContext(ctx, "tcp", dialAddr)
	if err != nil {
		return tlsProbeResult{}, err
	}
	defer func() { _ = raw.Close() }()
	conn := tls.Client(raw, &tls.Config{ServerName: sni, MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}) //nolint:gosec // reads the served certificate; trust is evaluated separately below
	hsCtx, cancel := context.WithTimeout(ctx, probeDialTimeout)
	defer cancel()
	if err := conn.HandshakeContext(hsCtx); err != nil {
		return tlsProbeResult{}, err
	}
	certs := conn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return tlsProbeResult{}, net.ErrClosed
	}
	leaf := certs[0]
	pool := x509.NewCertPool()
	for _, c := range certs[1:] {
		pool.AddCert(c)
	}
	_, verr := leaf.Verify(x509.VerifyOptions{DNSName: sni, Intermediates: pool})
	issuer := leaf.Issuer.CommonName
	if len(leaf.Issuer.Organization) > 0 {
		issuer = leaf.Issuer.Organization[0]
		if leaf.Issuer.CommonName != "" {
			issuer += " " + leaf.Issuer.CommonName
		}
	}
	return tlsProbeResult{Issuer: issuer, NotAfter: leaf.NotAfter, Trusted: verr == nil}, nil
}

func defaultHTTPProbe(ctx context.Context, rawURL, dialAddr string) (int, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}, //nolint:gosec // status probe only, certificate trust is its own step
	}
	if dialAddr != "" {
		d := net.Dialer{Timeout: probeDialTimeout}
		tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return d.DialContext(ctx, network, dialAddr)
		}
	}
	client := &http.Client{
		Transport:     tr,
		Timeout:       probeHTTPTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer tr.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}

// probeDialAddr is where the TLS and HTTP probes connect: this server's own
// ingress (so a stale local resolver cannot hide a working route), or the
// public name when a proxy upstream owns TLS.
func (rt *Router) probeDialAddr(domain string, upstreamTLS bool, publicPort int) string {
	if upstreamTLS {
		port := publicPort
		if port == 0 {
			port = 443
		}
		return net.JoinHostPort(domain, strconv.Itoa(port))
	}
	port := rt.doctorHTTPSPort
	if port == 0 {
		port = defaultDoctorHTTPSPort
	}
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}
