package alerting

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// certStorageCertsPrefix is the top-level storage-key prefix every
// certmagic-managed certificate lives under; see internal/api's own
// (former) copy of this constant for the certmagic.KeyBuilder shape.
const certStorageCertsPrefix = "certificates"

// DefaultCertExpiryWarningWindow is the default lead time before NotAfter for a certificate to report expiring_soon.
const DefaultCertExpiryWarningWindow = 14 * 24 * time.Hour

// DefaultCertRenewalStalledThreshold is how long NotAfter may stay unchanged in the warning window before renewal is reported stalled.
const DefaultCertRenewalStalledThreshold = 6 * time.Hour

// CertSource is redeclared here to avoid an import cycle, since internal/api imports this package.
type CertSource interface {
	ListCertStorageKeys(ctx context.Context, prefix string, recursive bool) ([]string, error)
	GetCertStorageValue(ctx context.Context, key string) (*store.CertStorageValue, error)
}

// CertInfo is one certificate's expiry fields, shared by the API and cert_expiry rules so they never disagree.
type CertInfo struct {
	Domain    string
	SANs      []string
	Issuer    string
	NotBefore time.Time
	NotAfter  time.Time
	// Status is "healthy", "expiring_soon", or "expired"; see
	// CertExpiryStatus.
	Status string
	// Internal marks a self-signed or local-CA certificate the platform issues by design.
	Internal bool
}

// Certificate expiry buckets reported by CertExpiryStatus.
const (
	certStatusHealthy      = "healthy"
	certStatusExpiringSoon = "expiring_soon"
	certStatusExpired      = "expired"
)

// CertExpiryStatus buckets notAfter into expired, expiring_soon or healthy.
// A notAfter exactly warningWindow away is still healthy: the window is when warnings start.
// boundary.
func CertExpiryStatus(notAfter, now time.Time, warningWindow time.Duration) string {
	switch {
	case !notAfter.After(now):
		return certStatusExpired
	case notAfter.Before(now.Add(warningWindow)):
		return certStatusExpiringSoon
	default:
		return certStatusHealthy
	}
}

