package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cleanup"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

type apiFakeTarget struct{ meta cleanup.Meta }

func (f apiFakeTarget) Meta() cleanup.Meta                           { return f.meta }
func (apiFakeTarget) Measure(context.Context) (cleanup.Usage, error) { return cleanup.Usage{}, nil }
func (apiFakeTarget) Preview(context.Context, int) (cleanup.Impact, error) {
	return cleanup.Impact{}, nil
}
func (apiFakeTarget) Run(context.Context, int, cleanup.RunOptions) cleanup.Result {
	return cleanup.Result{}
}

var registeredCleanupTestSettings sync.Map

func newCleanupTestRouter(t *testing.T) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouter(t)
	reg := cleanup.NewRegistry(func(d runtimesettings.Definition) {
		if _, seen := registeredCleanupTestSettings.LoadOrStore(d.Key, true); !seen {
			runtimesettings.Register(d)
		}
	})
	set := rt.RuntimeSettings()
	cleanup.RegisterBuiltins(reg, cleanup.BuiltinDeps{Store: db, Tokens: db, Janitor: db, Settings: set})
	reg.Register(apiFakeTarget{meta: cleanup.Meta{ID: "apitest.fresh", Category: cleanup.CategoryOther, Title: "Fresh", PolicySetting: runtimesettings.KeyEmailLogDays, Origin: cleanup.OriginNew}})
	rt.SetCleanup(cleanup.New(cleanup.Config{Registry: reg, Store: db, Settings: set, Audit: db, Limits: cleanup.LimitsFromEnv(func(string) (string, bool) { return "", false })}))
	return rt, db
}

func cleanupDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestCleanupAuthz(t *testing.T) {
	rt, db := newCleanupTestRouter(t)
	root := loginTestSession(t, rt, db)
	reader := sessionCookieForTest(t, rt, storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead}).ID)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/cleanup/messaging.email_log/preview", `{}`},
		{http.MethodPost, "/api/v1/cleanup/messaging.email_log/run", `{"dry_run":true}`},
		{http.MethodPut, "/api/v1/cleanup/messaging.email_log/mode", `{"mode":"off"}`},
	} {
		if rec := cleanupDo(t, rt, reader, tc.method, tc.path, tc.body); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as non-root = %d, want 403", tc.method, tc.path, rec.Code)
		}
	}
	for _, p := range []string{"/api/v1/cleanup", "/api/v1/cleanup/runs"} {
		if rec := cleanupDo(t, rt, reader, http.MethodGet, p, ""); rec.Code != http.StatusOK {
			t.Errorf("GET %s as reader = %d, want 200", p, rec.Code)
		}
	}
	if rec := cleanupDo(t, rt, root, http.MethodPost, "/api/v1/cleanup/messaging.email_log/preview", `{}`); rec.Code != http.StatusOK {
		t.Errorf("root preview = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCleanupOverviewShape(t *testing.T) {
	rt, db := newCleanupTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := cleanupDo(t, rt, cookie, http.MethodGet, "/api/v1/cleanup", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Targets []map[string]any `json:"targets"`
		Sched   map[string]any   `json:"scheduler"`
	}
	raw := rec.Body.Bytes()
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body.Sched["enabled"].(bool); !ok || body.Sched["interval"] == "" {
		t.Fatalf("scheduler = %v", body.Sched)
	}
	byID := map[string]map[string]any{}
	for _, tg := range body.Targets {
		byID[tg["id"].(string)] = tg
		for _, k := range []string{"id", "category", "title", "description", "policy_key", "days", "mode", "origin", "destructive", "floor", "usage", "last_run", "next_run", "read_through"} {
			if _, ok := tg[k]; !ok {
				t.Errorf("%v lacks %q", tg["id"], k)
			}
		}
	}
	audit := byID["audit.log"]
	if audit == nil || audit["policy_key"] != runtimesettings.KeyAuditLogDays || audit["floor"].(float64) != 90 || audit["mode"] != "on" || audit["days"].(float64) != 90 || audit["destructive"] != true {
		t.Fatalf("audit target = %v", audit)
	}
	if audit["last_run"] != nil || audit["read_through"] != false {
		t.Fatalf("audit last_run/read_through = %v / %v", audit["last_run"], audit["read_through"])
	}
	if fresh := byID["apitest.fresh"]; fresh == nil || fresh["mode"] != "preview" {
		t.Fatalf("a new target must start in preview: %v", fresh)
	}
	if audit["source"] != "default" {
		t.Fatalf("audit source = %v, want default", audit["source"])
	}
	tok := byID["auth.api_tokens"]
	if tok == nil {
		t.Fatal("no token target")
	}
	if u, _ := tok["usage"].(map[string]any); u == nil || u["bytes"] != nil {
		t.Fatalf("token usage = %v, want bytes null", tok["usage"])
	}
	jan := byID["docker.images"]
	if jan == nil || jan["read_through"] != true || jan["policy_key"] != "" || jan["mode"] != "preview" {
		t.Fatalf("janitor target = %v", jan)
	}
}

func TestCleanupRunPreviewHistoryAndCSV(t *testing.T) {
	rt, db := newCleanupTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, age := range []time.Duration{90 * 24 * time.Hour, 60 * 24 * time.Hour, time.Hour} {
		err := db.InsertEmailDelivery(ctx, store.EmailDelivery{CreatedAt: time.Now().Add(-age), Provider: "smtp", Kind: "x", Status: "sent"}, "a@b.c", false)
		if err != nil {
			t.Fatal(err)
		}
	}
	type resp struct {
		Rows  int64  `json:"rows"`
		Bytes int64  `json:"bytes"`
		More  bool   `json:"more_remaining"`
		Error string `json:"error"`
		Dry   bool   `json:"dry_run"`
	}
	call := func(path, body string) resp {
		rec := cleanupDo(t, rt, cookie, http.MethodPost, path, body)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d: %s", path, rec.Code, rec.Body.String())
		}
		var r resp
		if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	if p := call("/api/v1/cleanup/messaging.email_log/preview", `{"days":30}`); p.Rows != 2 || p.Error != "" || !p.Dry {
		t.Fatalf("preview = %+v", p)
	}
	if d := call("/api/v1/cleanup/messaging.email_log/run", `{"days":30,"dry_run":true}`); d.Rows != 2 || !d.Dry {
		t.Fatalf("dry run = %+v", d)
	}
	if n, _ := db.CleanupStats(ctx, store.CleanupEmailDeliveries); n.Rows != 3 {
		t.Fatalf("a dry run deleted rows: %d left", n.Rows)
	}
	if r := call("/api/v1/cleanup/messaging.email_log/run", `{"days":30}`); r.Rows != 2 || r.More || r.Dry {
		t.Fatalf("run = %+v", r)
	}
	if n, _ := db.CleanupStats(ctx, store.CleanupEmailDeliveries); n.Rows != 1 {
		t.Fatalf("rows left = %d", n.Rows)
	}

	rec := cleanupDo(t, rt, cookie, http.MethodGet, "/api/v1/cleanup/runs?target=messaging.email_log&limit=1", "")
	var page struct {
		Runs []map[string]any `json:"runs"`
		Next string           `json:"next_cursor"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Runs) != 1 || page.Next == "" {
		t.Fatalf("page 1 = %s (%v)", rec.Body.String(), err)
	}
	if id, ok := page.Runs[0]["id"].(float64); !ok || id < 1 || page.Runs[0]["dry_run"] != false || page.Runs[0]["trigger"] != "api" {
		t.Fatalf("run = %v", page.Runs[0])
	}
	rec = cleanupDo(t, rt, cookie, http.MethodGet, "/api/v1/cleanup/runs?target=messaging.email_log&limit=1&cursor="+page.Next, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &page)
	if len(page.Runs) != 1 || page.Runs[0]["dry_run"] != true || page.Next != "" {
		t.Fatalf("page 2 = %s", rec.Body.String())
	}
	rec = cleanupDo(t, rt, cookie, http.MethodGet, "/api/v1/cleanup/runs?dry_run=true&status=ok&since=2020-01-01&until=2999-01-01", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Runs) != 1 {
		t.Fatalf("filtered = %s", rec.Body.String())
	}

	rec = cleanupDo(t, rt, cookie, http.MethodGet, "/api/v1/cleanup/runs?format=csv", "")
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("content type = %q", ct)
	}
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if len(lines) != 3 || lines[0] != strings.Join(cleanup.RunsCSVHeader, ",") {
		t.Fatalf("csv = %q", rec.Body.String())
	}

	var audits int
	rows, err := db.ListAuditEntries(ctx, 100, nil, store.AuditEntryFilter{})
	if err == nil {
		for _, e := range rows {
			if e.Action == cleanup.AuditActionRun {
				audits++
			}
		}
	}
	if audits != 2 {
		t.Fatalf("cleanup_run audit rows = %d (err %v), want one per run including the dry run", audits, err)
	}
	for _, bad := range []string{"since=yesterday", "status=maybe", "format=xml", "cursor=-1", "dry_run=perhaps", "limit=0"} {
		if rec := cleanupDo(t, rt, cookie, http.MethodGet, "/api/v1/cleanup/runs?"+bad, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, rec.Code)
		}
	}
}

func TestCleanupErrors(t *testing.T) {
	rt, db := newCleanupTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	if rec := cleanupDo(t, rt, cookie, http.MethodPost, "/api/v1/cleanup/nope/run", `{}`); rec.Code != http.StatusNotFound || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Errorf("unknown target = %d %s", rec.Code, rec.Body.String())
	}
	rec := cleanupDo(t, rt, cookie, http.MethodPost, "/api/v1/cleanup/audit.log/run", `{"days":30}`)
	var fe runtimeSettingError
	if rec.Code != http.StatusBadRequest || json.Unmarshal(rec.Body.Bytes(), &fe) != nil || fe.Field != runtimesettings.KeyAuditLogDays {
		t.Errorf("floor violation = %d %s, want 400 naming the policy key", rec.Code, rec.Body.String())
	}
	if rec := cleanupDo(t, rt, cookie, http.MethodPost, "/api/v1/cleanup/docker.images/run", `{}`); rec.Code != http.StatusConflict {
		t.Errorf("read-through run = %d, want 409", rec.Code)
	}
	if rec := cleanupDo(t, rt, cookie, http.MethodPut, "/api/v1/cleanup/docker.images/mode", `{"mode":"off"}`); rec.Code != http.StatusConflict {
		t.Errorf("read-through mode = %d, want 409", rec.Code)
	}
	if rec := cleanupDo(t, rt, cookie, http.MethodPost, "/api/v1/cleanup/messaging.email_log/run", `{"days":-4}`); rec.Code != http.StatusBadRequest {
		t.Errorf("negative days = %d, want 400", rec.Code)
	}
}

func TestCleanupModeChangeIsAuditedAndReturnsTarget(t *testing.T) {
	rt, db := newCleanupTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := cleanupDo(t, rt, cookie, http.MethodPut, "/api/v1/cleanup/apitest.fresh/mode", `{"mode":"on"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var view map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || view["id"] != "apitest.fresh" || view["mode"] != "on" {
		t.Fatalf("response = %s", rec.Body.String())
	}
	rows, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range rows {
		found = found || (e.Action == AuditActionSettingsChange && strings.Contains(e.Detail, "cleanup.apitest.fresh.mode"))
	}
	if !found {
		t.Fatal("the mode change wrote no settings_change audit row")
	}
	if rec := cleanupDo(t, rt, cookie, http.MethodPut, "/api/v1/cleanup/apitest.fresh/mode", `{"mode":"sometimes"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("bad mode = %d", rec.Code)
	}
	if rec := cleanupDo(t, rt, cookie, http.MethodPut, "/api/v1/cleanup/apitest.fresh/mode", `{"mode":"preview"}`); rec.Code != http.StatusOK {
		t.Fatalf("back to preview = %d", rec.Code)
	}
}

func TestCleanupPreviewReviewItem(t *testing.T) {
	rt, db := newCleanupTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := cleanupDo(t, rt, cookie, http.MethodGet, "/api/v1/settings/runtime/onboarding", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("onboarding = %d: %s", rec.Code, rec.Body.String())
	}
	var body runtimeOnboarding
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, it := range body.Items {
		if it.Key == CleanupPreviewKey {
			if string(it.Value) != "6" || !it.Onboarding {
				t.Fatalf("review item = %+v, want 6: one managed target plus the five janitor stages in dry run", it)
			}
			return
		}
	}
	t.Fatalf("no %s item in %+v", CleanupPreviewKey, body.Items)
}

func TestRetentionSettingsExposeAllowedValues(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rec := cleanupDo(t, rt, cookie, http.MethodGet, "/api/v1/settings/runtime/"+runtimesettings.KeyEmailLogDays, "")
	var got struct {
		Allowed []int `json:"allowed_values"`
		Value   int   `json:"value"`
		Env     string
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got.Allowed) != 6 || got.Allowed[0] != 0 || got.Value != 30 {
		t.Fatalf("setting = %s (%v)", rec.Body.String(), err)
	}
}
