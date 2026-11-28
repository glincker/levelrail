package api

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/glincker/levelrail/internal/store"
)

func testHostKeyLine(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return k.Type() + " " + ssh.FingerprintSHA256(k)
}

func TestSFTPBackupTarget_CreateStoresPinnedKeyAndHidesSecrets(t *testing.T) {
	t.Parallel()
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	hk := testHostKeyLine(t)

	body := `{"name":"nas","provider":"sftp","endpoint":"8.8.8.8:2222","bucket":"backups/lr","access_key_id":"bob","secret_access_key":"pw-123","host_key":"` + hk + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "pw-123") {
		t.Fatal("password echoed")
	}
	var got backupTargetResource
	_ = json.NewDecoder(rec.Body).Decode(&got)
	if got.Provider != "sftp" || got.Endpoint != "8.8.8.8:2222" || got.HostKey != hk || got.Bucket != "backups/lr" {
		t.Fatalf("resource = %+v", got)
	}
	stored, _ := db.GetBackupTarget(context.Background(), got.ID)
	if stored.Endpoint != store.SFTPEndpointScheme+"8.8.8.8:2222" || stored.Region != hk {
		t.Fatalf("stored = %+v", stored)
	}
	seen := map[string]string{}
	for _, c := range setter.calls {
		seen[c.envKey] = c.plaintext
	}
	if seen["secret_access_key"] != "pw-123" || seen["access_key_id"] != "bob" {
		t.Fatalf("secrets = %v", seen)
	}
}

func TestSFTPBackupTarget_Validation(t *testing.T) {
	t.Parallel()
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	hk := testHostKeyLine(t)
	base := func(extra string) string {
		return `{"name":"nas","provider":"sftp","endpoint":"8.8.8.8","bucket":"b","access_key_id":"bob",` + extra + `}`
	}
	cases := map[string]string{
		"no host key (no silent trust)": base(`"secret_access_key":"p"`),
		"malformed host key":            base(`"secret_access_key":"p","host_key":"ssh-ed25519 MD5:aa"`),
		"password and key":              base(`"secret_access_key":"p","private_key":"x","host_key":"` + hk + `"`),
		"no credential":                 base(`"host_key":"` + hk + `"`),
		"bad key material":              base(`"private_key":"not a key","host_key":"` + hk + `"`),
		"traversal":                     strings.Replace(base(`"secret_access_key":"p","host_key":"`+hk+`"`), `"b"`, `"../x"`, 1),
		"loopback":                      strings.Replace(base(`"secret_access_key":"p","host_key":"`+hk+`"`), "8.8.8.8", "127.0.0.1", 1),
		"metadata":                      strings.Replace(base(`"secret_access_key":"p","host_key":"`+hk+`"`), "8.8.8.8", "169.254.169.254", 1),
		"bad port":                      strings.Replace(base(`"secret_access_key":"p","host_key":"`+hk+`"`), "8.8.8.8", "8.8.8.8:99999", 1),
	}
	for name, body := range cases {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, body = %s", name, rec.Code, rec.Body.String())
		}
	}
	if len(setter.calls) != 0 {
		t.Fatalf("credentials stored for rejected requests: %d", len(setter.calls))
	}
}

func TestSFTPBackupTarget_RepinNeedsNewCredentials(t *testing.T) {
	t.Parallel()
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	hk := testHostKeyLine(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets",
		`{"name":"nas","provider":"sftp","endpoint":"8.8.8.8","bucket":"b","access_key_id":"bob","secret_access_key":"p","host_key":"`+hk+`"}`))
	var created backupTargetResource
	_ = json.NewDecoder(rec.Body).Decode(&created)

	put := func(body string) int {
		r := httptest.NewRecorder()
		rt.Handler().ServeHTTP(r, authedRequest(t, cookie, http.MethodPut, "/api/v1/backup-targets/"+created.ID, body))
		return r.Code
	}
	other := testHostKeyLine(t)
	if c := put(`{"name":"nas","provider":"sftp","endpoint":"8.8.8.8","bucket":"b","host_key":"` + other + `"}`); c != http.StatusBadRequest {
		t.Errorf("re-pin without credentials: %d, want 400", c)
	}
	if c := put(`{"name":"nas2","provider":"sftp","endpoint":"8.8.8.8","bucket":"c","host_key":"` + hk + `"}`); c != http.StatusOK {
		t.Errorf("same key, no credentials: %d, want 200", c)
	}
	if c := put(`{"name":"nas","provider":"sftp","endpoint":"8.8.8.8","bucket":"b","host_key":"` + other + `","access_key_id":"bob","secret_access_key":"q"}`); c != http.StatusOK {
		t.Errorf("re-pin with credentials: %d, want 200", c)
	}
}

func TestSFTPHostKeyScan_RefusesInternalHosts(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouterWithBackupSecrets(t, &fakeBackupSecretsSetter{})
	cookie := loginTestSession(t, rt, db)
	var first string
	for _, ep := range []string{"127.0.0.1:22", "169.254.169.254", "[64:ff9b::a9fe:a9fe]:22", "100.100.100.200"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets/sftp-host-key", `{"endpoint":"`+ep+`"}`))
		if rec.Code != http.StatusBadGateway {
			t.Errorf("%q: status = %d, body = %s", ep, rec.Code, rec.Body.String())
		}
		var body struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(rec.Body).Decode(&body)
		if first == "" {
			first = body.Error
		} else if body.Error != first {
			t.Errorf("%q: answer %q differs from %q", ep, body.Error, first)
		}
	}
	for _, ep := range []string{"", "host/with/slash"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets/sftp-host-key", `{"endpoint":"`+ep+`"}`))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, body = %s", ep, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/backup-targets/sftp-host-key", strings.NewReader(`{}`)))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated: %d", rec.Code)
	}
}

func TestSFTPBackupTarget_CustomProviderCannotCarrySFTPEndpoint(t *testing.T) {
	t.Parallel()
	setter := &fakeBackupSecretsSetter{}
	rt, db := newTestRouterWithBackupSecrets(t, setter)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets",
		`{"name":"x","provider":"custom","endpoint":"sftp+127.0.0.1:22","region":"r","bucket":"b","access_key_id":"a","secret_access_key":"s"}`))
	if rec.Code != http.StatusBadRequest || len(setter.calls) != 0 {
		t.Fatalf("status = %d, calls = %d, body = %s", rec.Code, len(setter.calls), rec.Body.String())
	}
}

func TestSFTPHostKeyScan_RateLimited(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouterWithBackupSecrets(t, &fakeBackupSecretsSetter{})
	cookie := loginTestSession(t, rt, db)
	var limited bool
	for i := 0; i < 40 && !limited; i++ {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/backup-targets/sftp-host-key", `{"endpoint":"127.0.0.1:22"}`))
		limited = rec.Code == http.StatusTooManyRequests
	}
	if !limited {
		t.Fatal("scan was never rate limited")
	}
}
