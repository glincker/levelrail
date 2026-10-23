package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
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
	// ListDomainTLSCerts backs handleListCertificates' Source field (api/
	// certificates.go): every domain with a custom cert on file, so the
	// certificate center page can tell "custom" apart from "acme" per row.
	ListDomainTLSCerts(ctx context.Context) ([]store.DomainTLSCert, error)
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
	// Applied is true once an ingress pass after the upload loaded the
	// certificate; false means it is stored but still pending, and ApplyIssue
	// says why when the last pass could not use it.
	Applied    bool   `json:"applied"`
	ApplyIssue string `json:"apply_issue,omitempty"`
}

const (
	errCodeTLSCertNoMasterKey     = "tls_cert_no_master_key"
	errCodeTLSCertSaveCert        = "tls_cert_save_certificate_failed"
	errCodeTLSCertSaveKey         = "tls_cert_save_key_failed"
	errCodeTLSCertSaveRow         = "tls_cert_save_record_failed"
	errCodeTLSCertDeleteRow       = "tls_cert_delete_record_failed"
	errCodeTLSCertDeleteStoredKey = "tls_cert_delete_stored_key_failed"
	msgTLSCertNoMasterKey         = "certificate upload needs the control plane master key, which is not configured: set APP_MASTER_KEY (or APP_MASTER_KEY_FILE) and restart the control plane"
)

// secretResolver is the optional read side of DomainTLSCertSecrets, used to
// restore the previous certificate when an overwrite fails part way.
type secretResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// domainConditionsReader is the optional ingress-pass evidence the store
// offers, used to tell an applied certificate from a pending one.
type domainConditionsReader interface {
	ListDomainConditions(ctx context.Context) ([]store.DomainCondition, time.Time, bool, error)
}

