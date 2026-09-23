package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// doctorHTTPDoer is the single-method http.Client seam GET
// /api/v1/system/doctor's network checks share, the same "fake instead
// of a real client" shape firewallCommandRunner already establishes for
// exec.Command. Satisfied by *http.Client.
type doctorHTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// doctorDialContextFunc dials a raw TCP connection for
// external_reachability_*. Satisfied by (&net.Dialer{}).DialContext.
type doctorDialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

const (
	// defaultDoctorNetworkTimeout bounds the whole group of network
	// checks below, which run concurrently: a stuck outbound request
	// must never make GET /api/v1/system/doctor itself slow.
	defaultDoctorNetworkTimeout = 5 * time.Second

	// defaultDoctorPublicIPEndpoint is a plain-text "what's my IP"
	// endpoint, queried by public_ip and, transitively, by
	// external_reachability_* and clock_skew.
	defaultDoctorPublicIPEndpoint = "https://api.ipify.org"

	// defaultDoctorClockSkewWarnAge is how far this host's clock may
	// drift from a remote HTTP Date header before clock_skew warns.
	// Wider than a strict NTP budget: the Date header only has
	// one-second resolution and ordinary request latency adds noise,
	// and the failure this check exists to catch (certificate
	// validation rejecting a connection over a badly wrong clock) only
	// bites at a much coarser scale than a few seconds.
	defaultDoctorClockSkewWarnAge = 5 * time.Minute

	// defaultDoctorACMEDirectoryURL mirrors internal/ingress.ACMEIssuer.
	// CA's own doc comment: Caddy's compiled-in default when no
	// override is configured, Let's Encrypt's real production
	// directory.
	defaultDoctorACMEDirectoryURL = "https://acme-v02.api.letsencrypt.org/directory"
)

func (rt *Router) doctorHTTPClientOrDefault() doctorHTTPDoer {
	if rt.doctorHTTPClient != nil {
		return rt.doctorHTTPClient
	}
	return http.DefaultClient
}

func (rt *Router) doctorDialContextOrDefault() doctorDialContextFunc {
	if rt.doctorDialContext != nil {
		return rt.doctorDialContext
	}
	return (&net.Dialer{}).DialContext
}

func (rt *Router) doctorNetworkTimeoutOrDefault() time.Duration {
	if rt.doctorNetworkTimeout > 0 {
		return rt.doctorNetworkTimeout
	}
	return defaultDoctorNetworkTimeout
}

// doctorRunNetworkChecks runs every outbound-network doctor check
// concurrently under one shared, bounded timeout, so one slow or
// blackholed check can never make the whole doctor response slow.
// external_reachability_* needs the public IP public_ip itself detects,
// so that one runs first; the rest run in parallel once it resolves (or
// fails to).
func (rt *Router) doctorRunNetworkChecks(ctx context.Context, httpPort, httpsPort int) []doctorCheckResource {
	netCtx, cancel := context.WithTimeout(ctx, rt.doctorNetworkTimeoutOrDefault())
	defer cancel()

	publicIPCheck, publicIP := rt.doctorCheckPublicIP(netCtx)

	rest := make([]doctorCheckResource, 4)
	var wg sync.WaitGroup
	wg.Add(len(rest))
	go func() { defer wg.Done(); rest[0] = rt.doctorCheckExternalReachability(netCtx, publicIP, httpPort) }()
	go func() { defer wg.Done(); rest[1] = rt.doctorCheckExternalReachability(netCtx, publicIP, httpsPort) }()
	go func() { defer wg.Done(); rest[2] = rt.doctorCheckACMEReachability(netCtx) }()
	go func() { defer wg.Done(); rest[3] = rt.doctorCheckClockSkew(netCtx) }()
	wg.Wait()

	return append([]doctorCheckResource{publicIPCheck}, rest...)
}

// doctorCheckPublicIP detects this host's outbound-facing public IP.
// Degrades to unknown, never fail, on any error: a host with no
// outbound internet access (air-gapped, or an egress proxy that blocks
// this endpoint) is a real, supported deployment, not a doctor failure.
func (rt *Router) doctorCheckPublicIP(ctx context.Context) (check doctorCheckResource, ip string) {
	const code, name = "public_ip", "Public IP address"
	endpoint := rt.doctorPublicIPEndpoint
	if endpoint == "" {
		endpoint = defaultDoctorPublicIPEndpoint
	}

	ip, err := doctorFetchPublicIP(ctx, rt.doctorHTTPClientOrDefault(), endpoint)
	if err != nil {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusUnknown,
			Message: fmt.Sprintf("could not detect a public IP, likely offline or outbound HTTPS is blocked: %s", err),
		}, ""
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "detected " + ip}, ip
}

// doctorFetchPublicIP GETs endpoint and parses its plain-text IP body.
func doctorFetchPublicIP(ctx context.Context, client doctorHTTPDoer, endpoint string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	ip := strings.TrimSpace(string(body))
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("endpoint did not return a valid IP: %q", ip)
	}
	return ip, nil
}

