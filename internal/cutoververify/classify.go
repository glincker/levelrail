package cutoververify

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"unicode"
)

// Error classes shown in reports instead of raw Go error text, which can carry
// resolved addresses and internal detail.
const (
	ClassDNSFailed      = "dns_failed"
	ClassConnectRefused = "connect_refused"
	ClassTimeout        = "timeout"
	ClassTLSFailed      = "tls_failed"
	ClassBlocked        = "blocked_address"
	ClassConnectFailed  = "connect_failed"
	ClassRequestFailed  = "request_failed"
)

const maxShown = 200

// Classify maps err to one stable class name.
func Classify(err error) string {
	var (
		dnsErr  *net.DNSError
		opErr   *net.OpError
		certErr *tls.CertificateVerificationError
		unknown x509.UnknownAuthorityError
		host    x509.HostnameError
		inval   x509.CertificateInvalidError
		record  tls.RecordHeaderError
		alert   tls.AlertError
	)
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrNonPublicAddress):
		return ClassBlocked
	case errors.Is(err, context.DeadlineExceeded) || isTimeout(err):
		return ClassTimeout
	case errors.As(err, &dnsErr):
		return ClassDNSFailed
	case errors.Is(err, syscall.ECONNREFUSED):
		return ClassConnectRefused
	case errors.As(err, &certErr), errors.As(err, &unknown), errors.As(err, &host), errors.As(err, &inval),
		errors.As(err, &record), errors.As(err, &alert), strings.Contains(err.Error(), "tls:"):
		return ClassTLSFailed
	case errors.As(err, &opErr) && opErr.Op == "dial":
		return ClassConnectFailed
	}
	return ClassRequestFailed
}

// classifyLookup is Classify for a name lookup, where anything but a timeout is a DNS failure.
func classifyLookup(err error) string {
	if c := Classify(err); c == ClassTimeout || c == ClassBlocked {
		return c
	}
	return ClassDNSFailed
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// clean drops control characters from text a remote server sent and shortens it.
func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	if r := []rune(s); len(r) > maxShown {
		return string(r[:maxShown]) + "..."
	}
	return s
}

// safeLocation returns a redirect target that is fit to show. It reports false
// when the target points at a host that is not public, so a response cannot
// make the report echo internal names.
func safeLocation(loc string) (string, bool) {
	u, err := url.Parse(loc)
	if err != nil {
		return "", false
	}
	if u.Host == "" {
		return clean(loc), true
	}
	h := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if ip, perr := netip.ParseAddr(h); perr == nil {
		return clean(loc), IsPublicAddr(ip)
	}
	last := h[strings.LastIndex(h, ".")+1:]
	switch {
	case h == "" || !strings.Contains(h, "."), last == "localhost", last == "local", last == "internal", last == "localdomain", last == "lan", last == "home":
		return "", false
	case strings.HasPrefix(last, "0x") || strings.Trim(last, "0123456789") == "":
		// Numeric shorthand such as 127.1 or 2130706433 is an address in disguise.
		return "", false
	}
	return clean(loc), true
}
