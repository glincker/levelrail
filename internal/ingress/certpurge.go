package ingress

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/caddyserver/certmagic"

	"github.com/GLINCKER/levelrail/internal/store"
)

const certPurgeMarkerKey = "levelrail/cert-purge-issuer"

// CertPurgeMarkerStore adds the marker read and write PurgeCertsFromOtherIssuersOnce needs.
type CertPurgeMarkerStore interface {
	CertPurgeStore
	GetCertStorageValue(ctx context.Context, key string) (*store.CertStorageValue, error)
	SaveCertStorageValue(ctx context.Context, key string, value []byte) error
}

// PurgeCertsFromOtherIssuersOnce runs the purge only when the configured
// issuer differs from the one last purged for, so a restart never deletes
// valid certificates again and burns CA rate limits.
func PurgeCertsFromOtherIssuersOnce(ctx context.Context, s CertPurgeMarkerStore, directoryURL string) (int, error) {
	keep := issuerKeyFor(directoryURL)
	if v, err := s.GetCertStorageValue(ctx, certPurgeMarkerKey); err == nil && string(v.Value) == keep {
		return 0, nil
	}
	n, err := PurgeCertsFromOtherIssuers(ctx, s, directoryURL)
	if err != nil {
		return n, err
	}
	if err := s.SaveCertStorageValue(ctx, certPurgeMarkerKey, []byte(keep)); err != nil {
		return n, fmt.Errorf("ingress: record purged issuer: %w", err)
	}
	return n, nil
}

func issuerKeyFor(directoryURL string) string {
	if strings.TrimSpace(directoryURL) == "" {
		directoryURL = certmagic.LetsEncryptProductionCA
	}
	return certmagic.StorageKeys.Safe((&certmagic.ACMEIssuer{CA: directoryURL}).IssuerKey())
}

// CertPurgeStore is the storage surface PurgeCertsFromOtherIssuers needs.
type CertPurgeStore interface {
	ListCertStorageKeys(ctx context.Context, prefix string, recursive bool) ([]string, error)
	DeleteCertStorageValue(ctx context.Context, key string) error
}

// PurgeCertsFromOtherIssuers deletes every stored certificate not issued
// through directoryURL's CA ("" means Let's Encrypt production). certmagic
// reuses a still-valid certificate from any issuer, so without this a switch
// from staging or the internal CA to the real CA would never re-issue until
// the old certificate expired. Returns the number of keys removed.
func PurgeCertsFromOtherIssuers(ctx context.Context, s CertPurgeStore, directoryURL string) (int, error) {
	keep := issuerKeyFor(directoryURL)
	keys, err := s.ListCertStorageKeys(ctx, "certificates", true)
	if err != nil {
		return 0, fmt.Errorf("ingress: list certificates for purge: %w", err)
	}
	removed := 0
	for _, key := range keys {
		parts := strings.Split(path.Clean(key), "/")
		if len(parts) < 4 || parts[0] != "certificates" || parts[1] == keep {
			continue
		}
		if err := s.DeleteCertStorageValue(ctx, key); err != nil {
			return removed, fmt.Errorf("ingress: purge certificate key %q: %w", key, err)
		}
		removed++
	}
	return removed, nil
}
