package api

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testSessionJWT(exp time.Time) string {
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"RS256"}`)) + "." + enc([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix()))) + "." + enc([]byte("sig"))
}

func TestOracleSessionAccountLifecycle(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db,
		WithNodeProviderSecrets(newFakeNodeProviderSecrets()), WithAgentCAFingerprint("test-ca-fingerprint"))
	rt.oracleTestBase = srv.URL
	cookie := loginTestSession(t, rt, db)
	pemText, _ := oracleTestKey(t)
	good := testSessionJWT(time.Now().Add(time.Hour))

	imp, _ := json.Marshal(map[string]any{"profile_config": "[DEFAULT]\ntenancy=" + oTenancy + "\nregion=us-phoenix-1\nkey_file=/x\n", "session_token": good, "private_key_pem": pemText})
	rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/cloud-accounts/oracle/session-import", string(imp))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), good) || strings.Contains(rec.Body.String(), "PRIVATE KEY") {
		t.Fatalf("import = %d %s", rec.Code, rec.Body.String())
	}

	create := func(tok string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(map[string]any{"id": "ora-s", "provider": "oracle", "method": "oci_session", "verify": true, "tenancy_ocid": oTenancy,
			"region": "us-phoenix-1", "compartment_ocid": oComp, "session_token": tok, "private_key_pem": pemText})
		return provReq(rt, cookie, t, http.MethodPost, "/api/v1/cloud-accounts", string(b))
	}
	if rec := create(testSessionJWT(time.Now().Add(-time.Minute))); rec.Code != http.StatusBadRequest {
		t.Fatalf("expired create = %d", rec.Code)
	}
	rec = create(good)
	if rec.Code != http.StatusCreated || strings.Contains(rec.Body.String(), good) || strings.Contains(rec.Body.String(), "PRIVATE KEY") {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(gotAuth, `Signature version="1",keyId="ST$`+good) {
		t.Fatalf("verify did not sign with the session keyId: %q", gotAuth)
	}

	// Near expiry the account reports needs_relogin, then a PATCH restores it.
	near, _ := json.Marshal(map[string]any{"session_token": testSessionJWT(time.Now().Add(2 * time.Minute)), "private_key_pem": pemText})
	if rec := provReq(rt, cookie, t, http.MethodPatch, "/api/v1/cloud-accounts/ora-s", string(near)); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"needs_relogin":true`) {
		t.Fatalf("patch near = %d %s", rec.Code, rec.Body.String())
	}
	if rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/cloud-accounts/ora-s/verify", "{}"); !strings.Contains(rec.Body.String(), "needs_relogin") || !strings.Contains(rec.Body.String(), "oci session authenticate") {
		t.Fatalf("verify = %d %s", rec.Code, rec.Body.String())
	}
	fresh, _ := json.Marshal(map[string]any{"session_token": testSessionJWT(time.Now().Add(time.Hour)), "private_key_pem": pemText})
	if rec := provReq(rt, cookie, t, http.MethodPatch, "/api/v1/cloud-accounts/ora-s", string(fresh)); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "needs_relogin") {
		t.Fatalf("patch fresh = %d %s", rec.Code, rec.Body.String())
	}
}
