package alerting

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// certStorageCertsPrefix is the top-level storage-key prefix every
// certmagic-managed certificate lives under; see internal/api's own
// (former) copy of this constant for the certmagic.KeyBuilder shape.
const certStorageCertsPrefix = "certificates"

// DefaultCertExpiryWarningWindow is how far ahead of a certificate's
// NotAfter ListCertificates starts reporting "expiring_soon" instead of
// "healthy" when no override is configured. Also GET
// /api/v1/certificates' own default (api.Router.certExpiryWarningWindow),
// so the dashboard's TLS card and a kind=cert_expiry rule always agree
// on when a certificate first becomes a concern.
const DefaultCertExpiryWarningWindow = 14 * 24 * time.Hour

// DefaultCertRenewalStalledThreshold is how long a certificate can sit
// in "expiring_soon" or "expired" with an unchanged NotAfter before
// EvaluateCertExpiry treats it as a stronger "renewal appears stalled"
// signal rather than a plain expiry warning. Caddy's own ACME renewal
// starts at roughly a third of a certificate's remaining lifetime
// (~30 days out for a 90-day Let's Encrypt cert), well before the
// 14-day warning window even opens, so a real renewal attempt should
// already be long done by the time a certificate enters that window;
// several hours of the window elapsing with zero movement on NotAfter
// is a genuine anomaly, not just evaluation-tick jitter.
const DefaultCertRenewalStalledThreshold = 6 * time.Hour

// CertSource is the narrow storage surface a cert-expiry evaluation
// needs: the same two-method shape internal/api.CertStore already
// defines. Duplicated here rather than imported because internal/api
// imports internal/alerting (for AlertRules et al.), so the reverse
// import would cycle; *store.DB satisfies both interfaces structurally.
type CertSource interface {
	ListCertStorageKeys(ctx context.Context, prefix string, recursive bool) ([]string, error)
	GetCertStorageValue(ctx context.Context, key string) (*store.CertStorageValue, error)
}

// CertInfo is one stored certificate's expiry-relevant fields, the
// single computation both GET /api/v1/certificates (internal/api) and a
// kind=cert_expiry alert rule read via ListCertificates, so the two can
// never silently disagree about a certificate's status.
type CertInfo struct {
	Domain    string
	SANs      []string
	Issuer    string
	NotBefore time.Time
	NotAfter  time.Time
	// Status is "healthy", "expiring_soon", or "expired"; see
	// CertExpiryStatus.
	Status string
}

// CertExpiryStatus buckets notAfter, relative to now, into the three
// states a certificate can be in: "expired" once notAfter has passed or
// is passing this instant, "expiring_soon" within warningWindow of it,
// "healthy" otherwise. A notAfter exactly warningWindow away is still
// "healthy": the window is when to start warning, not an inclusive
// boundary.
func CertExpiryStatus(notAfter, now time.Time, warningWindow time.Duration) string {
	switch {
	case !notAfter.After(now):
		return "expired"
	case notAfter.Before(now.Add(warningWindow)):
		return "expiring_soon"
	default:
		return "healthy"
	}
}

// ListCertificates returns every certificate currently in source,
// parsed from its own stored PEM bytes, sorted by domain. A control
// plane that has never issued a certificate returns an empty slice, not
// an error. A single malformed or since-deleted entry is skipped and
// logged, never fails the whole list, the same "one broken resource
// must not block the rest" shape used throughout this codebase.
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
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Domain < out[j].Domain })
	return out, nil
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

// certDomain picks a stored certificate's primary subject: the first SAN
// if it has any, else its CommonName, else the domain segment of its own
// storage key, so a pathological certificate with neither still gets a
// usable label instead of an empty string.
func certDomain(cert *x509.Certificate, key string) string {
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}
	return path.Base(path.Dir(key))
}

// CertExpiryObservationStore is the narrow store surface
// EvaluateCertExpiry needs to remember what it saw last tick, per
// domain. *DB satisfies this structurally.
type CertExpiryObservationStore interface {
	UpsertCertExpiryObservation(ctx context.Context, o CertExpiryObservation) error
	ListCertExpiryObservations(ctx context.Context, ruleID string) ([]CertExpiryObservation, error)
}

