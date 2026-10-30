// Package mailcert checks, validates and installs the TLS certificate the
// instance-owned mail server serves (ADR 062). It holds no transport of its
// own: the API supplies the volume, the restart and the probe.
package mailcert

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

// Paths inside the mail config volume, relative to the directory mox reads its config from.
const (
	CertPath     = "certs/managed.pem"
	KeyPath      = "certs/managed.key"
	PrevCertPath = "certs/managed.prev.pem"
	PrevKeyPath  = "certs/managed.prev.key"
)

// SubmissionTLSPort is the implicit TLS submission port probed for the served certificate.
const SubmissionTLSPort = 465

const (
	envWindow        = "APP_MAIL_RESTART_WINDOW"
	envProbeTimeout  = "APP_MAIL_CERT_PROBE_TIMEOUT_SECONDS"
	envVerifySeconds = "APP_MAIL_CERT_VERIFY_SECONDS"
	envPollSeconds   = "APP_MAIL_CERT_POLL_SECONDS"
	envMaxPEMKB      = "APP_MAIL_CERT_MAX_PEM_KB"
)

// ErrACMENotImplemented is the seam for issuing through the ingress ACME (ADR 062, option A).
var ErrACMENotImplemented = errors.New("mail cert: obtaining a certificate through ACME is not yet implemented, upload one instead")

// ErrNotPrivateTarget means the probe address is not a private or loopback address.
var ErrNotPrivateTarget = errors.New("mail cert: the probe target is not a private address")

// ErrKeyModeTooWide means the node wrote the key wider than 0600, usually an agent without the private write option.
var ErrKeyModeTooWide = errors.New("mail cert: the node did not write the key with mode 0600, upgrade the node agent")

// Config holds the env driven knobs; defaults live here and nowhere else.
type Config struct {
	Window        Window
	ProbeTimeout  time.Duration
	VerifyTimeout time.Duration
	PollInterval  time.Duration
	MaxPEMBytes   int64
}

// ConfigFromEnv reads Config through lookup, keeping the defaults for unset or invalid values.
func ConfigFromEnv(lookup func(string) (string, bool)) (Config, error) {
	c := Config{ProbeTimeout: 5 * time.Second, VerifyTimeout: 90 * time.Second, PollInterval: 3 * time.Second, MaxPEMBytes: 64 << 10}
	secs := func(key string, dst *time.Duration) {
		if v, ok := lookup(key); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
				*dst = time.Duration(n) * time.Second
			}
		}
	}
	secs(envProbeTimeout, &c.ProbeTimeout)
	secs(envVerifySeconds, &c.VerifyTimeout)
	secs(envPollSeconds, &c.PollInterval)
	if v, ok := lookup(envMaxPEMKB); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			c.MaxPEMBytes = int64(n) << 10
		}
	}
	if v, ok := lookup(envWindow); ok {
		w, err := ParseWindow(v)
		if err != nil {
			return c, err
		}
		c.Window = w
	}
	return c, nil
}

// Window is a daily UTC quiet window "HH:MM-HH:MM"; the zero Window allows any time.
type Window struct {
	set        bool
	start, end int
}

// ParseWindow parses "HH:MM-HH:MM" (UTC); an empty string is the zero Window.
func ParseWindow(s string) (Window, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Window{}, nil
	}
	a, b, ok := strings.Cut(s, "-")
	start, err1 := parseClock(a)
	end, err2 := parseClock(b)
	if !ok || err1 != nil || err2 != nil || start == end {
		return Window{}, fmt.Errorf("mail cert: %s must look like 02:00-05:00 (UTC)", envWindow)
	}
	return Window{set: true, start: start, end: end}, nil
}

func parseClock(s string) (int, error) {
	t, err := time.Parse("15:04", strings.TrimSpace(s))
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}

// Contains reports whether t falls inside the window; a window may wrap past midnight.
func (w Window) Contains(t time.Time) bool {
	if !w.set {
		return true
	}
	u := t.UTC()
	m := u.Hour()*60 + u.Minute()
	if w.start < w.end {
		return m >= w.start && m < w.end
	}
	return m >= w.start || m < w.end
}

// String renders the window as configured, or "any time".
func (w Window) String() string {
	if !w.set {
		return "any time"
	}
	return fmt.Sprintf("%02d:%02d-%02d:%02d UTC", w.start/60, w.start%60, w.end/60, w.end%60)
}

