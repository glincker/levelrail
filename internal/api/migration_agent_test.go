package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/migrateagent"
	"github.com/GLINCKER/levelrail/internal/migrationplan"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeMigrationAgent struct {
	aud     *migrateagent.Auditor
	revoked []string
	scan    []migrateagent.StatDoc
	offline bool
	volumes string
}

func (f *fakeMigrationAgent) Create(context.Context, string, string) (migrateagent.Created, error) {
	return migrateagent.Created{ID: "ms_fake", Token: "mig_ms_fake." + strings.Repeat("S", 43), ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func (f *fakeMigrationAgent) List(context.Context) ([]migrateagent.SessionView, error) {
	return []migrateagent.SessionView{{ID: "ms_fake", State: "connected"}}, nil
}

func (f *fakeMigrationAgent) Get(_ context.Context, id string) (migrateagent.SessionView, migrateagent.InventoryDoc, migrateagent.ScanDoc, error) {
	if id != "ms_fake" {
		return migrateagent.SessionView{}, migrateagent.InventoryDoc{}, migrateagent.ScanDoc{}, store.ErrMigrationAgentNotFound
	}
	inv, _ := f.Discover(context.Background(), id)
	return migrateagent.SessionView{ID: id, State: "connected"}, inv, migrateagent.ScanDoc{}, nil
}

func (f *fakeMigrationAgent) Revoke(_ context.Context, id, _ string) error {
	f.revoked = append(f.revoked, id)
	return nil
}

func (f *fakeMigrationAgent) Discover(context.Context, string) (migrateagent.InventoryDoc, error) {
	if f.offline {
		return migrateagent.InventoryDoc{}, migrateagent.ErrOffline
	}
	return migrateagent.InventoryDoc{Containers: []migrateagent.ContainerDoc{
		{Name: "shop-web-1", Image: "web"}, {Name: "legacy-cron", Image: "alpine"}, {Name: "pg", Image: "postgres:16", DatabaseEngine: "postgres"},
	}}, nil
}

func (f *fakeMigrationAgent) Scan(context.Context, string, []string) (migrateagent.ScanDoc, error) {
	return migrateagent.ScanDoc{Stats: f.scan}, nil
}

func (f *fakeMigrationAgent) Auditor() *migrateagent.Auditor { return f.aud }
func (f *fakeMigrationAgent) Config() migrateagent.Config {
	dir := f.volumes
	if dir == "" {
		dir = "/nonexistent"
	}
	return migrateagent.Config{SessionMax: 72 * time.Hour, VolumesDir: dir}
}

func TestMigrationAgentVerifyAdvancesAndUndoes(t *testing.T) {
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	src := newImportSourceServer(t)
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithSecretSetter(&fakeSecretSetter{}), WithAgentCAFingerprint("abc123def456"))
	fake := &fakeMigrationAgent{aud: &migrateagent.Auditor{Saver: db}, scan: []migrateagent.StatDoc{{Target: "uploads", Files: 3, Bytes: 90}},
		volumes: volumesWithFiles(t, "app-shop-web-uploads", 3, 30)}
	rt.SetMigrationAgent(fake)
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

	verify := func(stats string) migrationVerifyResponse {
		t.Helper()
		rec := do(http.MethodPost, "/api/v1/migration/agent/sessions/ms_fake/verify", `{"app":"shop-web","target_stats":[`+stats+`]}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("verify: %d %s", rec.Code, rec.Body.String())
		}
		var out migrationVerifyResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	diff := verify(`{"name":"app-shop-web-uploads","files":2,"bytes":90}`)
	if diff.Advance.Advanced || diff.Comparisons[0].Status != migrateagent.StatusDiffers || diff.Checklist.Current != migrationplan.StepDataVerified {
		t.Fatalf("a diff must not advance: %+v", diff)
	}

	forged := verify(`{"name":"app-shop-web-uploads","files":3,"bytes":90}`)
	if forged.Advance.Advanced || forged.Comparisons[0].Status != migrateagent.StatusMatch ||
		forged.Comparisons[0].TargetFrom != migrateagent.TargetOperator || forged.Checklist.Current != migrationplan.StepDataVerified {
		t.Fatalf("caller-supplied numbers must never auto-complete the step: %+v", forged)
	}

	ok := verify(``)
	if !ok.Advance.Advanced || ok.Checklist.Current != migrationplan.StepShadowRunning || ok.Databases != 1 {
		t.Fatalf("zero diffs must advance: %+v", ok)
	}
	var st checklistStepView
	for _, s := range ok.Checklist.Steps {
		if s.ID == migrationplan.StepDataVerified {
			st = s
		}
	}
	if st.Status != "done" || st.Actor != migrateagent.Actor || len(st.Evidence) < 2 {
		t.Fatalf("step not attributed to the agent with evidence: %+v", st)
	}

	again := verify(``)
	if again.Advance.Advanced || again.Checklist.Current != migrationplan.StepShadowRunning {
		t.Fatalf("a second verify must not move later steps: %+v", again)
	}

	rec := do(http.MethodPost, "/api/v1/migration/checklists/shop-web/steps/data_verified", `{"action":"reset"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("undo: %d %s", rec.Code, rec.Body.String())
	}
	var cl checklistView
	_ = json.Unmarshal(rec.Body.Bytes(), &cl)
	if cl.Current != migrationplan.StepDataVerified {
		t.Fatalf("undo did not reopen data_verified: %+v", cl)
	}
	entries, err := db.ListAuditEntries(context.Background(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	overrides := 0
	for _, e := range entries {
		seen[e.Action] = true
		if e.Action == migrateagent.ActionVerified && strings.Contains(e.Detail, `"overrides":1`) {
			overrides++
		}
	}
	if overrides != 2 {
		t.Errorf("verify audits with overrides=1: got %d, want 2", overrides)
	}
	for _, want := range []string{migrateagent.ActionVerified, migrateagent.ActionStepAdvanced, migrateagent.ActionStepReopened} {
		if !seen[want] {
			t.Errorf("missing audit action %s", want)
		}
	}
}

func TestMigrationAgentRoutes(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db, WithAgentCAFingerprint("abc123def456"), WithDoctorAgentAdvertise("203.0.113.5", 9443))
	cookie := loginTestSession(t, rt, db)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	if rec := do(http.MethodGet, "/api/v1/migration/agent/sessions", ""); rec.Code != http.StatusNotImplemented {
		t.Fatalf("without the service: %d", rec.Code)
	}
	fake := &fakeMigrationAgent{aud: &migrateagent.Auditor{Saver: db}}
	rt.SetMigrationAgent(fake)

	rec := do(http.MethodPost, "/api/v1/migration/agent/sessions", `{"label":"old box"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created createMigrationAgentResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	for _, want := range []string{"APP_CONTROL_PLANE_ADDR=203.0.113.5:9443", "APP_CA_FINGERPRINT=abc123def456", "APP_MIGRATION_TOKEN=" + created.Token, " install"} {
		if !strings.Contains(created.Command, want) {
			t.Errorf("command is missing %q: %s", want, created.Command)
		}
	}
	if rec := do(http.MethodPost, "/api/v1/migration/agent/sessions", `{"label":"bad\nlabel"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("multi-line label: %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/migration/agent/sessions", `{"control_plane_addr":"x; rm -rf /"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("injected address: %d", rec.Code)
	}

	rec = do(http.MethodGet, "/api/v1/migration/agent/sessions/ms_fake", "")
	var detail migrationAgentDetail
	_ = json.Unmarshal(rec.Body.Bytes(), &detail)
	if rec.Code != http.StatusOK || len(detail.Inventory.Containers) != 3 || !detail.Inventory.Containers[1].Unlisted {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/api/v1/migration/agent/sessions/ms_other", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown session: %d", rec.Code)
	}
	fake.offline = true
	if rec := do(http.MethodPost, "/api/v1/migration/agent/sessions/ms_fake/discover", ""); rec.Code != http.StatusConflict {
		t.Fatalf("offline agent: %d", rec.Code)
	}
	if rec := do(http.MethodDelete, "/api/v1/migration/agent/sessions/ms_fake", ""); rec.Code != http.StatusNoContent || len(fake.revoked) != 1 {
		t.Fatalf("revoke: %d %v", rec.Code, fake.revoked)
	}
	if rec := do(http.MethodDelete, "/api/v1/migration/agent/sessions/ms_other", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("revoke unknown: %d", rec.Code)
	}
}

func TestMigrationAgentCommandNeedsPin(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(nil, testBrand(), db)
	rt.SetMigrationAgent(&fakeMigrationAgent{aud: &migrateagent.Auditor{Saver: db}})
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/migration/agent/sessions", `{}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("without a CA fingerprint: %d %s", rec.Code, rec.Body.String())
	}
}
