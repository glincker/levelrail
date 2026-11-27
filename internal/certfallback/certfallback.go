// Package certfallback finds and replaces the untrusted local-issuer certificate
// Caddy stores for a domain when a public CA could not issue one.
package certfallback

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	certsPrefix = "certificates"
	localIssuer = "local"
)

// Store is the cert storage surface; *store.DB satisfies it.
type Store interface {
	ListCertStorageKeys(ctx context.Context, prefix string, recursive bool) ([]string, error)
	GetCertStorageValue(ctx context.Context, key string) (*store.CertStorageValue, error)
	DeleteCertStorageValue(ctx context.Context, key string) error
}

// IsLocalIssuerCert reports whether a stored certificate came from Caddy's
// internal CA, by its storage issuer segment or its issuer name.
func IsLocalIssuerCert(key string, cert *x509.Certificate) bool {
	parts := strings.Split(path.Clean(key), "/")
	if len(parts) > 2 && parts[0] == certsPrefix && parts[1] == localIssuer {
		return true
	}
	return strings.Contains(cert.Issuer.CommonName, "Local Authority")
}

// Held maps each lowercase domain to the storage directories holding a
// local-issuer certificate for it. Public-CA certificates are never listed.
func Held(ctx context.Context, s Store) (map[string][]string, error) {
	keys, err := s.ListCertStorageKeys(ctx, certsPrefix, true)
	if err != nil {
		return nil, fmt.Errorf("certfallback: list cert storage: %w", err)
	}
	out := map[string][]string{}
	for _, key := range keys {
		if path.Ext(key) != ".crt" {
			continue
		}
		v, err := s.GetCertStorageValue(ctx, key)
		if err != nil {
			if errors.Is(err, store.ErrCertStorageKeyNotFound) {
				continue
			}
			return nil, fmt.Errorf("certfallback: read %q: %w", key, err)
		}
		block, _ := pem.Decode(v.Value)
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !IsLocalIssuerCert(key, cert) {
			continue
		}
		dir := path.Dir(key)
		names := append([]string{cert.Subject.CommonName}, cert.DNSNames...)
		for _, n := range names {
			n = strings.ToLower(strings.TrimSpace(n))
			if n != "" && !contains(out[n], dir) {
				out[n] = append(out[n], dir)
			}
		}
	}
	return out, nil
}

// Drop deletes only the local-issuer storage directories held for domain and
// returns how many it removed; a public-CA certificate is never touched.
func Drop(ctx context.Context, s Store, domain string) (int, error) {
	held, err := Held(ctx, s)
	if err != nil {
		return 0, err
	}
	dirs := held[strings.ToLower(strings.TrimSpace(domain))]
	for i, dir := range dirs {
		if err := s.DeleteCertStorageValue(ctx, dir); err != nil {
			return i, fmt.Errorf("certfallback: drop %q: %w", dir, err)
		}
	}
	return len(dirs), nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
