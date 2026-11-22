package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/quota"
	"github.com/GLINCKER/levelrail/internal/store"
)

func bulkCall(t *testing.T, rt *Router, cookie *http.Cookie, body string) (int, bulkAppsResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	rt.Handler().ServeHTTP(w, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/bulk", body))
	var resp bulkAppsResponse
	if w.Code == http.StatusMultiStatus {
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v body=%s", err, w.Body.String())
		}
	}
	return w.Code, resp
}

func bulkStatuses(resp bulkAppsResponse) map[string]string {
	out := map[string]string{}
	for _, r := range resp.Results {
		out[r.Name] = r.Status
	}
	return out
}

func TestBulkApps_PerAppAuthorization(t *testing.T) {
	actions := []struct{ action, value, ability string }{
		{bulkActionRestart, "", AbilityDeploy},
		{bulkActionStop, "", AbilityDeploy},
		{bulkActionStart, "", AbilityDeploy},
		{bulkActionAddTag, "team:core", AbilityWrite},
		{bulkActionRemoveTag, "team:core", AbilityWrite},
	}
	for _, tc := range actions {
		t.Run(tc.action, func(t *testing.T) {
			rt, db := newTestRouter(t)
			bootstrapTestAdmin(t, db)
			seedScheduledTaskApp(t, db, "secret-app")
			seedScheduledTaskApp(t, db, "open-app")
			user := storeUserWithAbilitiesForTest(t, db, "u@example.com", []string{AbilityRead, AbilityWrite, AbilityDeploy})
			attachTestPolicy(t, db, "deny-"+tc.action, "Deny", tc.ability, "app:secret-app", store.PrincipalTypeUser, user.ID)
			cookie := sessionCookieForTest(t, rt, user.ID)

			code, resp := bulkCall(t, rt, cookie, `{"action":"`+tc.action+`","names":["secret-app","open-app"],"value":"`+tc.value+`"}`)
			if code != http.StatusMultiStatus {
				t.Fatalf("status = %d, want 207", code)
			}
			got := bulkStatuses(resp)
			if got["secret-app"] != bulkStatusDenied {
				t.Errorf("secret-app = %q, want denied", got["secret-app"])
			}
			if got["open-app"] == bulkStatusDenied || got["open-app"] == bulkStatusError {
				t.Errorf("open-app = %q, want applied or skipped", got["open-app"])
			}
		})
	}
}

func TestBulkApps_DryRunChangesNothing(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	code, resp := bulkCall(t, rt, cookie, `{"action":"delete","names":["web"],"dry_run":true}`)
	if code != http.StatusMultiStatus || bulkStatuses(resp)["web"] != bulkStatusWouldRun {
		t.Fatalf("dry run = %d %+v", code, resp)
	}
	if _, err := db.GetDesiredService(t.Context(), "web"); err != nil {
		t.Errorf("app should still exist: %v", err)
	}
}

func TestBulkApps_DeleteNeedsConfirmationAndAudits(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "a")
	seedScheduledTaskApp(t, db, "b")

	if code, _ := bulkCall(t, rt, cookie, `{"action":"delete","names":["a","b"]}`); code != http.StatusConflict {
		t.Fatalf("no confirmation status = %d, want 409", code)
	}
	if code, _ := bulkCall(t, rt, cookie, `{"action":"delete","names":["a","b"],"confirm_names":["a"]}`); code != http.StatusConflict {
		t.Fatalf("partial confirmation status = %d, want 409", code)
	}
	code, resp := bulkCall(t, rt, cookie, `{"action":"delete","names":["a","b"],"confirm_names":["b","a"]}`)
	if code != http.StatusMultiStatus || resp.Counts[bulkStatusOK] != 2 {
		t.Fatalf("confirmed delete = %d %+v", code, resp)
	}
	entries, err := db.ListAuditEntries(t.Context(), 50, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	found := 0
	for _, e := range entries {
		if e.Method == http.MethodDelete && (e.Path == "/api/v1/apps/a" || e.Path == "/api/v1/apps/b") {
			found++
		}
	}
	if found != 2 {
		t.Errorf("per-app delete audit rows = %d, want 2", found)
	}
}

