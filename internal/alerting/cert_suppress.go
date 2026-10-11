package alerting

import (
	"context"
	"path"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// CertSuppressor returns, for one evaluation, whether a stored certificate's
// hostname must be ignored because a proxy terminates its TLS.
type CertSuppressor func(ctx context.Context) func(domain string) bool

type suppressedCertSource struct {
	inner    CertSource
	suppress CertSuppressor
}

// NewSuppressedCertSource hides stored certificates whose hostname suppress
// reports, so cert expiry and renewal rules never fire on a certificate
// visitors do not receive. A nil suppress returns inner unchanged.
func NewSuppressedCertSource(inner CertSource, suppress CertSuppressor) CertSource {
	if suppress == nil {
		return inner
	}
	return suppressedCertSource{inner: inner, suppress: suppress}
}

func (s suppressedCertSource) ListCertStorageKeys(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	keys, err := s.inner.ListCertStorageKeys(ctx, prefix, recursive)
	if err != nil {
		return nil, err
	}
	skip := s.suppress(ctx)
	out := keys[:0:0]
	for _, k := range keys {
		if path.Ext(k) == ".crt" && skip(storageKeyDomain(k)) {
			continue
		}
		out = append(out, k)
	}
	return out, nil
}

func (s suppressedCertSource) GetCertStorageValue(ctx context.Context, key string) (*store.CertStorageValue, error) {
	return s.inner.GetCertStorageValue(ctx, key)
}

type suppressedACMESource struct {
	inner    ACMEFailureSource
	suppress CertSuppressor
}

// NewSuppressedACMESource reports no failure for hostnames suppress names, so
// a stale attempt made before a proxy took over TLS never raises an alert.
func NewSuppressedACMESource(inner ACMEFailureSource, suppress CertSuppressor) ACMEFailureSource {
	if suppress == nil || inner == nil {
		return inner
	}
	return suppressedACMESource{inner: inner, suppress: suppress}
}

func (s suppressedACMESource) ACMEFailing(domain string) (string, bool) {
	if s.suppress(context.Background())(domain) {
		return "", false
	}
	return s.inner.ACMEFailing(domain)
}

// storageKeyDomain is the hostname segment of a certmagic key such as
// certificates/<issuer>/<host>/<host>.crt, with certmagic's wildcard escape undone.
func storageKeyDomain(key string) string {
	host := path.Base(path.Dir(key))
	if rest, ok := strings.CutPrefix(host, "wildcard_"); ok {
		return "*" + rest
	}
	return host
}
