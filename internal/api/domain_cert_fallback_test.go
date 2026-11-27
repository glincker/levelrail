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

func seedCertUnder(t *testing.T, db *store.DB, seg, domain string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	key := "certificates/" + seg + "/" + domain + "/" + domain + ".crt"
	if err := db.SaveCertStorageValue(context.Background(), key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})); err != nil {
		t.Fatal(err)
	}
}

func TestDomainsListFlagsFallbackAndRenewDropsIt(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	if err := db.UpdateIngressSettings(context.Background(), store.IngressSettings{ACMEEnabled: true}); err != nil {
		t.Fatal(err)
	}
	seedCertUnder(t, db, "local", "app.example.com")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/domains", ""))
	var rows []domainResource
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil || len(rows) == 0 {
		t.Fatalf("list: %v, %s", err, rec.Body.String())
	}
	if !rows[0].Fallback {
		t.Fatalf("fallback flag not set: %+v", rows[0])
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/domains/app.example.com/cert/renew", ""))
	var got renewCertificateResponse
	if rec.Code != http.StatusAccepted || json.Unmarshal(rec.Body.Bytes(), &got) != nil || !got.DroppedFallback {
		t.Fatalf("renew: %d %s", rec.Code, rec.Body.String())
	}
	keys, _ := db.ListCertStorageKeys(context.Background(), "certificates", true)
	if len(keys) != 0 {
		t.Errorf("fallback certificate still stored: %v", keys)
	}
}