// ListCertificates skips and logs a malformed entry, so one bad certificate does not fail the list.
func ListCertificates(ctx context.Context, source CertSource, warningWindow time.Duration, now time.Time, logger *slog.Logger) ([]CertInfo, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if warningWindow <= 0 {
		warningWindow = DefaultCertExpiryWarningWindow
	}

	keys, err := source.ListCertStorageKeys(ctx, certStorageCertsPrefix, true)
	if err != nil {
		return nil, fmt.Errorf("alerting: list cert storage keys: %w", err)
	}

	out := make([]CertInfo, 0, len(keys))
	for _, key := range keys {
		// Every certmagic site prefix also holds a ".key" and a ".json"
		// entry alongside the ".crt" this loop wants.
		if path.Ext(key) != ".crt" {
			continue
		}

		v, err := source.GetCertStorageValue(ctx, key)
		if err != nil {
			if !errors.Is(err, store.ErrCertStorageKeyNotFound) {
				logger.Warn("alerting: list certificates: load stored certificate failed, skipping",
					slog.String("key", key), slog.String("error", err.Error()))
			}
			continue
		}
		cert, err := parseLeafCertificate(v.Value)
		if err != nil {
			logger.Warn("alerting: list certificates: parse stored certificate failed, skipping",
				slog.String("key", key), slog.String("error", err.Error()))
			continue
		}
		out = append(out, CertInfo{
			Domain:    certDomain(cert, key),
			SANs:      cert.DNSNames,
			Issuer:    cert.Issuer.CommonName,
			NotBefore: cert.NotBefore,
			NotAfter:  cert.NotAfter,
			Status:    CertExpiryStatus(cert.NotAfter, now, warningWindow),
			Internal:  isInternalCert(cert, key),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out, nil
}

func isInternalCert(cert *x509.Certificate, key string) bool {
	if bytes.Equal(cert.RawIssuer, cert.RawSubject) || strings.Contains(cert.Issuer.CommonName, "Local Authority") {
		return true
	}
	parts := strings.Split(key, "/")
	return len(parts) > 2 && parts[1] == "local"
}

// parseLeafCertificate parses the first certificate in a PEM-encoded
// chain, the leaf: a stored ".crt" value is the full chain, leaf first.
func parseLeafCertificate(pemBytes []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("alerting: no PEM block found in stored certificate")
	}
	return x509.ParseCertificate(block.Bytes)
}

// Falls back to the storage key's domain segment so a certificate with no names still gets a label.
func certDomain(cert *x509.Certificate, key string) string {
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}
	return path.Base(path.Dir(key))
}

// certMatchesDomain reports whether domain is cert's CommonName or one of
// its SANs, case-insensitively.
func certMatchesDomain(cert *x509.Certificate, domain string) bool {
	if strings.EqualFold(cert.Subject.CommonName, domain) {
		return true
	}
	for _, san := range cert.DNSNames {
		if strings.EqualFold(san, domain) {
			return true
		}
	}
	return false
}

// CertDeleter is the narrow storage surface DeleteCertificateForDomain
// needs beyond CertSource's two read methods: *store.DB satisfies this
// structurally, same as CertSource.
type CertDeleter interface {
	DeleteCertStorageValue(ctx context.Context, key string) error
}

// DeleteCertificateForDomain removes matching entries; no match is a no-op since reconcile re-issues anyway.
func DeleteCertificateForDomain(ctx context.Context, source CertSource, deleter CertDeleter, domain string) (found bool, err error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return false, fmt.Errorf("alerting: delete certificate for domain: domain is required")
	}

	keys, err := source.ListCertStorageKeys(ctx, certStorageCertsPrefix, true)
	if err != nil {
		return false, fmt.Errorf("alerting: delete certificate for domain %q: list cert storage keys: %w", domain, err)
	}

	dirs := map[string]bool{}
	for _, key := range keys {
		if path.Ext(key) != ".crt" {
			continue
		}
		v, err := source.GetCertStorageValue(ctx, key)
		if err != nil {
			continue
		}
		cert, err := parseLeafCertificate(v.Value)
		if err != nil {
			continue
		}
		if certMatchesDomain(cert, domain) {
			dirs[path.Dir(key)] = true
		}
	}

	for dir := range dirs {
		if err := deleter.DeleteCertStorageValue(ctx, dir); err != nil {
			return false, fmt.Errorf("alerting: delete certificate for domain %q: delete storage dir %q: %w", domain, dir, err)
		}
	}
	return len(dirs) > 0, nil
}

// CertExpiryObservationStore is the narrow store surface
// EvaluateCertExpiry needs to remember what it saw last tick, per
// domain. *DB satisfies this structurally.
type CertExpiryObservationStore interface {
	UpsertCertExpiryObservation(ctx context.Context, o CertExpiryObservation) error
	ListCertExpiryObservations(ctx context.Context, ruleID string) ([]CertExpiryObservation, error)
}

// CertEvalResult is what one cert_expiry evaluation reports besides the rule state.
type CertEvalResult struct {
	Notices []string
	Facts   []CertFact
	// Superseded is true when this rule saw unhealthy certificates but a tighter rule owns them all.
	Superseded bool
}

// EvaluateCertExpiry returns one notice per non-healthy certificate, with no debounce since expiry moves only with real time.
// A renewal stuck on the same NotAfter past stalledThreshold is reported as stalled, not a plain warning.
func EvaluateCertExpiry(ctx context.Context, certs CertSource, snapshots CertExpiryObservationStore, r Rule, warningWindow, stalledThreshold time.Duration, now time.Time, logger *slog.Logger) (Rule, []string, error) {
	next, res, err := EvaluateCertExpiryPolicy(ctx, certs, snapshots, r, warningWindow, stalledThreshold, now, logger, CertExpiryPolicy{})
	return next, res.Notices, err
}

