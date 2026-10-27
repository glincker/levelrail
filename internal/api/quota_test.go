package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/quota"
	"github.com/GLINCKER/levelrail/internal/store"
)

func quotaFixture(t *testing.T) (*Router, *store.DB, *http.Cookie, string) {
	t.Helper()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	const projectID = "proj_quota"
	if err := db.SaveProject(context.Background(), store.Project{ID: projectID, Name: "shop", CreatedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	return rt, db, cookie, projectID
}

func quotaDo(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec.Code, rec.Body.String()
}

func createQuotaApp(t *testing.T, rt *Router, cookie *http.Cookie, name, projectID, extra string) (int, string) {
	t.Helper()
	body := fmt.Sprintf(`{"name":%q,"image":"levelrail/web:1","port":3000,"project_id":%q%s}`, name, projectID, extra)
	return quotaDo(t, rt, cookie, http.MethodPost, "/api/v1/apps", body)
}

func TestQuota_AppCountLimitNamesLimitAndUsage(t *testing.T) {
	rt, db, cookie, proj := quotaFixture(t)
	if err := db.SetQuota(context.Background(), quota.ScopeProject, proj, quota.Amounts{Apps: 2}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a1", "a2"} {
		if code, body := createQuotaApp(t, rt, cookie, name, proj, ""); code != http.StatusCreated {
			t.Fatalf("create %s = %d %s, want 201 (exactly at the limit is allowed)", name, code, body)
		}
	}
	code, body := createQuotaApp(t, rt, cookie, "a3", proj, "")
	if code != http.StatusConflict {
		t.Fatalf("third create = %d %s, want 409", code, body)
	}
	var e codedError
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatal(err)
	}
	if e.Code != quotaExceededCode || !strings.Contains(e.Error, "apps quota") || !strings.Contains(e.Error, "2 of 2") {
		t.Errorf("error = %+v, want code and a message naming the limit and usage", e)
	}
	if _, err := db.GetDesiredService(context.Background(), "a3"); err == nil {
		t.Error("a refused create left a desired service behind")
	}
}

func TestQuota_RefusalReleasesTheGuard(t *testing.T) {
	rt, db, cookie, proj := quotaFixture(t)
	if err := db.SetQuota(context.Background(), quota.ScopeProject, proj, quota.Amounts{Apps: 1, MemoryBytes: 100 << 20}); err != nil {
		t.Fatal(err)
	}
	big := `,"resources":{"memory_bytes":209715200}`
	if code, _ := createQuotaApp(t, rt, cookie, "big", proj, big); code != http.StatusConflict {
		t.Fatalf("oversize create = %d, want 409", code)
	}
	done := make(chan int, 1)
	go func() {
		code, _ := createQuotaApp(t, rt, cookie, "small", proj, "")
		done <- code
	}()
	if code := <-done; code != http.StatusCreated {
		t.Fatalf("create after a refusal = %d, want 201 (guard must be released)", code)
	}
}

func TestQuota_ConcurrentCreatesNeverExceedLimit(t *testing.T) {
	rt, db, cookie, proj := quotaFixture(t)
	const limit, attempts = 3, 12
	if err := db.SetQuota(context.Background(), quota.ScopeProject, proj, quota.Amounts{Apps: limit}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	codes := make(chan int, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, _ := createQuotaApp(t, rt, cookie, fmt.Sprintf("c%d", i), proj, "")
			codes <- code
		}(i)
	}
	wg.Wait()
	close(codes)
	created := 0
	for c := range codes {
		if c == http.StatusCreated {
			created++
		}
	}
	used, err := db.QuotaUsage(context.Background(), quota.ScopeProject, proj)
	if err != nil {
		t.Fatal(err)
	}
	if created != limit || used.Apps != limit {
		t.Fatalf("created %d, stored %d, want exactly %d", created, used.Apps, limit)
	}
}

func TestQuota_TeamDefaultsFromEnvAndMemoryResize(t *testing.T) {
	t.Setenv(envQuotaDefaultMemoryMB, "512")
	rt, _, cookie, proj := quotaFixture(t)
	if code, body := createQuotaApp(t, rt, cookie, "web", proj, `,"resources":{"memory_bytes":268435456}`); code != http.StatusCreated {
		t.Fatalf("create = %d %s", code, body)
	}
	grow := `{"name":"web","image":"levelrail/web:1","port":3000,"resources":{"memory_bytes":805306368}}`
	code, body := quotaDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web", grow)
	if code != http.StatusConflict || !strings.Contains(body, "memory quota") {
		t.Fatalf("resize = %d %s, want 409 naming memory", code, body)
	}
	shrink := `{"name":"web","image":"levelrail/web:1","port":3000,"resources":{"memory_bytes":134217728}}`
	if code, body := quotaDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/web", shrink); code != http.StatusOK {
		t.Fatalf("shrink = %d %s, want 200", code, body)
	}
}

func TestQuota_MoveIntoFullProjectIsRefused(t *testing.T) {
	rt, db, cookie, proj := quotaFixture(t)
	ctx := context.Background()
	if err := db.SetQuota(ctx, quota.ScopeProject, proj, quota.Amounts{Apps: 1}); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"in", "out"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: n, Image: "nginx:1", Port: 80}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpdateServiceProject(ctx, "in", proj); err != nil {
		t.Fatal(err)
	}
	code, body := quotaDo(t, rt, cookie, http.MethodPut, "/api/v1/apps/out/project", fmt.Sprintf(`{"project_id":%q}`, proj))
	if code != http.StatusConflict {
		t.Fatalf("move = %d %s, want 409", code, body)
	}
}

