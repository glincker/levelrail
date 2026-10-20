package platformimport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Environment variables that must be set before a request may opt in to
// private or loopback source addresses. Both the env var and the request
// field are required, so a leaked API token alone cannot aim the control
// plane at the internal network.
const (
	AllowPrivateEnv  = "APP_IMPORT_ALLOW_PRIVATE_NETWORKS"
	AllowLoopbackEnv = "APP_IMPORT_ALLOW_LOOPBACK"
)

const (
	maxResponseBytes = 8 << 20
	requestTimeout   = 30 * time.Second
	// MaxPages bounds every paginated or fan-out walk.
	MaxPages = 200
)

// ErrBlockedAddress is returned when the source URL resolves to an address
// the network policy forbids.
var ErrBlockedAddress = errors.New("platformimport: source address is not allowed")

// NetworkPolicy decides which destinations the source client may dial.
// Cloud metadata and link-local addresses are never allowed.
type NetworkPolicy struct {
	AllowPrivate  bool
	AllowLoopback bool
}

var (
	metadataPrefixes = []netip.Prefix{
		netip.MustParsePrefix("169.254.0.0/16"),
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("fd00:ec2::/32"),
		netip.MustParsePrefix("100.100.100.200/32"),
	}
	internalPrefixes = []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("fec0::/10"),
	}
)

// Check reports whether addr may be dialed under the policy.
func (p NetworkPolicy) Check(addr netip.Addr) error {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsMulticast() {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, addr)
	}
	for _, pre := range metadataPrefixes {
		if pre.Contains(addr) {
			return fmt.Errorf("%w: %s is a metadata or link-local address and can never be used", ErrBlockedAddress, addr)
		}
	}
	if addr.IsLoopback() {
		if p.AllowLoopback {
			return nil
		}
		return fmt.Errorf("%w: %s is loopback (set %s=true and allow_loopback to permit)", ErrBlockedAddress, addr, AllowLoopbackEnv)
	}
	internal := addr.IsPrivate()
	for _, pre := range internalPrefixes {
		if pre.Contains(addr) {
			internal = true
		}
	}
	if internal && !p.AllowPrivate {
		return fmt.Errorf("%w: %s is a private address (set %s=true and allow_private to permit)", ErrBlockedAddress, addr, AllowPrivateEnv)
	}
	return nil
}

func (p NetworkPolicy) control(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("parse dial address: %w", err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("parse dial address: %w", err)
	}
	return p.Check(ip)
}

// ClientOptions configure the source HTTP client.
type ClientOptions struct {
	Policy   NetworkPolicy
	Insecure bool
}

// newHTTPClient builds a client that checks every dialed IP (including
// after redirects and DNS rebinding) and ignores proxy settings.
func newHTTPClient(o ClientOptions) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, Control: o.Policy.control}
	return &http.Client{
		Timeout: requestTimeout,
		Transport: &http.Transport{
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: o.Insecure}, //nolint:gosec // explicit operator opt-in for self-signed sources
		},
		// Source credentials travel in custom headers Go copies onto every
		// redirect hop, so redirects are never followed.
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			return fmt.Errorf("source redirected to %s://%s; use that address as the source URL", req.URL.Scheme, req.URL.Host)
		},
	}
}

// requester performs the source platform's HTTP calls. Only GET is ever
// issued, plus one explicit login POST for CapRover.
type requester struct {
	hc      *http.Client
	base    string
	headers map[string]string
	secrets []string
}

func newRequester(baseURL string, o ClientOptions, secrets ...string) (*requester, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("source url must be an absolute http or https URL")
	}
	if u.User != nil {
		return nil, errors.New("source url must not embed credentials")
	}
	u.RawQuery, u.Fragment = "", ""
	return &requester{hc: newHTTPClient(o), base: u.String(), headers: map[string]string{}, secrets: secrets}, nil
}

func (r *requester) redact(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, s := range r.secrets {
		if s != "" {
			msg = strings.ReplaceAll(msg, s, "[redacted]")
		}
	}
	return errors.New(msg)
}

// APIError is a non-2xx reply from the source.
type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("source returned HTTP %d: %s", e.Status, e.Body)
}

func (r *requester) do(ctx context.Context, method, path string, query url.Values, body io.Reader, out any) error {
	u := r.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return r.redact(fmt.Errorf("build request: %w", err))
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range r.headers {
		req.Header.Set(k, v)
	}
	resp, err := r.hc.Do(req)
	if err != nil {
		return r.redact(fmt.Errorf("request %s %s: %w", method, path, err))
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return r.redact(fmt.Errorf("read response: %w", err))
	}
	if len(data) > maxResponseBytes {
		return fmt.Errorf("response for %s exceeds %d bytes", path, maxResponseBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet := strings.TrimSpace(string(data))
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return r.redact(&APIError{Status: resp.StatusCode, Body: snippet})
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response for %s: %w", path, err)
	}
	return nil
}

func (r *requester) get(ctx context.Context, path string, query url.Values, out any) error {
	return r.do(ctx, http.MethodGet, path, query, nil, out)
}
