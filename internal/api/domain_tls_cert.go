package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DomainTLSCertStore is the store surface GET/PUT/DELETE
// .../domains/{domain}/tls-cert need: the upload/expiry metadata for a
// domain's BYO certificate, same "core Store interface" shape as
// DomainBasicAuthStore.
type DomainTLSCertStore interface {
	GetDomainTLSCert(ctx context.Context, domain string) (store.DomainTLSCert, bool, error)
	SetDomainTLSCert(ctx context.Context, domain string, uploadedAt, expiresAt time.Time) error
	DeleteDomainTLSCert(ctx context.Context, domain string) error
}

// DomainTLSCertSecrets is the surface these handlers need from
// internal/secrets.Manager for a domain's certificate and private key:
// set both (PUT) and clear both (DELETE). *secrets.Manager satisfies
// this structurally, the same narrow shape DomainBasicAuthSecrets
// already establishes.
type DomainTLSCertSecrets interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	DeleteAll(ctx context.Context, serviceName string) error
}

// domainTLSCertResource is the wire shape for GET/PUT/DELETE
// .../domains/{domain}/tls-cert. Neither the certificate nor the private
// key ever appears in a response: only whether one is set and its
// expiry, mirroring domainBasicAuthResource's has_password convention.
type domainTLSCertResource struct {
	Domain     string `json:"domain"`
	Enabled    bool   `json:"enabled"`
	UploadedAt string `json:"uploaded_at,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

func toDomainTLSCertResource(domain string, cert store.DomainTLSCert, found bool) domainTLSCertResource {
	res := domainTLSCertResource{Domain: domain}
	if !found {
		return res
	}
	res.Enabled = true
	res.UploadedAt = cert.UploadedAt.UTC().Format(time.RFC3339)
	res.ExpiresAt = cert.ExpiresAt.UTC().Format(time.RFC3339)
	return res
}

// setDomainTLSCertRequest is PUT .../tls-cert's request body: the
// certificate and private key as PEM text. Both are required every
// call, unlike setDomainBasicAuthRequest's optional password: there is
// no "leave the current certificate unchanged" case here, a cert/key
// pair is only ever replaced as a matched whole.
type setDomainTLSCertRequest struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

// validateCertKeyPair parses certPEM/keyPEM as a matched pair using
// crypto/tls.X509KeyPair, the same validation Caddy's own load_pem
// certificate loader performs (modules/caddytls/pemloader.go's
// PEMLoader.LoadCertificates): any pair X509KeyPair rejects would also
// fail at Caddy's own load step, so this handler fails fast with a
// clearer error instead of only discovering the problem on the next
// ingress reconcile. An already-expired certificate is rejected outright
// rather than accepted with a warning: an operator uploading dead cert
// material is almost certainly a mistake, and this codebase already
// treats a TLS/cert problem as worth failing loudly on (see
// internal/reconcile/ingress's own "fail closed on a security control"
// precedent for basic auth).
func validateCertKeyPair(certPEM, keyPEM string) (*x509.Certificate, error) {
	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("certificate and key do not form a valid pair: %w", err)
	}
	leaf := pair.Leaf
	if leaf == nil {
		leaf, err = x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}
	}
	if time.Now().After(leaf.NotAfter) {
		return nil, fmt.Errorf("certificate is already expired (expired at %s)", leaf.NotAfter.UTC().Format(time.RFC3339))
	}
	return leaf, nil
}

// handleGetDomainTLSCert handles GET
// /api/v1/apps/{name}/domains/{domain}/tls-cert: current BYO certificate
// state for domain. AbilityRead, the same passive-visibility tier GET
// .../domains/{domain}/auth already uses.
func (rt *Router) handleGetDomainTLSCert(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	cert, found, err := rt.domainTLSCert.GetDomainTLSCert(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: get domain tls cert failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainTLSCertResource(domain, cert, found))
}

// handleSetDomainTLSCert handles PUT
// /api/v1/apps/{name}/domains/{domain}/tls-cert: uploads a BYO
// certificate for domain, used by Caddy in place of automatic ACME/
// internal issuance on the next ingress reconcile pass. AbilityRoot, the
// same "real infrastructure, high blast radius" tier PUT
// .../domains/{domain}/auth already reserves for a credential-bearing
// change. Returns 501 without domainTLSCertSecrets configured (no master
// key).
func (rt *Router) handleSetDomainTLSCert(w http.ResponseWriter, r *http.Request) {
	if rt.domainTLSCertSecrets == nil {
		writeError(w, http.StatusNotImplemented, "domain tls cert upload is not configured on this control plane (no master key set)")
		return
	}

	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	var req setDomainTLSCertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Cert = strings.TrimSpace(req.Cert)
	req.Key = strings.TrimSpace(req.Key)
	if req.Cert == "" || req.Key == "" {
		writeError(w, http.StatusBadRequest, "cert and key are both required")
		return
	}

	leaf, err := validateCertKeyPair(req.Cert, req.Key)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	key := store.DomainTLSCertSecretsKey(domain)
	if err := rt.domainTLSCertSecrets.SetValue(r.Context(), key, store.DomainTLSCertCertificateEnvKey, req.Cert); err != nil {
		rt.logger.Error("api: save domain tls certificate failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.domainTLSCertSecrets.SetValue(r.Context(), key, store.DomainTLSCertPrivateKeyEnvKey, req.Key); err != nil {
		rt.logger.Error("api: save domain tls private key failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.domainTLSCert.SetDomainTLSCert(r.Context(), domain, time.Now().UTC(), leaf.NotAfter); err != nil {
		rt.logger.Error("api: set domain tls cert failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	cert, found, err := rt.domainTLSCert.GetDomainTLSCert(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: get domain tls cert failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toDomainTLSCertResource(domain, cert, found))
}

// handleClearDomainTLSCert handles DELETE
// /api/v1/apps/{name}/domains/{domain}/tls-cert: removes domain's BYO
// certificate, reverting it to Caddy's automatic ACME/internal issuance
// on the next ingress reconcile pass. AbilityRoot. Idempotent: clearing
// a domain with none uploaded is not an error.
func (rt *Router) handleClearDomainTLSCert(w http.ResponseWriter, r *http.Request) {
	if rt.domainTLSCertSecrets == nil {
		writeError(w, http.StatusNotImplemented, "domain tls cert upload is not configured on this control plane (no master key set)")
		return
	}

	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	if err := rt.domainTLSCertSecrets.DeleteAll(r.Context(), store.DomainTLSCertSecretsKey(domain)); err != nil {
		rt.logger.Error("api: clear domain tls cert secrets failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if err := rt.domainTLSCert.DeleteDomainTLSCert(r.Context(), domain); err != nil {
		rt.logger.Error("api: clear domain tls cert failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, domainTLSCertResource{Domain: domain})
}
