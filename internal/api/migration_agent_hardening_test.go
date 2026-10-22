package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/migrateagent"
	"github.com/GLINCKER/levelrail/internal/migrationplan"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

// volumesWithFiles lays out <dir>/<volume>/_data holding n files of size bytes each.
func volumesWithFiles(t *testing.T, volume string, n, size int) string {
	t.Helper()
	dir := t.TempDir()
	data := filepath.Join(dir, volume, "_data")
	if err := os.MkdirAll(data, 0o750); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(data, "f"+strconv.Itoa(i)), []byte(strings.Repeat("x", size)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestMigrationAgentVerifyEmptyVolumesNeedAcknowledgement(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		advanced bool
	}{
		{"no acknowledgement blocks", `{"app":"shop-web"}`, false},
		{"explicit false blocks", `{"app":"shop-web","ack_empty_volumes":false}`, false},
		{"acknowledgement advances", `{"app":"shop-web","ack_empty_volumes":true}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(platformimport.AllowLoopbackEnv, "true")
			src := newImportSourceServer(t)
			db := openTestDB(t)
			rt := NewRouter(nil, testBrand(), db, WithSecretSetter(&fakeSecretSetter{}), WithAgentCAFingerprint("abc123def456"))
			rt.SetMigrationAgent(&fakeMigrationAgent{aud: &migrateagent.Auditor{Saver: db}, scan: []migrateagent.StatDoc{{Target: "uploads"}},
				volumes: volumesWithFiles(t, "app-shop-web-uploads", 0, 0)})
			cookie := loginTestSession(t, rt, db)
			do := func(method, path, body string) *httptest.ResponseRecorder {
				rec := httptest.NewRecorder()
				rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
				return rec
			}
			base := strings.TrimSuffix(importBody(src.URL), "}")
			if rec := do(http.MethodPost, "/api/v1/migration/import", base+`,"only":["a1"],"overrides":{"a1":{"name":"shop-web"}}}`); rec.Code != http.StatusOK {
				t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
			}
			rec := do(http.MethodPost, "/api/v1/migration/agent/sessions/ms_fake/verify", tc.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("verify: %d %s", rec.Code, rec.Body.String())
			}
			var out migrationVerifyResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.Comparisons[0].Status != migrateagent.StatusMatch {
				t.Fatalf("an empty volume still compares as a match: %+v", out.Comparisons)
			}
			if out.Advance.Advanced != tc.advanced {
				t.Fatalf("advanced=%v reason=%q want %v", out.Advance.Advanced, out.Advance.Reason, tc.advanced)
			}
			wantStep := migrationplan.StepDataVerified
			if tc.advanced {
				wantStep = migrationplan.StepShadowRunning
			}
			if out.Checklist.Current != wantStep {
				t.Fatalf("current step %q want %q", out.Checklist.Current, wantStep)
			}
			entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
			if err != nil {
				t.Fatal(err)
			}
			ack := `"ack_empty_volumes":` + strconv.FormatBool(tc.advanced)
			found := false
			for _, e := range entries {
				found = found || (e.Action == migrateagent.ActionVerified && strings.Contains(e.Detail, ack) && strings.Contains(e.Detail, `"overrides":0`))
			}
			if !found {
				t.Fatalf("the verification audit must record %s and overrides", ack)
			}
		})
	}
}

func TestMigrationAgentSessionDetailNeedsSensitiveAbility(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db)
	rt.SetMigrationAgent(&fakeMigrationAgent{aud: &migrateagent.Auditor{Saver: db}})
	const plaintext = "ro-token" //nolint:gosec // fake fixture
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_ro", Name: "ro", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		path string
		want int
	}{
		{"detail is sensitive", "/api/v1/migration/agent/sessions/ms_fake", http.StatusForbidden},
		{"list stays readable", "/api/v1/migration/agent/sessions", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, tc.path, "", plaintext))
			if rec.Code != tc.want {
				t.Fatalf("status %d want %d", rec.Code, tc.want)
			}
		})
	}
}