func TestQuota_SetValidatesAndAdminSeesUsage(t *testing.T) {
	rt, db, cookie, proj := quotaFixture(t)
	if err := db.SaveDesiredDatabase(context.Background(), store.DesiredDatabase{Name: "pg", Engine: store.EnginePostgres, Version: "16"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateDatabaseProject(context.Background(), "pg", proj); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/projects/" + proj + "/quota"
	if code, _ := quotaDo(t, rt, cookie, http.MethodPut, path, `{"max_apps":-1}`); code != http.StatusBadRequest {
		t.Errorf("negative = %d, want 400", code)
	}
	if code, body := quotaDo(t, rt, cookie, http.MethodPut, path, `{"max_databases":4,"max_memory_mb":2048,"max_cpu":1.5}`); code != http.StatusOK {
		t.Fatalf("set = %d %s", code, body)
	}
	_, body := quotaDo(t, rt, cookie, http.MethodGet, path, "")
	var got quotaResource
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if got.Limits.MaxDatabases != 4 || got.Limits.MaxMemoryMB != 2048 || got.Limits.MaxCPU != 1.5 || got.Usage.Databases != 1 || !got.CanManage || got.Name != "shop" {
		t.Errorf("quota = %+v", got)
	}
	if code, _ := quotaDo(t, rt, cookie, http.MethodDelete, path, ""); code != http.StatusOK {
		t.Errorf("delete = %d", code)
	}
	if after, _ := db.GetQuota(context.Background(), quota.ScopeProject, proj); !after.Unlimited() {
		t.Errorf("limits after delete = %+v, want unlimited", after)
	}
}

func TestQuota_MembersSeeUsageButCannotChange(t *testing.T) {
	f := newTeamFixture(t)
	path := "/api/v1/teams/" + teamA + "/quota"
	if code, body := teamDo(t, f.rt, http.MethodPut, path, `{"max_apps":5}`, withCookie(f.cookies[store.TeamRoleAdmin])); code != http.StatusOK {
		t.Fatalf("admin set = %d %s", code, body)
	}
	if code, _ := teamDo(t, f.rt, http.MethodPut, path, `{"max_apps":9}`, withCookie(f.cookies[store.TeamRoleDeveloper])); code != http.StatusForbidden {
		t.Errorf("developer set = %d, want 403", code)
	}
	code, body := teamDo(t, f.rt, http.MethodGet, path, "", withCookie(f.cookies[store.TeamRoleViewer]))
	if code != http.StatusOK {
		t.Fatalf("viewer get = %d %s", code, body)
	}
	var got quotaResource
	_ = json.Unmarshal([]byte(body), &got)
	if got.Limits.MaxApps != 5 || got.CanManage || got.Usage.Apps == 0 {
		t.Errorf("viewer view = %+v, want limit 5, no manage, real usage", got)
	}
	if code, _ := teamDo(t, f.rt, http.MethodGet, "/api/v1/teams/"+teamB+"/quota", "", withCookie(f.cookies[store.TeamRoleViewer])); code == http.StatusOK {
		t.Error("a member read another team's quota")
	}
}

func TestQuota_AttentionItemCrossesWarnThreshold(t *testing.T) {
	rt, db, cookie, proj := quotaFixture(t)
	ctx := context.Background()
	if err := db.SetQuota(ctx, quota.ScopeProject, proj, quota.Amounts{Apps: 5}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if code, body := createQuotaApp(t, rt, cookie, fmt.Sprintf("w%d", i), proj, ""); code != http.StatusCreated {
			t.Fatalf("create = %d %s", code, body)
		}
	}
	_, body := quotaDo(t, rt, cookie, http.MethodGet, "/api/v1/attention/feed", "")
	var feed attentionFeedResponse
	if err := json.Unmarshal([]byte(body), &feed); err != nil {
		t.Fatal(err)
	}
	for _, it := range feed.Items {
		if it.Kind == attention.KindQuotaNearLimit && strings.Contains(it.Subject, proj) {
			return
		}
	}
	t.Fatalf("no quota item at 80%% usage: %s", body)
}

func TestQuota_StandingShmOnlyAppsCountTowardMemory(t *testing.T) {
	rt, db, cookie, proj := quotaFixture(t)
	ctx := context.Background()
	if err := db.SetQuota(ctx, quota.ScopeProject, proj, quota.Amounts{MemoryBytes: 2 << 30}); err != nil {
		t.Fatal(err)
	}
	shm := `,"resources":{"shm_size_bytes":1073741824}`
	for _, name := range []string{"s1", "s2"} {
		if code, body := createQuotaApp(t, rt, cookie, name, proj, shm); code != http.StatusCreated {
			t.Fatalf("create %s = %d %s, want 201", name, code, body)
		}
	}
	used, err := db.QuotaUsage(ctx, quota.ScopeProject, proj)
	if err != nil {
		t.Fatal(err)
	}
	if used.MemoryBytes != 2<<30 {
		t.Fatalf("standing memory = %d, want %d (shm counts when no memory limit is set)", used.MemoryBytes, int64(2<<30))
	}
	if code, body := createQuotaApp(t, rt, cookie, "s3", proj, shm); code != http.StatusConflict {
		t.Fatalf("third shm-only create = %d %s, want 409", code, body)
	}
}
