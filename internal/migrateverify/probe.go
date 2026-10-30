package migrateverify

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

// Env names that tune the probes and the thresholds that classify them.
const (
	EnvProbeTimeout      = "APP_MIGRATE_VERIFY_PROBE_TIMEOUT"
	EnvProbeConcurrency  = "APP_MIGRATE_VERIFY_PROBE_CONCURRENCY"
	EnvAllowPrivateProbe = "APP_MIGRATE_VERIFY_ALLOW_PRIVATE_PROBES"
	EnvTLSWarnDays       = "APP_MIGRATE_VERIFY_TLS_WARN_DAYS"
	EnvLatencyFactor     = "APP_MIGRATE_VERIFY_LATENCY_FACTOR"
	EnvLatencyFloorMS    = "APP_MIGRATE_VERIFY_LATENCY_FLOOR_MS"
)

// Fallbacks used when the env var is unset or invalid.
const (
	DefaultProbeTimeout     = 10 * time.Second
	DefaultProbeConcurrency = 6
	MaxProbeTimeout         = 60 * time.Second
	MaxProbeConcurrency     = 32
	defaultTLSWarnDays      = 14
	defaultLatencyFactor    = 3.0
	defaultLatencyFloorMS   = 300
)

// ErrBlockedAddress is returned when a probe would dial a private or loopback address.
var ErrBlockedAddress = platformimport.ErrBlockedAddress

// ProbeResult is what one side answered for a domain.
type ProbeResult struct {
	URL            string `json:"url"`
	Status         int    `json:"status,omitempty"`
	LatencyMS      int64  `json:"latency_ms,omitempty"`
	TLS            bool   `json:"tls"`
	TLSValid       bool   `json:"tls_valid"`
	TLSDaysLeft    int    `json:"tls_days_left,omitempty"`
	TLSError       string `json:"tls_error,omitempty"`
	RedirectTarget string `json:"redirect_target,omitempty"`
	// RedirectOffDomain is true when the redirect leaves the probed host's domain family. It is never followed.
	RedirectOffDomain bool   `json:"redirect_off_domain,omitempty"`
	Error             string `json:"error,omitempty"`
}

// DomainProbe pairs both sides of one domain.
type DomainProbe struct {
	App    string      `json:"app"`
	Domain string      `json:"domain"`
	Staged bool        `json:"staged,omitempty"`
	Source ProbeResult `json:"source"`
	Target ProbeResult `json:"target"`
}

// ProbeOptions configure a Prober.
type ProbeOptions struct {
	Timeout     time.Duration
	Concurrency int
	// AllowPrivate lets probes reach private and loopback addresses. Metadata and link-local stay blocked.
	AllowPrivate bool
	// SourceAddr and TargetAddr, when set, are the IP (or ip:port) each side is dialed at while the
	// domain stays the Host and TLS name. Empty means normal DNS.
	SourceAddr string
	TargetAddr string
}

// ClampProbeOptions applies defaults and upper bounds.
func ClampProbeOptions(o ProbeOptions) ProbeOptions {
	if o.Timeout <= 0 {
		o.Timeout = DefaultProbeTimeout
	}
	if o.Timeout > MaxProbeTimeout {
		o.Timeout = MaxProbeTimeout
	}
	if o.Concurrency <= 0 {
		o.Concurrency = DefaultProbeConcurrency
	}
	if o.Concurrency > MaxProbeConcurrency {
		o.Concurrency = MaxProbeConcurrency
	}
	return o
}

// ValidateAddr checks a dial override: an IP, optionally with a port.
func ValidateAddr(addr string) error {
	if addr == "" {
		return nil
	}
	host := addr
	if h, port, err := net.SplitHostPort(addr); err == nil {
		host = h
		if port == "" {
			return errors.New("address has an empty port")
		}
	}
	if _, err := netip.ParseAddr(strings.Trim(host, "[]")); err != nil {
		return fmt.Errorf("%q is not an IP address", addr)
	}
	return nil
}

// Prober runs HTTP probes under the SSRF guard.
type Prober struct {
	Opts ProbeOptions
}

func (p Prober) policy() platformimport.NetworkPolicy {
	return platformimport.NetworkPolicy{AllowPrivate: p.Opts.AllowPrivate, AllowLoopback: p.Opts.AllowPrivate}
}

func dialControl(policy platformimport.NetworkPolicy) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("parse dial address: %w", err)
		}
		ip, err := netip.ParseAddr(host)
		if err != nil {
			return fmt.Errorf("parse dial address: %w", err)
		}
		return policy.Check(ip)
	}
}

func (p Prober) client(domain, override string, verify bool) *http.Client {
	dialer := &net.Dialer{Timeout: p.Opts.Timeout, Control: dialControl(p.policy())}
	return &http.Client{
		Timeout: p.Opts.Timeout,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if override != "" {
					addr = overrideAddr(override, addr)
				}
				return dialer.DialContext(ctx, network, addr)
			},
			TLSHandshakeTimeout: p.Opts.Timeout,
			DisableKeepAlives:   true,
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, ServerName: domain, InsecureSkipVerify: !verify}, //nolint:gosec // a failed verify is reported, then retried only to read the status
		},
		// A redirect is reported, never followed: the next hop could be anywhere.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func overrideAddr(override, original string) string {
	if _, _, err := net.SplitHostPort(override); err == nil {
		return override
	}
	_, port, err := net.SplitHostPort(original)
	if err != nil {
		port = "443"
	}
	return net.JoinHostPort(strings.Trim(override, "[]"), port)
}