// annotateTLSCertApplied sets Applied and ApplyIssue from the last ingress pass.
func (rt *Router) annotateTLSCertApplied(ctx context.Context, res *domainTLSCertResource, cert store.DomainTLSCert) {
	if !res.Enabled {
		return
	}
	reader, ok := rt.domainTLSCert.(domainConditionsReader)
	if !ok {
		return
	}
	conds, at, known, err := reader.ListDomainConditions(ctx)
	if err != nil {
		rt.logger.Warn("api: read domain conditions failed", slog.String("error", err.Error()), slog.String("domain", res.Domain))
		return
	}
	if !known || at.Before(cert.UploadedAt) {
		return
	}
	for _, c := range conds {
		if c.Domain == res.Domain && c.Code == store.DomainCondCustomCertUnresolved {
			res.ApplyIssue = c.Message
			return
		}
	}
	res.Applied = true
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
// crypto/tls.X509KeyPair, the exact check Caddy's own load_pem loader
// performs (modules/caddytls/pemloader.go), so a rejected pair here
// would also fail at Caddy's own load step. An already-expired
// certificate is rejected rather than accepted with a warning: uploading
// dead cert material is almost certainly a mistake worth failing loudly
// on, not silently accepting.
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
		rt.internalError(w, "api: get domain tls cert failed", err, slog.String("domain", domain))
		return
	}
	res := toDomainTLSCertResource(domain, cert, found)
	rt.annotateTLSCertApplied(r.Context(), &res, cert)
	writeJSON(w, http.StatusOK, res)
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
		writeCodedError(w, http.StatusNotImplemented, errCodeTLSCertNoMasterKey, msgTLSCertNoMasterKey)
		return
	}

	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	var req setDomainTLSCertRequest
	if !decodeReq(w, r, &req) {
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
	prev, hadPrev, err := rt.snapshotTLSCert(r.Context(), domain, key)
	if err != nil {
		rt.logger.Error("api: snapshot previous domain tls cert failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeCodedError(w, http.StatusInternalServerError, errCodeTLSCertSaveCert, "could not read the current certificate before replacing it, nothing was changed")
		return
	}

	steps := []struct {
		code, label, envKey, value string
	}{
		{errCodeTLSCertSaveCert, "certificate", store.DomainTLSCertCertificateEnvKey, req.Cert},
		{errCodeTLSCertSaveKey, "private key", store.DomainTLSCertPrivateKeyEnvKey, req.Key},
	}
	for _, st := range steps {
		if err := rt.domainTLSCertSecrets.SetValue(r.Context(), key, st.envKey, st.value); err != nil {
			rt.rollbackTLSCert(r.Context(), domain, key, prev, hadPrev)
			rt.logger.Error("api: save domain tls "+st.label+" failed", slog.String("error", err.Error()), slog.String("domain", domain))
			writeCodedError(w, http.StatusInternalServerError, st.code, "saving the "+st.label+" failed, the upload was rolled back and nothing changed")
			return
		}
	}

	if err := rt.domainTLSCert.SetDomainTLSCert(r.Context(), domain, time.Now().UTC(), leaf.NotAfter); err != nil {
		rt.rollbackTLSCert(r.Context(), domain, key, prev, hadPrev)
		rt.logger.Error("api: set domain tls cert failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeCodedError(w, http.StatusInternalServerError, errCodeTLSCertSaveRow, "recording the certificate failed, the upload was rolled back and nothing changed")
		return
	}

	rt.nudgeReconciler()
	cert, found, err := rt.domainTLSCert.GetDomainTLSCert(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: get domain tls cert failed", err, slog.String("domain", domain))
		return
	}
	res := toDomainTLSCertResource(domain, cert, found)
	rt.annotateTLSCertApplied(r.Context(), &res, cert)
	writeJSON(w, http.StatusOK, res)
}

type tlsCertSnapshot struct{ cert, key string }

// snapshotTLSCert reads the pair about to be overwritten so a failed upload
// can restore it; hadPrev is false when there is nothing to restore.
func (rt *Router) snapshotTLSCert(ctx context.Context, domain, key string) (tlsCertSnapshot, bool, error) {
	_, found, err := rt.domainTLSCert.GetDomainTLSCert(ctx, domain)
	if err != nil || !found {
		return tlsCertSnapshot{}, false, err
	}
	res, ok := rt.domainTLSCertSecrets.(secretResolver)
	if !ok {
		return tlsCertSnapshot{}, false, nil
	}
	certPEM, err := res.Resolve(ctx, key, store.DomainTLSCertCertificateEnvKey)
	if err != nil {
		return tlsCertSnapshot{}, false, fmt.Errorf("resolve current certificate: %w", err)
	}
	keyPEM, err := res.Resolve(ctx, key, store.DomainTLSCertPrivateKeyEnvKey)
	if err != nil {
		return tlsCertSnapshot{}, false, fmt.Errorf("resolve current private key: %w", err)
	}
	return tlsCertSnapshot{cert: certPEM, key: keyPEM}, true, nil
}

// rollbackTLSCert restores the previous pair, or removes the half-written one
// when the domain had no custom certificate before.
func (rt *Router) rollbackTLSCert(ctx context.Context, domain, key string, prev tlsCertSnapshot, hadPrev bool) {
	if hadPrev {
		for envKey, v := range map[string]string{store.DomainTLSCertCertificateEnvKey: prev.cert, store.DomainTLSCertPrivateKeyEnvKey: prev.key} {
			if err := rt.domainTLSCertSecrets.SetValue(ctx, key, envKey, v); err != nil {
				rt.logger.Error("api: restore previous domain tls secret failed", slog.String("error", err.Error()), slog.String("domain", domain), slog.String("slot", envKey))
			}
		}
		return
	}
	if _, found, err := rt.domainTLSCert.GetDomainTLSCert(ctx, domain); err == nil && found {
		return
	}
	if err := rt.domainTLSCertSecrets.DeleteAll(ctx, key); err != nil {
		rt.logger.Error("api: clean up partial domain tls secrets failed", slog.String("error", err.Error()), slog.String("domain", domain))
	}
}

// handleClearDomainTLSCert handles DELETE
// /api/v1/apps/{name}/domains/{domain}/tls-cert: removes domain's BYO
// certificate, reverting it to Caddy's automatic ACME/internal issuance
// on the next ingress reconcile pass. AbilityRoot. Idempotent: clearing
// a domain with none uploaded is not an error.
func (rt *Router) handleClearDomainTLSCert(w http.ResponseWriter, r *http.Request) {
	if rt.domainTLSCertSecrets == nil {
		writeCodedError(w, http.StatusNotImplemented, errCodeTLSCertNoMasterKey, msgTLSCertNoMasterKey)
		return
	}

	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	if err := rt.domainTLSCert.DeleteDomainTLSCert(r.Context(), domain); err != nil {
		rt.logger.Error("api: clear domain tls cert failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeCodedError(w, http.StatusInternalServerError, errCodeTLSCertDeleteRow, "removing the certificate record failed, the custom certificate is still in use")
		return
	}
	rt.nudgeReconciler()
	if err := rt.domainTLSCertSecrets.DeleteAll(r.Context(), store.DomainTLSCertSecretsKey(domain)); err != nil {
		rt.logger.Error("api: clear domain tls cert secrets failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeCodedError(w, http.StatusInternalServerError, errCodeTLSCertDeleteStoredKey, "the custom certificate is no longer used, but its stored key could not be erased: send the removal again")
		return
	}
	writeJSON(w, http.StatusOK, domainTLSCertResource{Domain: domain})
}

// renewCertificateResponse is POST .../cert/renew's response body.
type renewCertificateResponse struct {
	Domain string `json:"domain"`
	// HadStoredCertificate is true when a previously-issued certificate
	// was found and removed to force re-issuance; false when the renew
	// request found none (either domain has never been issued one yet,
	// or ingress hasn't reconciled it yet), still a valid "request it now"
	// call, not an error.
	HadStoredCertificate bool   `json:"had_stored_certificate"`
	Status               string `json:"status"`
}

// handleRenewDomainCertificate handles POST
// /api/v1/apps/{name}/domains/{domain}/cert/renew: clears domain's stored
// certificate and nudges the reconciler, so the next pass re-obtains one
// (ACME or internal, whichever that domain's policy uses) within seconds
// instead of waiting for the normal resync interval. AbilityRoot, same
// tier as PUT/DELETE .../tls-cert: real infra, real blast radius.
func (rt *Router) handleRenewDomainCertificate(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}

	if _, found, err := rt.domainTLSCert.GetDomainTLSCert(r.Context(), domain); err != nil {
		rt.internalError(w, "api: renew domain certificate: get domain tls cert failed", err, slog.String("domain", domain))
		return
	} else if found {
		writeError(w, http.StatusConflict, fmt.Sprintf("domain %q uses an operator-uploaded certificate; renewal only applies to automatic ACME/internal issuance. Remove the custom certificate first.", domain))
		return
	}

	hadCert, err := alerting.DeleteCertificateForDomain(r.Context(), rt.certs, rt.certs, domain)
	if err != nil {
		rt.internalError(w, "api: renew domain certificate failed", err, slog.String("domain", domain))
		return
	}

	rt.nudgeReconciler()
	rt.logger.Info("api: domain certificate renewal requested", slog.String("domain", domain), slog.Bool("had_stored_certificate", hadCert))

	writeJSON(w, http.StatusAccepted, renewCertificateResponse{
		Domain:               domain,
		HadStoredCertificate: hadCert,
		Status:               "renewal_requested",
	})
}

// annotateDomainIssues attaches the last ingress pass's per-domain problems.
// Best effort: a read failure only omits them.
func (rt *Router) annotateDomainIssues(ctx context.Context, rows []domainResource) {
	reader, ok := rt.domainTLSCert.(domainConditionsReader)
	if !ok || len(rows) == 0 {
		return
	}
	conds, _, known, err := reader.ListDomainConditions(ctx)
	if err != nil {
		rt.logger.Warn("api: list domains: conditions skipped", slog.String("error", err.Error()))
		return
	}
	if !known || len(conds) == 0 {
		return
	}
	byDomain := make(map[string][]store.DomainCondition, len(conds))
	for _, c := range conds {
		byDomain[c.Domain] = append(byDomain[c.Domain], c)
	}
	for i := range rows {
		rows[i].Issues = byDomain[rows[i].Domain]
	}
}