func TestBulkApps_TagCapAndValidation(t *testing.T) {
	t.Setenv("APP_MAX_TAGS_PER_APP", "1")
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	if code, _ := bulkCall(t, rt, cookie, `{"action":"add-tag","names":["web"],"value":"Bad Tag!"}`); code != http.StatusBadRequest {
		t.Errorf("invalid tag status = %d, want 400", code)
	}
	_, first := bulkCall(t, rt, cookie, `{"action":"add-tag","names":["web"],"value":"env:prod"}`)
	if bulkStatuses(first)["web"] != bulkStatusOK {
		t.Fatalf("first tag = %+v", first)
	}
	_, second := bulkCall(t, rt, cookie, `{"action":"add-tag","names":["web"],"value":"team:x"}`)
	if bulkStatuses(second)["web"] != bulkStatusSkipped || !strings.Contains(second.Results[0].Message, "maximum") {
		t.Errorf("over cap = %+v", second)
	}
	_, again := bulkCall(t, rt, cookie, `{"action":"add-tag","names":["web"],"value":"env:prod"}`)
	if bulkStatuses(again)["web"] != bulkStatusOK {
		t.Errorf("re-attaching existing tag = %+v, want ok", again)
	}
}

func TestBulkApps_SelectByTagAndMissingApp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	seedScheduledTaskApp(t, db, "worker")
	bulkCall(t, rt, cookie, `{"action":"add-tag","names":["web"],"value":"tier:edge"}`)

	_, resp := bulkCall(t, rt, cookie, `{"action":"stop","tag":"tier:edge"}`)
	if got := bulkStatuses(resp); len(got) != 1 || got["web"] != bulkStatusOK {
		t.Errorf("tag selection = %+v", got)
	}
	_, resp = bulkCall(t, rt, cookie, `{"action":"start","names":["ghost"]}`)
	if bulkStatuses(resp)["ghost"] != bulkStatusNotFound {
		t.Errorf("missing app = %+v", resp)
	}
}

func TestBulkApps_WavesHaltOnFail(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, n := range []string{"a", "b", "c", "d"} {
		seedScheduledTaskApp(t, db, n)
	}
	_, resp := bulkCall(t, rt, cookie, `{"action":"stop","names":["a","b","c","d"],"batch_size":2,"pause_seconds":1,"halt_on_fail":true}`)
	if len(resp.Results) != 4 {
		t.Fatalf("results = %+v", resp.Results)
	}
	for _, res := range resp.Results {
		if res.Name == "" {
			t.Errorf("result without a name: %+v", res)
		}
	}
	if resp.Counts[bulkStatusOK] != 4 {
		t.Errorf("no failure, every wave should run: %+v", resp.Counts)
	}
	failed := []bulkAppResult{{Status: bulkStatusOK}, {Status: bulkStatusError}}
	if !bulkWaveFailed(failed) {
		t.Error("an error row should fail the wave")
	}
	rows := make([]bulkAppResult, 3)
	bulkMarkSkipped(rows, []string{"x", "y", "z"}, 1, "later")
	if rows[0].Name != "" || rows[1].Name != "y" || rows[2].Status != bulkStatusSkipped {
		t.Errorf("skip fill = %+v", rows)
	}
}

func TestBulkApps_ScaleAndRollback(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "a-app")
	seedScheduledTaskApp(t, db, "b-app")

	if code, _ := bulkCall(t, rt, cookie, `{"action":"scale","names":["a-app"]}`); code != http.StatusBadRequest {
		t.Fatalf("scale without a count = %d, want 400", code)
	}
	code, resp := bulkCall(t, rt, cookie, `{"action":"scale","names":["a-app","b-app"],"replicas":3}`)
	if code != http.StatusMultiStatus {
		t.Fatalf("scale status = %d, want 207", code)
	}
	for name, st := range bulkStatuses(resp) {
		if st != bulkStatusOK {
			t.Errorf("scale %s = %q, want ok", name, st)
		}
		svc, err := db.GetDesiredService(context.Background(), name)
		if err != nil || svc.Replicas != 3 {
			t.Errorf("%s replicas = %v err=%v, want 3", name, svc, err)
		}
	}
	_, resp = bulkCall(t, rt, cookie, `{"action":"scale","names":["a-app"],"replicas":3}`)
	if got := bulkStatuses(resp)["a-app"]; got != bulkStatusSkipped {
		t.Errorf("rescale to same count = %q, want skipped", got)
	}
	_, resp = bulkCall(t, rt, cookie, `{"action":"rollback","names":["a-app"]}`)
	if got := bulkStatuses(resp)["a-app"]; got != bulkStatusSkipped {
		t.Errorf("rollback without history = %q, want skipped", got)
	}
}

