package api

import (
	"bytes"
	"context"
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
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	ocTenancy = "ocid1.tenancy.oc1..aaaaaaaatenancy1"
	ocComp    = "ocid1.compartment.oc1..aaaaaaaacomp0001"
	ocUser    = "ocid1.user.oc1..aaaaaaaacanaryuser77"
	ocPath    = "/api/v1/settings/cloud-connections/oracle"
)

type oracleFixture struct {
	rt      *Router
	db      *store.DB
	mgr     *secrets.Manager
	logs    *bytes.Buffer
	keyPEM  string
	keyLine string
	fp      string
}

func newOracleFixture(t *testing.T) oracleFixture {
	t.Helper()
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	mgr := secrets.NewManager(db, mk)
	logs := &bytes.Buffer{}
	rt := NewRouter(slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), testBrand(), db, WithComposeSecrets(mgr))
	rt.runtimeSettings = runtimesettings.New(db, func(k string) (string, bool) { return "true", k == "APP_CLOUD_CONNECTIONS_ENABLED" })
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&k.PublicKey)
	sum := md5.Sum(der) //nolint:gosec // OCI fingerprint format
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}))
	lines := strings.Split(keyPEM, "\n")
	return oracleFixture{rt: rt, db: db, mgr: mgr, logs: logs, keyPEM: keyPEM, keyLine: lines[len(lines)/2], fp: strings.Join(parts, ":")}
}

func (f oracleFixture) body(name, comp, keyPEM string) string {
	b, _ := json.Marshal(map[string]string{"name": name, "tenancy_ocid": ocTenancy, "region": "us-phoenix-1", "compartment_ocid": comp,
		"user_ocid": ocUser, "fingerprint": f.fp, "private_key_pem": keyPEM})
	return string(b)
}

func (f oracleFixture) post(t *testing.T, cookie *http.Cookie, remote, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := authedRequest(t, cookie, http.MethodPost, ocPath, body)
	req.RemoteAddr = remote
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestOracleConnectionCreateSealsAndNeverEchoes(t *testing.T) {
	f := newOracleFixture(t)
	cookie := loginTestSession(t, f.rt, f.db)
	rec := f.post(t, cookie, "127.0.0.1:5000", f.body("oci-main", ocComp, f.keyPEM))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" || !strings.Contains(rec.Body.String(), f.fp[:11]) {
		t.Fatalf("headers or prefix wrong: %v %s", rec.Header(), rec.Body.String())
	}
	svc := &cloudconn.Service{Rows: f.rt.cloudConnRows, Providers: cloudconn.DefaultRegistry()}
	c, err := svc.Get(context.Background(), "oci-main")
	if err != nil || c.AuthMethod != cloudconn.AuthStatic || c.Provider != cloudconn.ProviderOracle {
		t.Fatalf("stored connection %+v %v", c, err)
	}
	ns, key, _ := strings.Cut(c.CredentialRef, "/")
	sealed, err := f.db.GetSecretValue(context.Background(), ns, key)
	if err != nil || len(sealed) == 0 {
		t.Fatalf("sealed value missing: %v", err)
	}
	opened, err := cloudconn.OpenOracleSealed(context.Background(), f.mgr, c)
	if err != nil || opened.Secret.UserOCID != ocUser {
		t.Fatalf("sealed bundle does not open: %v", err)
	}
	audit, err := f.db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	if err != nil || len(audit) == 0 {
		t.Fatalf("no audit rows: %v", err)
	}
	var rows strings.Builder
	for _, a := range audit {
		rows.WriteString(a.Detail + a.Path + a.Action + "\n")
	}
	if !strings.Contains(rows.String(), auditCloudConnCreated) {
		t.Fatalf("no create audit row: %s", rows.String())
	}
	for where, s := range map[string]string{"sealed": string(sealed), "response": rec.Body.String(), "audit": rows.String(), "log": f.logs.String()} {
		if strings.Contains(s, f.keyLine) || strings.Contains(s, "PRIVATE KEY") || strings.Contains(s, "canaryuser77") {
			t.Errorf("%s leaks credential material", where)
		}
	}
	if dup := f.post(t, cookie, "127.0.0.1:5000", f.body("oci-main", ocComp, f.keyPEM)); dup.Code != http.StatusConflict || !strings.Contains(dup.Body.String(), "connection_exists") {
		t.Fatalf("duplicate = %d %s", dup.Code, dup.Body.String())
	}
}

func TestOracleConnectionRefusals(t *testing.T) {
	f := newOracleFixture(t)
	cookie := loginTestSession(t, f.rt, f.db)
	enc := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Headers: map[string]string{"Proc-Type": "4,ENCRYPTED", "DEK-Info": "AES-256-CBC,00112233445566778899AABBCCDDEEFF"}, Bytes: []byte("opaque")}))
	for name, c := range map[string]struct{ remote, body, want string }{
		"tenancy as compartment":  {"127.0.0.1:1", f.body("a1", ocTenancy, f.keyPEM), "compartment"},
		"encrypted key":           {"127.0.0.1:1", f.body("a2", ocComp, enc), "encrypted"},
		"bad user ocid":           {"127.0.0.1:1", strings.Replace(f.body("a3", ocComp, f.keyPEM), ocUser, "ocid1.group.oc1..x", 1), "user"},
		"plain http off loopback": {"198.51.100.7:1", f.body("a4", ocComp, f.keyPEM), "tls_required"},
	} {
		rec := f.post(t, cookie, c.remote, c.body)
		if rec.Code < 400 || rec.Code >= 500 || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s = %d %s", name, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String()+f.logs.String(), f.keyLine) {
			t.Errorf("%s leaked the key", name)
		}
	}
	list, _ := (&cloudconn.Service{Rows: f.rt.cloudConnRows, Providers: cloudconn.DefaultRegistry()}).List(context.Background())
	if len(list) != 0 {
		t.Fatalf("refused requests created %d connections", len(list))
	}
}

func TestOracleConnectionNeedsRoot(t *testing.T) {
	f := newOracleFixture(t)
	const plaintext = "oracle-write-token" //nolint:gosec // fixture
	if err := f.db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_ow", Name: "w", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite, AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, ocPath, strings.NewReader(f.body("oci-x", ocComp, f.keyPEM)))
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-root = %d", rec.Code)
	}
}
