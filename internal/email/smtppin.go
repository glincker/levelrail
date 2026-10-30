package email

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// ErrPinMismatch is a certificate that does not match the stored pin.
var ErrPinMismatch = errors.New("email: server certificate does not match the pinned fingerprint")

// CertSHA256 is the pin form of a certificate: hex SHA-256 of its DER bytes.
func CertSHA256(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// pinnedTLSConfig skips chain and name checks only to replace them with an exact leaf match.
func pinnedTLSConfig(host, pin string) *tls.Config {
	want := strings.ToLower(strings.TrimSpace(pin))
	return &tls.Config{
		ServerName:         host,
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, //nolint:gosec // replaced by the exact leaf pin in VerifyConnection
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return ErrPinMismatch
			}
			got := CertSHA256(cs.PeerCertificates[0].Raw)
			if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
				return ErrPinMismatch
			}
			return nil
		},
	}
}

// FormatPin binds a pin to one address so it never applies to another server.
func FormatPin(addr, sha string) string { return addr + " " + sha }

// PinFor returns the pin stored for addr, or "" when the stored pin names another address.
func PinFor(stored, addr string) string {
	a, sha, ok := strings.Cut(strings.TrimSpace(stored), " ")
	if !ok || a != addr || len(sha) != sha256.Size*2 {
		return ""
	}
	return sha
}

// FetchSMTPCertSHA256 reads the leaf certificate a submission server presents (implicit TLS on 465,
// STARTTLS otherwise). It authenticates nothing; callers use it only on a private network they trust.
func FetchSMTPCertSHA256(ctx context.Context, addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", err
	}
	var leaf []byte
	cfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: true, //nolint:gosec // trust on first use; the result is a pin, not a trust decision
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) > 0 {
				leaf = cs.PeerCertificates[0].Raw
			}
			return nil
		}}
	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	if port == "465" {
		conn, err := (&tls.Dialer{NetDialer: dialer, Config: cfg}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return "", err
		}
		_ = conn.Close()
	} else {
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return "", err
		}
		if d, ok := ctx.Deadline(); ok {
			_ = conn.SetDeadline(d)
		}
		c, err := smtp.NewClient(conn, host)
		if err != nil {
			_ = conn.Close()
			return "", err
		}
		defer func() { _ = c.Close() }()
		if err := c.StartTLS(cfg); err != nil {
			return "", fmt.Errorf("starttls: %w", err)
		}
		_ = c.Quit()
	}
	if len(leaf) == 0 {
		return "", errors.New("email: server presented no certificate")
	}
	return CertSHA256(leaf), nil
}
