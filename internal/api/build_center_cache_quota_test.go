package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeQuotaCacher struct {
	fakeAppCacher
	reclaimed []string
	res       build.AppCacheReclaim
	err       error
	block     bool
}

func (f *fakeQuotaCacher) ReclaimAppCacheQuota(ctx context.Context, id string, _ []string, quota int64, _ int) (build.AppCacheReclaim, error) {
	f.reclaimed = append(f.reclaimed, id)
	if f.block {
		<-ctx.Done()
		return build.AppCacheReclaim{}, ctx.Err()
	}
	r := f.res
	r.QuotaBytes = quota
	return r, f.err
}

func newQuotaRouter(t *testing.T, fake *fakeQuotaCacher) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	appCacheReclaims = &cacheReclaimLog{running: map[string]bool{}}
	rt, db, cookie := newSafetyRouter(t, stubResolver{})
	WithAppBuildCache(fake)(rt)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APP_BUILD_CACHE_APP_QUOTA_GB", "1")
	return rt, db, cookie
}

func TestCacheQuotaPartialPruneIsReportedAndAudited(t *testing.T) {
	fake := &fakeQuotaCacher{res: build.AppCacheReclaim{FreedBytes: 7, Removed: 2}, err: errors.New("prune cut off")}
	rt, db, _ := newQuotaRouter(t, fake)
	rt.AfterBuildFinish("web")
	got := appCacheReclaims.newestFirst(func(string) bool { return true })
	if len(got) != 1 || got[0].FreedBytes != 7 || !got[0].Partial {
		t.Fatalf("report = %+v, want one partial entry", got)
	}
	all, err := db.ListAuditEntries(context.Background(), 100, nil, store.AuditEntryFilter{})
	var entries []store.AuditEntry
	for _, e := range all {
		if e.Action == AuditActionAppBuildCacheReclaim {
			entries = append(entries, e)
		}
	}
	if err != nil || len(entries) != 1 || !strings.Contains(entries[0].Detail, `"freed_bytes":7`) {
		t.Fatalf("audit = %+v %v", entries, err)
	}
}

func TestCacheQuotaTimeoutReleasesClaimAndGate(t *testing.T) {
	fake := &fakeQuotaCacher{block: true}
	rt, _, _ := newQuotaRouter(t, fake)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := rt.reclaimAppCache(ctx, "web", cacheReclaimTriggerAuto); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline", err)
	}
	if !appCacheReclaims.claim("web") {
		t.Fatal("claim must be released after a timeout")
	}
	appCacheReclaims.release("web")
	select {
	case cacheScanGate <- struct{}{}:
		<-cacheScanGate
	default:
		t.Fatal("scan gate must be free after a timeout")
	}
	cacheScanGate <- struct{}{}
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	_, err := rt.reclaimAppCache(ctx2, "web", cacheReclaimTriggerAuto)
	<-cacheScanGate
	if !errors.Is(err, context.DeadlineExceeded) || len(fake.reclaimed) != 1 {
		t.Fatalf("held gate must stop a second scan: %v %d", err, len(fake.reclaimed))
	}
}

func TestCacheQuotaSkipsQueuedBuild(t *testing.T) {
	fake := &fakeQuotaCacher{res: build.AppCacheReclaim{Removed: 1}}
	rt, db, _ := newQuotaRouter(t, fake)
	queued := store.DeployAttempt{ID: "q1", ServiceName: "web", Image: "web:2", Status: store.DeployAttemptStatusQueued, StartedAt: time.Now()}
	if err := db.SaveDeployAttempt(context.Background(), queued); err != nil {
		t.Fatal(err)
	}
	rt.AfterBuildFinish("web")
	if len(fake.reclaimed) != 0 {
		t.Fatal("a queued build must block the automatic reclaim")
	}
}

func TestCacheQuotaReclaimAndReport(t *testing.T) {
	appCacheReclaims = &cacheReclaimLog{running: map[string]bool{}}
	rt, db, cookie := newSafetyRouter(t, stubResolver{})
	fake := &fakeQuotaCacher{res: build.AppCacheReclaim{UsageBytes: 3 << 30, FreedBytes: 2 << 30, Removed: 4}}
	WithAppBuildCache(fake)(rt)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/apps/web/build-cache/reclaim"

	t.Setenv("APP_BUILD_CACHE_APP_QUOTA_GB", "0")
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, path, "")); rec.Code != http.StatusConflict || len(fake.reclaimed) != 0 {
		t.Fatalf("quota 0 must be off: %d", rec.Code)
	}
	rt.AfterBuildFinish("web")
	if len(fake.reclaimed) != 0 {
		t.Fatal("post build hook must do nothing with the quota off")
	}

	t.Setenv("APP_BUILD_CACHE_APP_QUOTA_GB", "1")
	attempt := store.DeployAttempt{ID: "att1", ServiceName: "web", Image: "web:1", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}
	if err := db.SaveDeployAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	if rec := serve(rt, authedRequest(t, cookie, http.MethodPost, path, "")); rec.Code != http.StatusConflict || len(fake.reclaimed) != 0 {
		t.Fatalf("running build must block the reclaim: %d", rec.Code)
	}
	if err := db.FinishDeployAttempt(ctx, "att1", store.DeployAttemptStatusSucceeded, time.Now(), ""); err != nil {
		t.Fatal(err)
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, path, ""))
	var got cacheReclaimEntry
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.FreedBytes != 2<<30 || got.QuotaBytes != 1<<30 || got.Trigger != cacheReclaimTriggerManual {
		t.Fatalf("reclaim = %d %s", rec.Code, rec.Body.String())
	}
	if len(fake.reclaimed) != 1 || fake.reclaimed[0] != "web" {
		t.Fatalf("reclaimed %v, want only web", fake.reclaimed)
	}

	rec = serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/builds/cache/quota", ""))
	var report buildCacheQuotaResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &report) != nil {
		t.Fatalf("report = %d %s", rec.Code, rec.Body.String())
	}
	if report.AppQuotaGB != 1 || !report.Supported || len(report.Reclaims) != 2 || report.Reclaims[0].App != "web" || report.Reclaims[1].Skipped != cacheReclaimSkipRunning {
		t.Fatalf("report = %+v", report)
	}
}

func TestCacheQuotaReportFiltersByVisibility(t *testing.T) {
	l := &cacheReclaimLog{running: map[string]bool{}}
	l.add(cacheReclaimEntry{App: "mine"})
	l.add(cacheReclaimEntry{App: "theirs"})
	got := l.newestFirst(func(app string) bool { return app == "mine" })
	if len(got) != 1 || got[0].App != "mine" {
		t.Fatalf("visible = %+v, want only mine", got)
	}
	if !l.claim("web") || l.claim("web") {
		t.Fatal("a second reclaim of one app must not start")
	}
}