func TestBulkApps_SelectByProject(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	seedScheduledTaskApp(t, db, "worker")
	if err := db.SaveProject(context.Background(), store.Project{ID: "p1", Name: "shop", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}
	if err := db.UpdateServiceProject(context.Background(), "web", "p1"); err != nil {
		t.Fatalf("assign project: %v", err)
	}
	code, resp := bulkCall(t, rt, cookie, `{"action":"restart","project":"p1","dry_run":true}`)
	if got := bulkStatuses(resp); code != http.StatusMultiStatus || len(got) != 1 || got["web"] != bulkStatusWouldRun {
		t.Errorf("project selection = %d %+v", code, got)
	}
}

func TestBulkApps_SetMemoryCPUEnvDryRunAndQuota(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	seedScheduledTaskApp(t, db, "a-app")
	seedScheduledTaskApp(t, db, "b-app")
	const proj = "proj_bulk_res"
	if err := db.SaveProject(ctx, store.Project{ID: proj, Name: "bulk", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"a-app", "b-app"} {
		if err := db.UpdateServiceProject(ctx, n, proj); err != nil {
			t.Fatal(err)
		}
	}

	if code, _ := bulkCall(t, rt, cookie, `{"action":"set-memory","names":["a-app"]}`); code != http.StatusBadRequest {
		t.Fatalf("set-memory without a value = %d, want 400", code)
	}
	if code, _ := bulkCall(t, rt, cookie, `{"action":"set-env","names":["a-app"],"env_key":"1BAD","env_value":"x"}`); code != http.StatusBadRequest {
		t.Fatalf("set-env with a bad key = %d, want 400", code)
	}

	code, resp := bulkCall(t, rt, cookie, `{"action":"set-memory","names":["a-app","b-app"],"memory_bytes":536870912,"dry_run":true}`)
	if code != http.StatusMultiStatus || len(resp.Results) != 2 {
		t.Fatalf("dry run = %d %+v", code, resp)
	}
	for _, r := range resp.Results {
		if r.Status != bulkStatusWouldRun || r.Before != "unlimited" || r.After != "512 MiB" {
			t.Errorf("dry run %s = %+v, want would_apply unlimited to 512 MiB", r.Name, r)
		}
	}
	if svc, _ := db.GetDesiredService(ctx, "a-app"); svc.Resources != nil && svc.Resources.MemoryBytes != 0 {
		t.Errorf("dry run changed memory to %+v", svc.Resources)
	}

	if _, resp = bulkCall(t, rt, cookie, `{"action":"set-memory","names":["a-app","b-app"],"memory_bytes":536870912}`); resp.Counts[bulkStatusOK] != 2 {
		t.Fatalf("apply = %+v", resp)
	}
	if err := db.SetQuota(ctx, quota.ScopeProject, proj, quota.Amounts{MemoryBytes: 1100 << 20}); err != nil {
		t.Fatal(err)
	}
	_, resp = bulkCall(t, rt, cookie, `{"action":"set-memory","names":["a-app"],"memory_bytes":1073741824,"dry_run":true}`)
	if got := bulkStatuses(resp)["a-app"]; got != bulkStatusSkipped || !strings.Contains(resp.Results[0].Message, "memory quota") {
		t.Errorf("raise over quota = %+v, want skipped naming the memory quota", resp.Results)
	}
	_, resp = bulkCall(t, rt, cookie, `{"action":"set-memory","names":["a-app"],"memory_bytes":268435456}`)
	if got := bulkStatuses(resp)["a-app"]; got != bulkStatusOK {
		t.Errorf("lowering under quota = %q, want ok", got)
	}

	_, resp = bulkCall(t, rt, cookie, `{"action":"set-cpu","names":["b-app"],"nano_cpus":500000000}`)
	if bulkStatuses(resp)["b-app"] != bulkStatusOK {
		t.Fatalf("set-cpu = %+v", resp)
	}
	_, resp = bulkCall(t, rt, cookie, `{"action":"set-env","names":["b-app"],"env_key":"LOG_LEVEL","env_value":"debug"}`)
	if bulkStatuses(resp)["b-app"] != bulkStatusOK || resp.Results[0].Before != "(unset)" || resp.Results[0].After != "debug" {
		t.Fatalf("set-env = %+v", resp)
	}
	_, resp = bulkCall(t, rt, cookie, `{"action":"set-env","names":["b-app"],"env_key":"API_TOKEN","env_value":"s3cret-value","dry_run":true}`)
	if r := resp.Results[0]; r.After != "(hidden)" || strings.Contains(r.Message, "s3cret-value") {
		t.Errorf("secret-looking env result = %+v, want a hidden value", r)
	}
	svc, err := db.GetDesiredService(ctx, "b-app")
	if err != nil || svc.Env["LOG_LEVEL"] != "debug" || svc.Resources.NanoCPUs != 500000000 || svc.Resources.MemoryBytes != 512<<20 {
		t.Errorf("b-app = %+v err=%v", svc, err)
	}
}
