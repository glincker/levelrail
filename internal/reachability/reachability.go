// Package reachability checks which listening ports a host serves locally
// and, only when the operator opts in, which of them the internet can reach.
package reachability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"syscall"
	"time"
)

// Limits for the optional dial-back request.
const (
	DialbackTimeout          = 8 * time.Second
	DialbackMaxResponseBytes = 64 << 10
	DialbackMaxPorts         = 16
	LocalDialTimeout         = 1500 * time.Millisecond
)

// Verdicts for one port seen from outside.
const (
	ExternalNotChecked  = "not_checked"
	ExternalReachable   = "reachable"
	ExternalUnreachable = "unreachable"
	ExternalUnknown     = "unknown"
)

// ErrDialbackURL is returned for a dial-back URL that must not be contacted.
var ErrDialbackURL = errors.New("dial-back url refused")

// Dialer is the seam for local port probes.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// Target is one port the control plane or ingress is configured to listen on.
type Target struct {
	Name         string `json:"name"`
	Port         int    `json:"port"`
	LoopbackOnly bool   `json:"loopback_only,omitempty"`
}

// Probe is one local dial.
type Probe struct {
	Address   string `json:"address"`
	Listening bool   `json:"listening"`
}

// PortReport is the result for one target.
type PortReport struct {
	Target
	Listening bool    `json:"listening"`
	Probes    []Probe `json:"probes"`
	External  string  `json:"external"`
	Advice    string  `json:"advice,omitempty"`
}

// ProbeLocal dials 127.0.0.1 and the primary interface address (when known) for each target.
func ProbeLocal(ctx context.Context, d Dialer, targets []Target, primary string) []PortReport {
	out := make([]PortReport, 0, len(targets))
	for _, t := range targets {
		r := PortReport{Target: t, External: ExternalNotChecked}
		hosts := []string{"127.0.0.1"}
		if primary != "" {
			hosts = append(hosts, primary)
		}
		for _, h := range hosts {
			addr := net.JoinHostPort(h, strconv.Itoa(t.Port))
			ok := dialOnce(ctx, d, addr)
			r.Probes = append(r.Probes, Probe{Address: addr, Listening: ok})
			if ok {
				r.Listening = true
			}
		}
		out = append(out, r)
	}
	return out
}

func dialOnce(ctx context.Context, d Dialer, addr string) bool {
	c, cancel := context.WithTimeout(ctx, LocalDialTimeout)
	defer cancel()
	conn, err := d.DialContext(c, "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Exposable reports whether the port should be reachable from outside the host.
func (p PortReport) Exposable() bool {
	if !p.Listening || p.LoopbackOnly {
		return false
	}
	// With a primary address known, a port that answers only on loopback is bound privately.
	if len(p.Probes) > 1 {
		return p.Probes[1].Listening
	}
	return true
}

// ExposablePorts lists the ports worth asking a dial-back service about.
func ExposablePorts(rs []PortReport) []int {
	var ports []int
	for _, r := range rs {
		if r.Exposable() {
			ports = append(ports, r.Port)
		}
	}
	sort.Ints(ports)
	if len(ports) > DialbackMaxPorts {
		ports = ports[:DialbackMaxPorts]
	}
	return ports
}

// Advice is the plain-words next step for a port the internet cannot reach.
func Advice(port int) string {
	return fmt.Sprintf("Open TCP port %d inbound in your cloud firewall or security group (and in ufw or firewalld if enabled)", port)
}

// ApplyDialback folds a dial-back answer into the per-port reports.
func ApplyDialback(rs []PortReport, res DialbackResult) {
	reach := toSet(res.Reachable)
	unreach := toSet(res.Unreachable)
	for i := range rs {
		if !rs[i].Exposable() {
			continue
		}
		switch {
		case unreach[rs[i].Port]:
			rs[i].External = ExternalUnreachable
			rs[i].Advice = Advice(rs[i].Port)
		case reach[rs[i].Port]:
			rs[i].External = ExternalReachable
		default:
			rs[i].External = ExternalUnknown
		}
	}
}

func toSet(ports []int) map[int]bool {
	m := make(map[int]bool, len(ports))
	for _, p := range ports {
		m[p] = true
	}
	return m
}

// DialbackResult is the service answer.
type DialbackResult struct {
	Reachable   []int `json:"reachable"`
	Unreachable []int `json:"unreachable"`
}

// DialbackOptions tunes a dial-back call. Client nil builds the SSRF-safe default.
type DialbackOptions struct {
	AllowPrivate bool
	Client       *http.Client
	Resolve      func(ctx context.Context, host string) ([]netip.Addr, error)
}

// Dialback POSTs {ports:[...]} to rawURL. An empty URL contacts nothing.
func Dialback(ctx context.Context, rawURL string, ports []int, opt DialbackOptions) (DialbackResult, error) {
	var res DialbackResult
	if rawURL == "" || len(ports) == 0 {
		return res, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return res, fmt.Errorf("%w: must be an https URL", ErrDialbackURL)
	}
	if !opt.AllowPrivate {
		if err := checkPublicHost(ctx, u.Hostname(), opt.Resolve); err != nil {
			return res, err
		}
	}
	client := opt.Client
	if client == nil {
		client = safeClient(opt.AllowPrivate)
	}
	body, err := json.Marshal(map[string][]int{"ports": ports})
	if err != nil {
		return res, err
	}
	ctx, cancel := context.WithTimeout(ctx, DialbackTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return res, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return res, fmt.Errorf("dial-back request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("dial-back service answered %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, DialbackMaxResponseBytes+1))
	if err != nil {
		return res, fmt.Errorf("dial-back read: %w", err)
	}
	if len(raw) > DialbackMaxResponseBytes {
		return res, errors.New("dial-back response too large")
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return res, fmt.Errorf("dial-back response: %w", err)
	}
	return res, nil
}

func checkPublicHost(ctx context.Context, host string, resolve func(context.Context, string) ([]netip.Addr, error)) error {
	var addrs []netip.Addr
	if a, err := netip.ParseAddr(host); err == nil {
		addrs = []netip.Addr{a}
	} else {
		if resolve == nil {
			resolve = func(ctx context.Context, h string) ([]netip.Addr, error) {
				return net.DefaultResolver.LookupNetIP(ctx, "ip", h)
			}
		}
		var rerr error
		if addrs, rerr = resolve(ctx, host); rerr != nil || len(addrs) == 0 {
			return fmt.Errorf("%w: host does not resolve", ErrDialbackURL)
		}
	}
	for _, a := range addrs {
		if isPrivate(a) {
			return fmt.Errorf("%w: resolves to a private or loopback address", ErrDialbackURL)
		}
	}
	return nil
}

func isPrivate(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsMulticast() || a.IsUnspecified() || a.IsInterfaceLocalMulticast()
}

// safeClient re-checks the connected address so a DNS answer that changes between
// the pre-check and the dial cannot reach an internal host. Redirects are not followed.
func safeClient(allowPrivate bool) *http.Client {
	d := &net.Dialer{Timeout: DialbackTimeout}
	if !allowPrivate {
		d.Control = func(_, address string, _ syscall.RawConn) error {
			h, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			a, err := netip.ParseAddr(h)
			if err != nil || isPrivate(a) {
				return fmt.Errorf("%w: connected address is private", ErrDialbackURL)
			}
			return nil
		}
	}
	return &http.Client{
		Timeout:       DialbackTimeout,
		Transport:     &http.Transport{DialContext: d.DialContext, Proxy: nil},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