// EvaluateCertExpiry runs one KindCertExpiry rule against every
// certificate currently in certs and returns its updated evaluation
// state plus, when firing, one human-readable notice line per
// non-healthy certificate for Engine to attach to the outgoing Event.
//
// The rule fires (satisfied, in advanceState terms) the instant any
// certificate is "expiring_soon" or "expired", with no ForDuration
// debounce: unlike a noisy metric sample, expiry status only moves
// monotonically with real time, so there is nothing to debounce.
// LastValue is the number of days until the earliest-expiring
// certificate (negative once expired), across every certificate, not
// just the non-healthy ones, so an operator can see "how close" even
// while healthy.
//
// Per non-healthy domain, snapshots records the NotAfter it had when it
// first entered this expiry episode (EpisodeNotAfter/EpisodeStartedAt).
// If a later tick finds the same domain still non-healthy with an
// unchanged NotAfter for at least stalledThreshold, that domain's notice
// is marked as a renewal that appears stalled rather than a plain
// warning: Caddy's own renewal should have already succeeded well
// before the warning window even opened (DefaultCertRenewalStalledThreshold's
// own doc comment), so a stuck NotAfter this deep into the window is a
// stronger, more actionable signal than "getting close to expiry."
func EvaluateCertExpiry(ctx context.Context, certs CertSource, snapshots CertExpiryObservationStore, r Rule, warningWindow, stalledThreshold time.Duration, now time.Time, logger *slog.Logger) (Rule, []string, error) {
	if warningWindow <= 0 {
		warningWindow = DefaultCertExpiryWarningWindow
	}
	if stalledThreshold <= 0 {
		stalledThreshold = DefaultCertRenewalStalledThreshold
	}

	infos, err := ListCertificates(ctx, certs, warningWindow, now, logger)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: %w", r.ID, err)
	}

	next := r
	next.LastEvaluatedAt = &now

	if len(infos) == 0 {
		next.PendingSince = nil
		next.Firing = false
		next.FiringSince = nil
		return next, nil, nil
	}

	prev, err := snapshots.ListCertExpiryObservations(ctx, r.ID)
	if err != nil {
		return r, nil, fmt.Errorf("alerting: evaluate rule %q: load cert expiry observations: %w", r.ID, err)
	}
	prevByDomain := make(map[string]CertExpiryObservation, len(prev))
	for _, o := range prev {
		prevByDomain[o.Domain] = o
	}

	var (
		notices      []string
		anyUnhealthy bool
		minDays      float64
		haveMinDays  bool
	)

	for _, info := range infos {
		days := info.NotAfter.Sub(now).Hours() / 24
		if !haveMinDays || days < minDays {
			minDays, haveMinDays = days, true
		}

		if info.Status == "healthy" {
			// Clear any stale episode: the next time this domain goes
			// unhealthy, it starts a fresh one rather than reusing an
			// old, unrelated expiry's episode start.
			obs := CertExpiryObservation{
				RuleID: r.ID, Domain: info.Domain, Status: info.Status, NotAfter: info.NotAfter,
				EpisodeNotAfter: info.NotAfter, EpisodeStartedAt: now, ObservedAt: now,
			}
			if err := snapshots.UpsertCertExpiryObservation(ctx, obs); err != nil {
				return r, nil, fmt.Errorf("alerting: evaluate rule %q: save cert expiry observation for %q: %w", r.ID, info.Domain, err)
			}
			continue
		}

		anyUnhealthy = true
		episodeNotAfter, episodeStartedAt := info.NotAfter, now
		stalled := false
		if p, ok := prevByDomain[info.Domain]; ok &&
			(p.Status == "expiring_soon" || p.Status == "expired") &&
			p.EpisodeNotAfter.Equal(info.NotAfter) {
			episodeNotAfter, episodeStartedAt = p.EpisodeNotAfter, p.EpisodeStartedAt
			stalled = now.Sub(episodeStartedAt) >= stalledThreshold
		}

		obs := CertExpiryObservation{
			RuleID: r.ID, Domain: info.Domain, Status: info.Status, NotAfter: info.NotAfter,
			EpisodeNotAfter: episodeNotAfter, EpisodeStartedAt: episodeStartedAt, ObservedAt: now,
		}
		if err := snapshots.UpsertCertExpiryObservation(ctx, obs); err != nil {
			return r, nil, fmt.Errorf("alerting: evaluate rule %q: save cert expiry observation for %q: %w", r.ID, info.Domain, err)
		}

		notices = append(notices, certExpiryNotice(info, stalled))
	}

	next.LastValue = &minDays

	return advanceState(next, r, anyUnhealthy, 0, now), notices, nil
}

func certExpiryNotice(info CertInfo, stalled bool) string {
	base := fmt.Sprintf("%s: %s, expires %s", info.Domain, info.Status, info.NotAfter.UTC().Format(time.RFC3339))
	if !stalled {
		return base
	}
	return base + " (renewal appears stalled: NotAfter hasn't moved since it first entered this state, Caddy may have failed to renew)"
}