// Info is the public description of one certificate: never any key material.
type Info struct {
	Subject    string
	Issuer     string
	NotAfter   time.Time
	Serial     string
	SHA256     string
	SelfSigned bool
}

// DaysLeft is the whole days from now until NotAfter, negative once expired.
func (i Info) DaysLeft(now time.Time) int {
	return int(i.NotAfter.Sub(now).Hours() / 24)
}

func describe(c *x509.Certificate) Info {
	sum := sha256.Sum256(c.Raw)
	self := bytes.Equal(c.RawIssuer, c.RawSubject) && c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature) == nil
	return Info{
		Subject: c.Subject.String(), Issuer: c.Issuer.String(), NotAfter: c.NotAfter.UTC(),
		Serial: c.SerialNumber.Text(16), SHA256: hex.EncodeToString(sum[:]), SelfSigned: self,
	}
}

// Validate checks an uploaded pair: only certificates in the chain, one
// unencrypted key that matches the leaf, the leaf valid now and covering host.
func Validate(certPEM, keyPEM []byte, host string, now time.Time) (Info, error) {
	if err := onlyBlocks(certPEM, "CERTIFICATE"); err != nil {
		return Info{}, fmt.Errorf("certificate chain: %w", err)
	}
	if err := singleKey(keyPEM); err != nil {
		return Info{}, err
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return Info{}, errors.New("the key does not match the certificate, or one of them is not valid PEM")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return Info{}, errors.New("the first certificate could not be parsed")
	}
	switch {
	case now.After(leaf.NotAfter):
		return Info{}, fmt.Errorf("the certificate expired on %s", leaf.NotAfter.UTC().Format(time.DateOnly))
	case now.Before(leaf.NotBefore):
		return Info{}, fmt.Errorf("the certificate is not valid before %s", leaf.NotBefore.UTC().Format(time.DateOnly))
	}
	if host == "" {
		return Info{}, errors.New("the mail host name is not set (MAIL_HOSTNAME)")
	}
	if err := leaf.VerifyHostname(host); err != nil {
		return Info{}, fmt.Errorf("the certificate does not cover the mail host %s", host)
	}
	return describe(leaf), nil
}

func onlyBlocks(data []byte, want string) error {
	n := 0
	for rest := data; ; {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		if b.Type != want {
			return fmt.Errorf("expected only %s blocks, found %s", want, b.Type)
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("no %s block found", want)
	}
	return nil
}

func singleKey(data []byte) error {
	b, rest := pem.Decode(data)
	if b == nil {
		return errors.New("private key: no PEM block found")
	}
	if next, _ := pem.Decode(rest); next != nil {
		return errors.New("private key: expected exactly one PEM block")
	}
	if strings.Contains(b.Type, "ENCRYPTED") || strings.Contains(b.Headers["Proc-Type"], "ENCRYPTED") {
		return errors.New("private key: the key is encrypted, upload it without a passphrase")
	}
	if !strings.HasSuffix(b.Type, "PRIVATE KEY") {
		return fmt.Errorf("private key: unexpected %s block", b.Type)
	}
	return nil
}

// Probe dials ip:port with implicit TLS and describes the served leaf. Only
// private or loopback targets are allowed and no credentials are sent.
func Probe(ctx context.Context, ip string, port int, timeout time.Duration) (Info, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil || (!addr.IsPrivate() && !addr.IsLoopback()) {
		return Info{}, ErrNotPrivateTarget
	}
	d := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: timeout},
		// The probe reads what is served, including self-signed and wrong-name certificates.
		Config: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}, //nolint:gosec // inspection only, nothing is sent
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := d.DialContext(cctx, "tcp", netip.AddrPortFrom(addr, uint16(port)).String()) //nolint:gosec // port is a fixed constant
	if err != nil {
		return Info{}, fmt.Errorf("mail cert: TLS probe: %w", err)
	}
	defer func() { _ = conn.Close() }()
	tc, ok := conn.(*tls.Conn)
	if !ok {
		return Info{}, errors.New("mail cert: TLS probe: not a TLS connection")
	}
	peers := tc.ConnectionState().PeerCertificates
	if len(peers) == 0 {
		return Info{}, errors.New("mail cert: TLS probe: no certificate served")
	}
	return describe(peers[0]), nil
}
