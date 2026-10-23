package email

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"strings"
	"syscall"
)

// FailureKind is the typed reason a send failed, mapped to a Fix in the UI.
type FailureKind string

// The failure kinds Classify returns.
const (
	FailAuth              FailureKind = "auth_failed"
	FailSenderNotVerified FailureKind = "sender_not_verified"
	FailRegionMismatch    FailureKind = "region_mismatch"
	FailPortBlocked       FailureKind = "port_blocked"
	FailTLS               FailureKind = "tls_error"
	FailRateLimited       FailureKind = "rate_limited"
	FailNotConfigured     FailureKind = "not_configured"
	FailOther             FailureKind = "other"
)

// ErrRateLimited is returned when the configured sends-per-minute or per-day cap is reached.
var ErrRateLimited = errors.New("email: send rate limit reached")

// HTTPError is a non-2xx answer from an HTTP provider API.
type HTTPError struct {
	Provider string
	Status   int
	Body     string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("email: %s returned status %d: %s", e.Provider, e.Status, e.Body)
}

// SendError wraps a final failure with its kind and how many attempts were made.
type SendError struct {
	Kind     FailureKind
	Attempts int
	Err      error
}

func (e *SendError) Error() string { return e.Err.Error() }
func (e *SendError) Unwrap() error { return e.Err }

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// Classify maps a send error to a FailureKind.
func Classify(err error) FailureKind {
	if err == nil {
		return ""
	}
	var se *SendError
	if errors.As(err, &se) && se.Kind != "" {
		return se.Kind
	}
	if errors.Is(err, ErrNotConfigured) {
		return FailNotConfigured
	}
	if errors.Is(err, ErrRateLimited) {
		return FailRateLimited
	}
	var unknownAuthority x509.UnknownAuthorityError
	var badHost x509.HostnameError
	var badCert x509.CertificateInvalidError
	var recordHeader tls.RecordHeaderError
	if errors.As(err, &unknownAuthority) || errors.As(err, &badHost) || errors.As(err, &badCert) || errors.As(err, &recordHeader) {
		return FailTLS
	}
	msg := strings.ToLower(err.Error())
	var he *HTTPError
	if errors.As(err, &he) {
		switch {
		case he.Status == 429:
			return FailRateLimited
		case containsAny(msg, "not verified", "unverified", "sender signature", "domain is not verified", "verified sender"):
			return FailSenderNotVerified
		case he.Status == 401 || he.Status == 403:
			return FailAuth
		}
	}
	var pe *textproto.Error
	if errors.As(err, &pe) {
		switch {
		case pe.Code == 535 || pe.Code == 530 || pe.Code == 534 || pe.Code == 538:
			return FailAuth
		case containsAny(msg, "not verified", "not authorized to send from"):
			return FailSenderNotVerified
		case pe.Code == 454 && strings.Contains(msg, "tls"):
			return FailTLS
		}
	}
	switch {
	case strings.Contains(msg, "in region") && strings.Contains(msg, "not verified"):
		return FailRegionMismatch
	case strings.Contains(msg, "not verified"):
		return FailSenderNotVerified
	case containsAny(msg, "invalidclienttokenid", "signaturedoesnotmatch", "unrecognizedclient", "security token", "accessdenied", "authentication failed", "invalid api key", "unauthorized"):
		return FailAuth
	case containsAny(msg, "throttl", "too many requests", "rate limit", "sending quota"):
		return FailRateLimited
	case containsAny(msg, "starttls", "tls:", "x509", "certificate"):
		return FailTLS
	case strings.Contains(msg, "no such host") && strings.Contains(msg, "amazonaws"):
		return FailRegionMismatch
	}
	var ne net.Error
	var op *net.OpError
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) ||
		(errors.As(err, &op) && op.Op == "dial") || (errors.As(err, &ne) && ne.Timeout() && !errors.Is(err, context.Canceled)) {
		return FailPortBlocked
	}
	return FailOther
}

// IsTransient reports whether a retry could plausibly succeed.
func IsTransient(err error) bool {
	if err == nil || errors.Is(err, ErrRateLimited) || errors.Is(err, ErrNotConfigured) || errors.Is(err, context.Canceled) {
		return false
	}
	var he *HTTPError
	if errors.As(err, &he) {
		return he.Status == 429 || he.Status >= 500
	}
	var pe *textproto.Error
	if errors.As(err, &pe) {
		return pe.Code >= 400 && pe.Code < 500
	}
	switch Classify(err) {
	case FailRateLimited:
		return true
	case FailPortBlocked, FailAuth, FailSenderNotVerified, FailRegionMismatch, FailTLS:
		return false
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	return containsAny(strings.ToLower(err.Error()), "connection reset", "unexpected eof", "broken pipe", "serviceunavailable", "internalfailure")
}