// doctorCheckExternalReachability dials publicIP:port from this host
// itself, the only self-test available without a third-party "check my
// port" service. A successful dial is a real, positive signal. A failed
// one is not: many home and small-office routers don't support NAT
// hairpinning (a LAN host reaching its own public IP), so a dial
// failure here proves nothing about reachability from the real internet
// and must never be reported as fail.
func (rt *Router) doctorCheckExternalReachability(ctx context.Context, publicIP string, port int) doctorCheckResource {
	code := fmt.Sprintf("external_reachability_%d", port)
	name := fmt.Sprintf("External reachability (port %d)", port)
	const docsPath = "/troubleshooting#external-reachability-could-not-be-verified"

	if publicIP == "" {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusUnknown,
			Message:  "public IP could not be detected, so external reachability can't be tested from this host",
			DocsPath: docsPath,
		}
	}

	addr := net.JoinHostPort(publicIP, strconv.Itoa(port))
	conn, err := rt.doctorDialContextOrDefault()(ctx, "tcp", addr)
	if err != nil {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusUnknown,
			Message: fmt.Sprintf("could not verify from this host (%s); many routers block \"hairpin\" NAT loopback even when forwarding is set up correctly, so this does not mean the port is unreachable from the internet", err),
			Fix: fmt.Sprintf("Confirm your router or cloud firewall forwards port %d to this host, then verify from outside your network, e.g. https://canyouseeme.org/ or curl -m 5 http://%s:%d from a different network.",
				port, publicIP, port),
			DocsPath: docsPath,
		}
	}
	_ = conn.Close()
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%s is reachable", addr)}
}

// doctorCheckACMEReachability confirms this host can reach the ACME
// directory URL it will actually use: the platform's own configured
// override (store.IngressSettings.ACMEDirectoryURL) when set, else the
// same compiled-in default internal/ingress.NewACMEIssuer leaves Caddy
// to fall back to. Unreachable only fails outright when real ACME is
// actually enabled; otherwise it's a heads-up, since the internal
// issuer never needs this.
func (rt *Router) doctorCheckACMEReachability(ctx context.Context) doctorCheckResource {
	const code, name = "acme_reachability", "Outbound ACME reachability"
	const docsPath = "/acme-verification-runbook#prerequisites"

	directoryURL := defaultDoctorACMEDirectoryURL
	acmeEnabled := false
	if rt.ingressSettings != nil {
		if settings, err := rt.ingressSettings.GetIngressSettings(ctx); err == nil {
			acmeEnabled = settings.ACMEEnabled
			if settings.ACMEDirectoryURL != "" {
				directoryURL = settings.ACMEDirectoryURL
			}
		}
	}
	unreachableStatus := doctorStatusWarn
	if acmeEnabled {
		unreachableStatus = doctorStatusFail
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, directoryURL, nil)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: err.Error()}
	}
	resp, err := rt.doctorHTTPClientOrDefault().Do(req)
	if err != nil {
		return doctorCheckResource{
			Code: code, Name: name, Status: unreachableStatus,
			Message:  fmt.Sprintf("could not reach %s: %s", directoryURL, err),
			Fix:      "Confirm this host can make outbound HTTPS connections (egress firewall rules, any HTTP(S)_PROXY settings). A real TLS certificate can't be issued or renewed without reaching the ACME CA.",
			DocsPath: docsPath,
		}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return doctorCheckResource{
			Code: code, Name: name, Status: unreachableStatus,
			Message:  fmt.Sprintf("%s returned status %d", directoryURL, resp.StatusCode),
			Fix:      "Check the configured ACME directory URL under Settings > Ingress.",
			DocsPath: docsPath,
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "reachable"}
}

// doctorCheckClockSkew compares this host's clock against the remote
// HTTP Date header from the same public-IP endpoint public_ip uses, an
// independent request run purely for its Date header, so this check
// works the same whether or not this instance ever touches ACME.
func (rt *Router) doctorCheckClockSkew(ctx context.Context) doctorCheckResource {
	const code, name = "clock_skew", "Clock skew"
	endpoint := rt.doctorPublicIPEndpoint
	if endpoint == "" {
		endpoint = defaultDoctorPublicIPEndpoint
	}

	remoteTime, err := doctorFetchRemoteTime(ctx, rt.doctorHTTPClientOrDefault(), endpoint)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not determine remote time: %s", err)}
	}

	skew := time.Since(remoteTime)
	if skew < 0 {
		skew = -skew
	}
	threshold := rt.doctorClockSkewWarnAge
	if threshold <= 0 {
		threshold = defaultDoctorClockSkewWarnAge
	}
	if skew > threshold {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message:  fmt.Sprintf("off by about %s from a remote reference, above the %s warning threshold", skew.Round(time.Second), threshold),
			Fix:      "Install and enable an NTP client (chronyd or systemd-timesyncd both work) so this host's clock stays in sync automatically.",
			DocsPath: "/troubleshooting#clock-skew",
		}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("off by about %s", skew.Round(time.Second))}
}

// doctorFetchRemoteTime GETs endpoint and parses its response's Date
// header, discarding the body: callers only need the header.
func doctorFetchRemoteTime(ctx context.Context, client doctorHTTPDoer, endpoint string) (time.Time, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return time.Time{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 256))

	dateHeader := resp.Header.Get("Date")
	if dateHeader == "" {
		return time.Time{}, fmt.Errorf("response had no Date header")
	}
	t, err := http.ParseTime(dateHeader)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse Date header %q: %w", dateHeader, err)
	}
	return t, nil
}
