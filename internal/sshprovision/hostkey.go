package sshprovision

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// HostKey is one public host key a server presented, as ssh-keygen -l prints it.
type HostKey struct {
	// Type is the key algorithm, e.g. "ssh-ed25519" or "ssh-rsa".
	Type string
	// Fingerprint is "SHA256:" plus the unpadded base64 SHA-256 of the key.
	Fingerprint string
}

// ErrHostKeyRequired means a Provision call carried neither a pinned
// fingerprint nor an explicit opt out of host key verification.
var ErrHostKeyRequired = errors.New("sshprovision: a host key fingerprint is required, run the preflight and confirm the server's fingerprint")

// HostKeyMismatchError reports that no key the server presented matches the pinned fingerprint.
type HostKeyMismatchError struct {
	Expected string
	Observed []HostKey
}

func (e *HostKeyMismatchError) Error() string {
	parts := make([]string, 0, len(e.Observed))
	for _, k := range e.Observed {
		parts = append(parts, k.Type+" "+k.Fingerprint)
	}
	observed := strings.Join(parts, ", ")
	if observed == "" {
		observed = "no host key"
	}
	return fmt.Sprintf("sshprovision: host key verification failed: expected %s, server presented %s", e.Expected, observed)
}

var fingerprintRe = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`)

// NormalizeFingerprint trims fp, adds a missing "SHA256:" prefix, and
// reports whether the result is a well-formed SHA256 fingerprint.
func NormalizeFingerprint(fp string) (string, bool) {
	fp = strings.TrimRight(strings.TrimSpace(fp), "=")
	if !strings.HasPrefix(fp, "SHA256:") {
		fp = "SHA256:" + fp
	}
	return fp, fingerprintRe.MatchString(fp)
}

// scanFamilies offers one key family per connection, since a server shows
// exactly one host key per handshake and the operator may pin any of them.
var scanFamilies = [][]string{
	{ssh.KeyAlgoED25519},
	{ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521},
	{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA},
}

func algorithmsForKeyType(keyType string) []string {
	for _, fam := range scanFamilies {
		for _, a := range fam {
			if a == keyType {
				return fam
			}
		}
	}
	return []string{keyType}
}

// errKeyRecorded aborts the handshake right after key exchange, before
// the client ever reaches user authentication.
var errKeyRecorded = errors.New("sshprovision: host key recorded")

// ScanHostKeys connects to host:port and returns every host key the server
// presents, like ssh-keyscan. It never authenticates: each handshake is
// aborted inside the host key callback, so no username or credential is sent.
func (p *Provisioner) ScanHostKeys(ctx context.Context, host string, port int) ([]HostKey, error) {
	addr := Credentials{Host: host, Port: port}.addr()
	var keys []HostKey
	var lastErr error
	seen := map[string]bool{}
	for _, fam := range scanFamilies {
		k, err := p.scanOne(ctx, addr, fam)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if !seen[k.Fingerprint] {
			seen[k.Fingerprint] = true
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		if lastErr == nil {
			lastErr = errors.New("no host key offered")
		}
		return nil, fmt.Errorf("sshprovision: read host key from %s: %w", addr, lastErr)
	}
	return keys, nil
}

func (p *Provisioner) scanOne(ctx context.Context, addr string, algos []string) (HostKey, error) {
	timeout := p.dialTimeout()
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return HostKey{}, fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	var got *HostKey
	config := &ssh.ClientConfig{
		HostKeyAlgorithms: algos,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			got = &HostKey{Type: key.Type(), Fingerprint: ssh.FingerprintSHA256(key)}
			return errKeyRecorded
		},
		Timeout: timeout,
	}
	_, _, _, err = ssh.NewClientConn(conn, addr, config)
	if got != nil {
		return *got, nil
	}
	if err == nil {
		return HostKey{}, errors.New("handshake completed without a host key check")
	}
	return HostKey{}, fmt.Errorf("handshake: %w", err)
}

// PinnedHostKeyCallback accepts only a host key whose SHA256 fingerprint is fp.
func PinnedHostKeyCallback(fp string) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		got := ssh.FingerprintSHA256(key)
		if got != fp {
			return &HostKeyMismatchError{Expected: fp, Observed: []HostKey{{Type: key.Type(), Fingerprint: got}}}
		}
		return nil
	}
}

// hostKeyConfig resolves the host key policy for an authenticated dial. With
// a pinned fingerprint it first scans (credential free) to learn which key
// type carries it, then restricts the real handshake to that type.
func (p *Provisioner) hostKeyConfig(ctx context.Context, creds Credentials) (ssh.HostKeyCallback, []string, error) {
	if creds.InsecureSkipHostKeyCheck {
		return ssh.InsecureIgnoreHostKey(), nil, nil //nolint:gosec // explicit operator opt out, surfaced as a warning in the API response and audit log
	}
	if creds.HostKeyFingerprint == "" {
		return nil, nil, ErrHostKeyRequired
	}
	fp, ok := NormalizeFingerprint(creds.HostKeyFingerprint)
	if !ok {
		return nil, nil, fmt.Errorf("sshprovision: malformed host key fingerprint %q, want SHA256:... as ssh-keygen -l prints it", creds.HostKeyFingerprint)
	}
	keys, err := p.ScanHostKeys(ctx, creds.Host, creds.Port)
	if err != nil {
		return nil, nil, err
	}
	for _, k := range keys {
		if k.Fingerprint == fp {
			return PinnedHostKeyCallback(fp), algorithmsForKeyType(k.Type), nil
		}
	}
	return nil, nil, &HostKeyMismatchError{Expected: fp, Observed: keys}
}
