package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbupgrade"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newUpgradeTestRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	cat, err := dbupgrade.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	m := &dbupgrade.Manager{
		Store: db, Advisor: dbupgrade.Advisor{Catalog: cat, EOLWarn: dbupgrade.DefaultEOLWarn},
		Runner: &dbupgrade.Runner{Store: db, Catalog: cat},
		Now:    func() time.Time { return time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC) },
	}
	rt := NewRouter(discardLogger(), testBrand(), db, WithDatabaseUpgrader(m))
	cookie := loginTestSession(t, rt, db)
	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"postgres","version":"16.9"}`)
	return rt, db, cookie
}

func TestDatabaseUpgradeRoutes(t *testing.T) {
	rt, db, cookie := newUpgradeTestRouter(t)
	ctx := context.Background()
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	rec := do(http.MethodGet, "/api/v1/databases/main/upgrades", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get upgrades: %d %s", rec.Code, rec.Body.String())
	}
	var got dbUpgradesResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Policy.Inherited || got.Policy.AutoUpgrade != store.DBAutoUpgradeOff || !got.Advice.HasSecurityUpdate() || len(got.Blockers) == 0 {
		t.Errorf("overview = %+v", got)
	}
	if rec := do(http.MethodGet, "/api/v1/databases/missing/upgrades", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing database: %d", rec.Code)
	}

	policyTests := []struct {
		name, body string
		want       int
	}{
		{"major refused", `{"auto_upgrade":"major"}`, http.StatusBadRequest},
		{"backup off refused", `{"auto_upgrade":"patch","window_cron":"0 3 * * *","window_duration_seconds":3600,"backup_before":false}`, http.StatusBadRequest},
		{"window required", `{"auto_upgrade":"patch"}`, http.StatusBadRequest},
		{"bad timezone", `{"auto_upgrade":"patch","window_cron":"0 3 * * *","window_duration_seconds":3600,"window_timezone":"Nowhere/Here"}`, http.StatusBadRequest},
		{"bad json", `{`, http.StatusBadRequest},
		{"valid", `{"auto_upgrade":"patch","window_cron":"0 3 * * *","window_duration_seconds":3600,"window_timezone":"Europe/Berlin"}`, http.StatusOK},
	}
	for _, tt := range policyTests {
		if rec := do(http.MethodPut, "/api/v1/databases/main/upgrade-policy", tt.body); rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d (%s)", tt.name, rec.Code, tt.want, rec.Body.String())
		}
	}
	p, found, err := db.GetDBUpgradePolicy(ctx, "main")
	if err != nil || !found || p.AutoUpgrade != store.DBAutoUpgradePatch || !p.VerifyAfter || !p.RevertOnFailure {
		t.Errorf("saved policy = %+v, %v, %v", p, found, err)
	}
	if rec := do(http.MethodPut, "/api/v1/databases/main/upgrade-policy", `{"inherit":true}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"inherited":true`) {
		t.Errorf("inherit: %d %s", rec.Code, rec.Body.String())
	}

	nowTests := []struct {
		name, body string
		want       int
	}{
		{"confirm required", `{"version":"16.11"}`, http.StatusBadRequest},
		{"bad tag", `{"version":"../x","confirm":"main"}`, http.StatusBadRequest},
		{"no backup target", `{"version":"16.11","confirm":"main"}`, http.StatusConflict},
	}
	for _, tt := range nowTests {
		if rec := do(http.MethodPost, "/api/v1/databases/main/upgrade-now", tt.body); rec.Code != tt.want {
			t.Errorf("%s: status %d, want %d (%s)", tt.name, rec.Code, tt.want, rec.Body.String())
		}
	}

	if err := db.SaveBackupTarget(ctx, store.BackupTarget{ID: "tgt", Name: "s3", Provider: "aws", Region: "us-east-1", Bucket: "b", CreatedAt: "2026-10-10T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDatabaseBackupSchedule(ctx, "main", "tgt", "", 0, 0); err != nil {
		t.Fatal(err)
	}
	if rec := do(http.MethodPost, "/api/v1/databases/main/upgrade-now", `{"version":"17.7","confirm":"main"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("major via upgrade-now: %d %s", rec.Code, rec.Body.String())
	}
	rec = do(http.MethodPost, "/api/v1/databases/main/upgrade-now", `{"version":"16.11","confirm":"main"}`)
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), `"state":"pending"`) {
		t.Fatalf("upgrade now: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodPost, "/api/v1/databases/main/upgrade-now", `{"version":"16.11","confirm":"main"}`); rec.Code != http.StatusConflict {
		t.Errorf("second upgrade while one runs: %d", rec.Code)
	}

	sum := do(http.MethodGet, "/api/v1/databases/upgrade-summary", "")
	if sum.Code != http.StatusOK || !strings.Contains(sum.Body.String(), `"security_count":1`) || !strings.Contains(sum.Body.String(), `"active_state":"pending"`) {
		t.Errorf("summary: %d %s", sum.Code, sum.Body.String())
	}
	feed := do(http.MethodGet, "/api/v1/attention/feed", "")
	if !strings.Contains(feed.Body.String(), `"kind":"db_security_updates"`) {
		t.Errorf("attention feed lacks the security item: %s", feed.Body.String())
	}

	if rec := do(http.MethodPut, "/api/v1/settings/database-upgrades", `{"auto_upgrade":"minor","window_cron":"0 2 * * 6","window_duration_seconds":7200}`); rec.Code != http.StatusOK {
		t.Errorf("platform default: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/api/v1/settings/database-upgrades", ""); !strings.Contains(rec.Body.String(), `"auto_upgrade":"minor"`) {
		t.Errorf("platform default read back: %s", rec.Body.String())
	}
}

func TestDatabaseUpgradeRoutesAbilities(t *testing.T) {
	rt, db, _ := newUpgradeTestRouter(t)
	tok := seedMatrixToken(t, db, "ro-upgrades", []string{AbilityRead})
	call := func(method, path, body string) int {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := call(http.MethodGet, "/api/v1/databases/main/upgrades", ""); code != http.StatusOK {
		t.Errorf("read token GET upgrades = %d, want 200", code)
	}
	for _, c := range []struct{ method, path, body string }{
		{http.MethodPut, "/api/v1/databases/main/upgrade-policy", `{"inherit":true}`},
		{http.MethodPost, "/api/v1/databases/main/upgrade-now", `{"version":"16.11","confirm":"main"}`},
		{http.MethodPut, "/api/v1/settings/database-upgrades", `{"auto_upgrade":"off"}`},
	} {
		if code := call(c.method, c.path, c.body); code != http.StatusForbidden {
			t.Errorf("read token %s %s = %d, want 403", c.method, c.path, code)
		}
	}
}

func TestDatabaseUpgradeRoutesNotConfigured(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/databases/upgrade-summary", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("unconfigured summary = %d, want 501", rec.Code)
	}
}
