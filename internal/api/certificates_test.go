package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// seedCert generates a real, self-signed leaf certificate for domain with
// the given notBefore/notAfter, PEM-encodes it, and stores it at the same
// certmagic.KeyBuilder.SiteCert-shaped key
// ("certificates/internal/<domain>/<domain>.crt") a real Caddy TLS
// automation run would use (see internal/ingress/certstorage.go), so
// handleListCertificates is exercised against the real storage-key
// convention it parses, not an invented shortcut. "internal" stands in
// for the issuer key segment: every call site in this file only ever
// exercises Caddy's offline internal issuer (see certificateStatus's own
// doc comment on why real ACME isn't in play here), so it isn't a
// parameter.
func seedCert(t *testing.T, db *store.DB, domain string, notBefore, notAfter time.Time) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		Issuer:       pkix.Name{CommonName: "Test CA"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	key := "certificates/internal/" + domain + "/" + domain + ".crt"
	if err := db.SaveCertStorageValue(context.Background(), key, pemBytes); err != nil {
		t.Fatalf("seed cert %q: %v", domain, err)
	}
}

func TestHandleListCertificates_Empty(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/certificates", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []certificateStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d certificates, want 0 for a control plane that has never issued one", len(got))
	}
}

func TestHandleListCertificates_StatusBuckets(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	now := time.Now()

	seedCert(t, db, "healthy.example.com", now.Add(-24*time.Hour), now.Add(60*24*time.Hour))
	seedCert(t, db, "soon.example.com", now.Add(-24*time.Hour), now.Add(5*24*time.Hour))
	seedCert(t, db, "expired.example.com", now.Add(-100*24*time.Hour), now.Add(-1*time.Hour))

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/certificates", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []certificateStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d certificates, want 3: %+v", len(got), got)
	}

	byDomain := make(map[string]certificateStatus, len(got))
	for _, c := range got {
		byDomain[c.Domain] = c
	}

	tests := []struct {
		domain string
		want   string
	}{
		{"healthy.example.com", "healthy"},
		{"soon.example.com", "expiring_soon"},
		{"expired.example.com", "expired"},
	}
	for _, tt := range tests {
		c, ok := byDomain[tt.domain]
		if !ok {
			t.Errorf("missing entry for domain %q in %+v", tt.domain, got)
			continue
		}
		if c.Status != tt.want {
			t.Errorf("domain %q: status = %q, want %q", tt.domain, c.Status, tt.want)
		}
		if c.NotAfter.IsZero() {
			t.Errorf("domain %q: NotAfter is zero", tt.domain)
		}
		if len(c.SANs) != 1 || c.SANs[0] != tt.domain {
			t.Errorf("domain %q: SANs = %v, want [%s]", tt.domain, c.SANs, tt.domain)
		}
	}
}

func TestHandleListCertificates_IgnoresNonLeafKeys(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	now := time.Now()

	seedCert(t, db, "example.com", now.Add(-24*time.Hour), now.Add(60*24*time.Hour))
	// A real certmagic run also stores a sibling private key and a JSON
	// metadata file under the same site prefix; neither is a ".crt" and
	// neither should produce a row.
	if err := db.SaveCertStorageValue(context.Background(), "certificates/internal/example.com/example.com.key", []byte("not a cert")); err != nil {
		t.Fatalf("seed sibling key: %v", err)
	}
	if err := db.SaveCertStorageValue(context.Background(), "certificates/internal/example.com/example.com.json", []byte("{}")); err != nil {
		t.Fatalf("seed sibling meta: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/certificates", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got []certificateStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d certificates, want exactly 1 (the .crt only): %+v", len(got), got)
	}
}

func TestCertificatesRoute_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/certificates", nil)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d for an unauthenticated request", rec.Code, http.StatusUnauthorized)
	}
}

func TestStatusForExpiry(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const window = 14 * 24 * time.Hour

	tests := []struct {
		name     string
		notAfter time.Time
		want     string
	}{
		{"far in the future", now.Add(60 * 24 * time.Hour), "healthy"},
		{"just inside the warning window", now.Add(13 * 24 * time.Hour), "expiring_soon"},
		{"exactly at the warning window boundary", now.Add(window), "healthy"},
		{"already past", now.Add(-1 * time.Hour), "expired"},
		{"expires this instant", now, "expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusForExpiry(tt.notAfter, now, window); got != tt.want {
				t.Errorf("statusForExpiry(%v, %v, %v) = %q, want %q", tt.notAfter, now, window, got, tt.want)
			}
		})
	}
}
