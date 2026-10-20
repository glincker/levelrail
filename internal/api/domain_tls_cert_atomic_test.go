package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// flakyTLSSecrets fails SetValue for one slot, the half-succeeded upload.
type flakyTLSSecrets struct {
	*fakeDomainTLSCertSecrets
	failEnvKey string
}

func (f *flakyTLSSecrets) SetValue(ctx context.Context, serviceName, envKey, plaintext string) error {
	if envKey == f.failEnvKey {
		return errors.New("disk full")
	}
	return f.fakeDomainTLSCertSecrets.SetValue(ctx, serviceName, envKey, plaintext)
}

func (f *flakyTLSSecrets) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	v, ok := f.values[serviceName+"/"+envKey]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func putTLSCert(t *testing.T, rt *Router, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	certPEM, keyPEM := genCertKeyPEM(t, time.Now().Add(-time.Hour), time.Now().AddDate(1, 0, 0))
	body, err := json.Marshal(map[string]string{"cert": certPEM, "key": keyPEM})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/app.example.com/tls-cert", string(body)))
	return rec
}

func TestHandleSetDomainTLSCert_KeyWriteFailureLeavesNoOrphans(t *testing.T) {
	s := &flakyTLSSecrets{fakeDomainTLSCertSecrets: newFakeDomainTLSCertSecrets(), failEnvKey: store.DomainTLSCertPrivateKeyEnvKey}
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := putTLSCert(t, rt, cookie)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
	var got codedError
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Code != errCodeTLSCertSaveKey {
		t.Errorf("body = %s, want code %s naming the failing step", rec.Body.String(), errCodeTLSCertSaveKey)
	}
	if len(s.values) != 0 {
		t.Errorf("secrets = %v, want the half-written certificate removed", s.values)
	}
	if _, found, _ := db.GetDomainTLSCert(context.Background(), "app.example.com"); found {
		t.Error("a certificate row exists after a failed upload")
	}
}

func TestHandleSetDomainTLSCert_FailedReplaceRestoresPreviousPair(t *testing.T) {
	s := &flakyTLSSecrets{fakeDomainTLSCertSecrets: newFakeDomainTLSCertSecrets()}
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	if rec := putTLSCert(t, rt, cookie); rec.Code != http.StatusOK {
		t.Fatalf("first upload status = %d, body = %s", rec.Code, rec.Body.String())
	}
	key := store.DomainTLSCertSecretsKey("app.example.com")
	oldCert := s.values[key+"/"+store.DomainTLSCertCertificateEnvKey]
	oldKey := s.values[key+"/"+store.DomainTLSCertPrivateKeyEnvKey]

	s.failEnvKey = store.DomainTLSCertPrivateKeyEnvKey
	if rec := putTLSCert(t, rt, cookie); rec.Code != http.StatusInternalServerError {
		t.Fatalf("second upload status = %d, want 500", rec.Code)
	}
	s.failEnvKey = ""
	if s.values[key+"/"+store.DomainTLSCertCertificateEnvKey] != oldCert || s.values[key+"/"+store.DomainTLSCertPrivateKeyEnvKey] != oldKey {
		t.Error("previous certificate and key were not restored after the failed replace")
	}
	if _, found, _ := db.GetDomainTLSCert(context.Background(), "app.example.com"); !found {
		t.Error("the previous certificate row was lost")
	}
}

func TestHandleSetDomainTLSCert_NoMasterKeyNamesTheNextStep(t *testing.T) {
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, nil)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := putTLSCert(t, rt, cookie)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
	var got codedError
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Code != errCodeTLSCertNoMasterKey {
		t.Errorf("body = %s, want code %s", rec.Body.String(), errCodeTLSCertNoMasterKey)
	}
}

func TestHandleGetDomainTLSCert_AppliedOnlyAfterIngressPass(t *testing.T) {
	s := newFakeDomainTLSCertSecrets()
	rt, db := newTestRouterWithDomainTLSCertSecrets(t, s)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)
	if rec := putTLSCert(t, rt, cookie); rec.Code != http.StatusOK {
		t.Fatalf("upload status = %d", rec.Code)
	}

	read := func() domainTLSCertResource {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/domains/app.example.com/tls-cert", ""))
		var res domainTLSCertResource
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return res
	}
	if res := read(); res.Applied {
		t.Errorf("applied = true before any ingress pass, want pending: %+v", res)
	}

	time.Sleep(5 * time.Millisecond)
	if err := db.ReplaceDomainConditions(context.Background(), nil); err != nil {
		t.Fatalf("ReplaceDomainConditions() error = %v", err)
	}
	if res := read(); !res.Applied {
		t.Errorf("applied = false after a clean ingress pass: %+v", res)
	}

	if err := db.ReplaceDomainConditions(context.Background(), []store.DomainCondition{{
		Domain: "app.example.com", Code: store.DomainCondCustomCertUnresolved, Severity: store.DomainConditionDegraded, Message: "key unreadable",
	}}); err != nil {
		t.Fatalf("ReplaceDomainConditions() error = %v", err)
	}
	if res := read(); res.Applied || res.ApplyIssue != "key unreadable" {
		t.Errorf("got %+v, want pending with the ingress issue", res)
	}
}
