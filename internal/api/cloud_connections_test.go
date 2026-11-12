package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newCloudRouter(t *testing.T, enabled bool, class string, probeErr error) (*Router, *store.DB, map[string]*cloudconn.Fake) {
	rt, db := newTestRouter(t)
	rt.runtimeSettings = runtimesettings.New(db, func(k string) (string, bool) {
		if enabled && k == "APP_CLOUD_CONNECTIONS_ENABLED" {
			return "true", true
		}
		return "", false
	})
	reg, fakes := cloudconn.FakeRegistry(class, probeErr)
	rt.SetCloudProviders(reg)
	return rt, db, fakes
}

const cloudOCIBody = `{"name":"oci-test","provider":"oracle","auth_method":"secret_ref","credential_ref":"secret://vault/oci#key",` +
	`"config":{"region":"us-ashburn-1","account_id":"ocid1.tenancy.x","compartment":"ocid1.compartment.x"}}`

func TestCloudConnectionsOffBlocksEverything(t *testing.T) {
	rt, db, _ := newCloudRouter(t, false, "", nil)
	cookie := loginTestSession(t, rt, db)
	for _, c := range [][3]string{
		{"GET", cloudConnectionsPath, ""}, {"POST", cloudConnectionsPath, cloudOCIBody},
		{"PUT", cloudConnectionsPath + "/a", `{}`}, {"DELETE", cloudConnectionsPath + "/a", ""}, {"POST", cloudConnectionsPath + "/a/test", ""},
	} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, c[0], c[1], c[2]))
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "cloud.connections_enabled") {
			t.Errorf("%s %s = %d %s, want 403 naming the setting", c[0], c[1], rec.Code, rec.Body.String())
		}
	}
}

func TestCloudConnectionsWriteNeedsRoot(t *testing.T) {
	rt, db, _ := newCloudRouter(t, true, "", nil)
	const plaintext = "cloud-write-token" //nolint:gosec // fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_cw", Name: "w", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite, AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	do := func(method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	for _, c := range [][3]string{
		{"POST", cloudConnectionsPath, cloudOCIBody}, {"PUT", cloudConnectionsPath + "/a", `{}`},
		{"DELETE", cloudConnectionsPath + "/a", ""}, {"POST", cloudConnectionsPath + "/a/test", ""},
	} {
		if got := do(c[0], c[1], c[2]); got != http.StatusForbidden {
			t.Errorf("%s %s = %d, want 403", c[0], c[1], got)
		}
	}
	if got := do("GET", cloudConnectionsPath, ""); got != http.StatusOK {
		t.Errorf("list with read ability = %d", got)
	}
}

func TestCloudConnectionLifecycleProbeAndRedaction(t *testing.T) {
	probeErr := errors.New("provider body with canary-cloud-77")
	rt, db, fakes := newCloudRouter(t, true, cloudconn.ClassAuthFailed, probeErr)
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, b string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, b))
		return rec
	}
	var all strings.Builder
	rec := do("POST", cloudConnectionsPath, cloudOCIBody)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	all.WriteString(rec.Body.String())
	if rec := do("POST", cloudConnectionsPath, cloudOCIBody); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d", rec.Code)
	}
	if rec := do("POST", cloudConnectionsPath, `{"name":"Bad","provider":"oracle","auth_method":"oidc_federation"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid = %d", rec.Code)
	}
	all.WriteString(do("GET", cloudConnectionsPath, "").Body.String())
	rec = do("PUT", cloudConnectionsPath+"/oci-test", `{"team_id":"team1"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"team_id":"team1"`) {
		t.Fatalf("update = %d %s", rec.Code, rec.Body.String())
	}
	all.WriteString(rec.Body.String())
	// secret_ref with no resolver registered: classified as unresolved, the provider is never reached.
	rec = do("POST", cloudConnectionsPath+"/oci-test/test", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), cloudconn.ClassCredentialUnresolved) {
		t.Fatalf("test unresolved = %d %s", rec.Code, rec.Body.String())
	}
	if n := len(fakes["oracle"].Checked); n != 0 {
		t.Fatalf("provider reached %d times without a credential", n)
	}
	// oidc_federation needs no credential, so the fake provider is probed and its text never surfaces.
	if rec := do("PUT", cloudConnectionsPath+"/oci-test", `{"auth_method":"oidc_federation"}`); rec.Code != http.StatusOK {
		t.Fatalf("switch auth = %d %s", rec.Code, rec.Body.String())
	}
	rec = do("POST", cloudConnectionsPath+"/oci-test/test", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"class":"auth_failed"`) || len(fakes["oracle"].Checked) != 1 {
		t.Fatalf("probe = %d %s", rec.Code, rec.Body.String())
	}
	all.WriteString(rec.Body.String())
	if rec := do("DELETE", cloudConnectionsPath+"/oci-test", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	entries, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{Action: "cloud_connection"})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 6 {
		t.Fatalf("audit rows = %d, want created, 2 updated, 2 tested, deleted", len(entries))
	}
	var audit strings.Builder
	for _, e := range entries {
		audit.WriteString(e.Detail + e.Path + e.Action)
	}
	all.WriteString(audit.String())
	if strings.Contains(all.String(), "canary-cloud-77") {
		t.Fatal("provider output leaked into a response or an audit row")
	}
	if strings.Contains(audit.String(), "secret://") {
		t.Fatal("credential reference leaked into an audit row")
	}
}