// ValidDomain accepts a plain hostname, never a URL, port, wildcard or path.
func ValidDomain(d string) bool {
	if d == "" || len(d) > 253 || strings.ContainsAny(d, "/:@?# *\\") {
		return false
	}
	return strings.Contains(d, ".") || net.ParseIP(d) != nil
}

// Probe requests https://domain/ and falls back to http when nothing answers TLS.
func (p Prober) Probe(ctx context.Context, domain, override string) ProbeResult {
	p.Opts = ClampProbeOptions(p.Opts)
	if !ValidDomain(domain) {
		return ProbeResult{URL: "https://" + domain + "/", Error: "not a probeable hostname"}
	}
	res := p.once(ctx, "https", domain, override, true)
	if res.TLSError != "" && res.Status == 0 && isCertError(res.TLSError) {
		insecure := p.once(ctx, "https", domain, override, false)
		insecure.TLSError = res.TLSError
		insecure.TLSValid = false
		return insecure
	}
	if res.Status == 0 && res.TLSError == "" && !errors.Is(ctx.Err(), context.Canceled) && !strings.Contains(res.Error, ErrBlockedAddress.Error()) {
		if plain := p.once(ctx, "http", domain, override, true); plain.Status != 0 {
			return plain
		}
	}
	return res
}

func isCertError(msg string) bool {
	return strings.Contains(msg, "certificate") || strings.Contains(msg, "x509")
}

func (p Prober) once(ctx context.Context, scheme, domain, override string, verify bool) ProbeResult {
	res := ProbeResult{URL: scheme + "://" + domain + "/", TLS: scheme == "https"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, res.URL, nil)
	if err != nil {
		res.Error = "build request: " + err.Error()
		return res
	}
	req.Header.Set("User-Agent", "migrate-verify/1")
	start := time.Now()
	resp, err := p.client(domain, override, verify).Do(req)
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		msg := scrubURLError(err)
		res.Error = msg
		var cert x509.UnknownAuthorityError
		var hostErr x509.HostnameError
		var invalid x509.CertificateInvalidError
		if errors.As(err, &cert) || errors.As(err, &hostErr) || errors.As(err, &invalid) || isCertError(msg) {
			res.TLSError = msg
		}
		return res
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	res.Status = resp.StatusCode
	if scheme == "https" && resp.TLS != nil {
		res.TLSValid = verify
		if len(resp.TLS.PeerCertificates) > 0 {
			res.TLSDaysLeft = int(time.Until(resp.TLS.PeerCertificates[0].NotAfter).Hours() / 24)
		}
	}
	if loc := resp.Header.Get("Location"); loc != "" && resp.StatusCode >= 300 && resp.StatusCode < 400 {
		res.RedirectTarget, res.RedirectOffDomain = redirectSummary(resp.Request.URL, loc)
	}
	return res
}

func scrubURLError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err.Error()
	}
	return err.Error()
}

// redirectSummary resolves Location against base and drops the query, which can carry tokens.
func redirectSummary(base *url.URL, location string) (target string, offDomain bool) {
	next, err := base.Parse(location)
	if err != nil {
		return "(unparseable redirect)", true
	}
	return next.Scheme + "://" + next.Host + next.Path, !SameDomainFamily(base.Hostname(), next.Hostname())
}

// SameDomainFamily reports whether two hosts share their last two labels
// (or are equal), so www and apex count as one family and another site does not.
func SameDomainFamily(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSuffix(a, ".")), strings.ToLower(strings.TrimSuffix(b, "."))
	if a == b {
		return true
	}
	return familyRoot(a) != "" && familyRoot(a) == familyRoot(b)
}

func familyRoot(host string) string {
	if net.ParseIP(host) != nil {
		return ""
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return ""
	}
	return strings.Join(labels[len(labels)-2:], ".")
}

// Pair is one domain to probe on both sides.
type Pair struct {
	App    string
	Domain string
	Staged bool
}

// ProbeAll probes every pair on both sides with bounded concurrency. Results keep the input order.
func (p Prober) ProbeAll(ctx context.Context, pairs []Pair) []DomainProbe {
	p.Opts = ClampProbeOptions(p.Opts)
	out := make([]DomainProbe, len(pairs))
	sem := make(chan struct{}, p.Opts.Concurrency)
	var wg sync.WaitGroup
	for i, pr := range pairs {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = DomainProbe{App: pr.App, Domain: pr.Domain, Staged: pr.Staged,
				Source: p.Probe(ctx, pr.Domain, p.Opts.SourceAddr),
				Target: p.Probe(ctx, pr.Domain, p.Opts.TargetAddr)}
		}()
	}
	wg.Wait()
	return out
}
