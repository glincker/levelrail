package api

import (
	"crypto/md5" //nolint:gosec // OCI fingerprint format
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	oUserB = "ocid1.user.oc1..bbbbbbbb99999999"
	oCompB = "ocid1.compartment.oc1..cccccccc99999999"
)

func oracleTestKey(t *testing.T) (pemText, fp string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	sum := md5.Sum(der) //nolint:gosec // OCI fingerprint format
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})), strings.Join(parts, ":")
}

func TestOracleAccountsSideBySideAreIsolated(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{} // compartment -> keyId user part
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/20160918")
		switch {
		case p == "/instances":
			comp := r.URL.Query().Get("compartmentId")
			mu.Lock()
			seen[comp] = r.Header.Get("Authorization")
			mu.Unlock()
			name := "alpha-vm"
			if comp == oCompB {
				name = "beta-vm"
			}
			_, _ = w.Write([]byte(`[{"id":"` + oInst + `","displayName":"` + name + `","lifecycleState":"RUNNING","shape":"VM.Standard.A1.Flex","region":"us-phoenix-1"}]`))
		case strings.HasSuffix(p, "/regionSubscriptions"):
			_, _ = w.Write([]byte(`[{"regionName":"us-phoenix-1","status":"READY","isHomeRegion":true},{"regionName":"eu-frankfurt-1","status":"IN_PROGRESS"}]`))
		case p == "/20200107/usage":
			_, _ = w.Write([]byte(`{"items":[]}`))
		default:
			w.WriteHeader(http.StatusForbidden)
		}
	}))
	defer srv.Close()

	secrets := newFakeNodeProviderSecrets()
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db,
		WithNodeProviderSecrets(secrets), WithAgentCAFingerprint("test-ca-fingerprint"))
	rt.oracleTestBase = srv.URL
	cookie := loginTestSession(t, rt, db)

	add := func(id, user, comp string) {
		pemText, fp := oracleTestKey(t)
		body, _ := json.Marshal(map[string]any{
			"id": id, "provider": "oracle", "verify": true, "tenancy_ocid": oTenancy, "user_ocid": user, "fingerprint": fp,
			"private_key_pem": pemText, "region": "us-phoenix-1", "compartment_ocid": comp,
		})
		rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/cloud-accounts", string(body))
		if rec.Code != http.StatusCreated {
			t.Fatalf("add %s = %d %s", id, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "PRIVATE KEY") || strings.Contains(rec.Body.String(), "private_key") {
			t.Fatalf("account response leaked the key: %s", rec.Body.String())
		}
	}
	add("ora-a", oUser, oComp)
	add("ora-b", oUserB, oCompB)

	names := func(acct, query string) string {
		rec := provReq(rt, cookie, t, http.MethodGet, "/api/v1/cloud-accounts/"+acct+"/instances"+query, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("list %s = %d %s", acct, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	if got := names("ora-a", "?fresh=1"); !strings.Contains(got, "alpha-vm") || strings.Contains(got, "beta-vm") {
		t.Fatalf("account a list = %s", got)
	}
	if got := names("ora-b", "?fresh=1"); !strings.Contains(got, "beta-vm") || strings.Contains(got, "alpha-vm") {
		t.Fatalf("account b list = %s", got)
	}
	if rec := provReq(rt, cookie, t, http.MethodGet, "/api/v1/cloud-accounts/ora-a/instances?compartment="+oCompB, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("foreign compartment = %d, want 403", rec.Code)
	}
	mu.Lock()
	if !strings.Contains(seen[oComp], "/"+oUser+"/") || !strings.Contains(seen[oCompB], "/"+oUserB+"/") {
		t.Fatalf("each account must sign with its own key id")
	}
	mu.Unlock()

	rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/cloud-accounts/ora-a/probe", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("probe = %d %s", rec.Code, rec.Body.String())
	}
	var probe struct {
		Capabilities []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
			Hint string `json:"hint"`
		} `json:"capabilities"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &probe)
	got := map[string]bool{}
	for _, c := range probe.Capabilities {
		got[c.Name] = c.OK
		if !c.OK && c.Hint == "" {
			t.Fatalf("capability %s failed without a fix hint", c.Name)
		}
	}
	if !got["compute"] || !got["usage"] || got["budgets"] {
		t.Fatalf("capabilities = %v, want compute and usage ok, budgets refused", got)
	}

	rec = provReq(rt, cookie, t, http.MethodGet, "/api/v1/cloud-accounts/ora-b/regions", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "us-phoenix-1") || strings.Contains(rec.Body.String(), "eu-frankfurt-1") {
		t.Fatalf("regions = %d %s", rec.Code, rec.Body.String())
	}
}

func TestOraclePolicyKitEndpoint(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithNodeProviderSecrets(newFakeNodeProviderSecrets()))
	cookie := loginTestSession(t, rt, db)
	rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/cloud-accounts/oracle/policy-kit", `{"group_name":"lr-group","compartment":"levelrail-test","policy_name":"lr-pol"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy kit = %d %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"read instance-family in compartment levelrail-test", "read usage-report in tenancy", "read usage-budgets in tenancy", "lr-pol"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("missing %q in %s", want, rec.Body.String())
		}
	}
	if strings.Contains(rec.Body.String(), "manage") {
		t.Fatalf("default kit must be read only: %s", rec.Body.String())
	}
	if rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/cloud-accounts/oracle/policy-kit", `{"group_name":"g","compartment":"tenancy"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("tenancy scope = %d, want 400", rec.Code)
	}
}