// EvaluateCertExpiryPolicy is EvaluateCertExpiry with the quieting policy applied: one window per certificate, no alert for certificates the platform renews itself.
func EvaluateCertExpiryPolicy(ctx context.Context, certs CertSource, snapshots CertExpiryObservationStore, r Rule, warningWindow, stalledThreshold time.Duration, now time.Time, logger *slog.Logger, policy CertExpiryPolicy) (Rule, CertEvalResult, error) {
	var res CertEvalResult
	if warningWindow <= 0 {
		warningWindow = DefaultCertExpiryWarningWindow
	}
	if stalledThreshold <= 0 {
		stalledThreshold = DefaultCertRenewalStalledThreshold
	}

	infos, err := ListCertificates(ctx, certs, warningWindow, now, logger)
	if err != nil {
		return r, res, fmt.Errorf("alerting: evaluate rule %q: %w", r.ID, err)
	}

	next := r
	next.LastEvaluatedAt = &now

	if len(infos) == 0 {
		next.PendingSince = nil
		next.Firing = false
		next.FiringSince = nil
		return next, res, nil
	}

	prev, err := snapshots.ListCertExpiryObservations(ctx, r.ID)
	if err != nil {
		return r, res, fmt.Errorf("alerting: evaluate rule %q: load cert expiry observations: %w", r.ID, err)
	}
	prevByDomain := make(map[string]CertExpiryObservation, len(prev))
	for _, o := range prev {
		prevByDomain[o.Domain] = o
	}

	var (
		anyUnhealthy bool
		minDays      float64
		haveMinDays  bool
	)

	for _, info := range infos {
		days := info.NotAfter.Sub(now).Hours() / 24
		if !haveMinDays || days < minDays {
			minDays, haveMinDays = days, true
		}

		if info.Status == certStatusHealthy {
			obs := CertExpiryObservation{
				RuleID: r.ID, Domain: info.Domain, Status: info.Status, NotAfter: info.NotAfter,
				EpisodeNotAfter: info.NotAfter, EpisodeStartedAt: now, ObservedAt: now,
			}
			if err := snapshots.UpsertCertExpiryObservation(ctx, obs); err != nil {
				return r, res, fmt.Errorf("alerting: evaluate rule %q: save cert expiry observation for %q: %w", r.ID, info.Domain, err)
			}
			continue
		}

		episodeNotAfter, episodeStartedAt := info.NotAfter, now
		stalled := false
		if p, ok := prevByDomain[info.Domain]; ok &&
			(p.Status == certStatusExpiringSoon || p.Status == certStatusExpired) &&
			p.EpisodeNotAfter.Equal(info.NotAfter) {
			episodeNotAfter, episodeStartedAt = p.EpisodeNotAfter, p.EpisodeStartedAt
			stalled = now.Sub(episodeStartedAt) >= stalledThreshold
		}

		obs := CertExpiryObservation{
			RuleID: r.ID, Domain: info.Domain, Status: info.Status, NotAfter: info.NotAfter,
			EpisodeNotAfter: episodeNotAfter, EpisodeStartedAt: episodeStartedAt, ObservedAt: now,
		}
		if err := snapshots.UpsertCertExpiryObservation(ctx, obs); err != nil {
			return r, res, fmt.Errorf("alerting: evaluate rule %q: save cert expiry observation for %q: %w", r.ID, info.Domain, err)
		}

		if !policy.Enabled {
			anyUnhealthy = true
			res.Notices = append(res.Notices, certExpiryNotice(info, stalled))
			continue
		}
		if t := policy.tightest(info.NotAfter.Sub(now)); t != 0 && t != warningWindow {
			res.Superseded = true
			continue
		}
		fact, alert := policy.assess(info, now, stalledThreshold, stalled)
		if !alert {
			continue
		}
		anyUnhealthy = true
		res.Facts = append(res.Facts, fact)
		res.Notices = append(res.Notices, fact.Notice())
	}

	next.LastValue = &minDays
	if anyUnhealthy {
		res.Superseded = false
	}

	return advanceState(next, r, anyUnhealthy, 0, now), res, nil
}

func certExpiryNotice(info CertInfo, stalled bool) string {
	base := fmt.Sprintf("%s: %s, expires %s", info.Domain, info.Status, info.NotAfter.UTC().Format(time.RFC3339))
	if !stalled {
		return base
	}
	return base + " (renewal appears stalled: NotAfter hasn't moved since it first entered this state, Caddy may have failed to renew)"
}
