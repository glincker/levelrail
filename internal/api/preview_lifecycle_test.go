package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/libdns/libdns"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestPreviewMemoryBytes(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"256Mi", 256 << 20}, {"1Gi", 1 << 30}, {"512M", 512_000_000}, {"", 0}, {"abc", 0}, {"-5Mi", 0},
	}
	for _, tt := range tests {
		if got := previewMemoryBytes(tt.in); got != tt.want {
			t.Errorf("previewMemoryBytes(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestClampPreviewResources(t *testing.T) {
	tests := []struct {
		name     string
		in       *spec.Resources
		wantMem  string
		wantCPU  float64
		limitMem string
		limitCPU float64
	}{
		{"none set gets the preview limits", nil, "256Mi", 0.25, "256Mi", 0.25},
		{"production above the ceiling is clamped", &spec.Resources{Memory: "2Gi", CPU: 2}, "256Mi", 0.25, "256Mi", 0.25},
		{"production below the ceiling is kept", &spec.Resources{Memory: "128Mi", CPU: 0.1}, "128Mi", 0.1, "256Mi", 0.25},
		{"unparseable production memory falls back", &spec.Resources{Memory: "lots"}, "256Mi", 0.25, "256Mi", 0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := spec.Service{Resources: tt.in}
			clampPreviewResources(&svc, tt.limitMem, tt.limitCPU)
			if svc.Resources.Memory != tt.wantMem || svc.Resources.CPU != tt.wantCPU {
				t.Errorf("resources = %+v, want memory %q cpu %v", *svc.Resources, tt.wantMem, tt.wantCPU)
			}
		})
	}
}

func TestDropDatabaseEnv(t *testing.T) {
	env := map[string]spec.EnvVar{
		"DATABASE_URL":    {Value: "postgres://prod"},
		"APP_DB_HOST":     {Value: "prod-db"},
		"REDIS_URL":       {Value: "redis://prod"},
		"FEATURE_FLAG":    {Value: "on"},
		"SESSION_TIMEOUT": {Value: "30"},
		"CUSTOM_CONN":     {Value: "x"},
		"ref":             {From: "postgres.main.url"},
	}
	got := dropDatabaseEnv(env, "CUSTOM_CONN")
	for _, k := range []string{"DATABASE_URL", "APP_DB_HOST", "REDIS_URL", "CUSTOM_CONN", "ref"} {
		if _, ok := got[k]; ok {
			t.Errorf("%s survived the database strip", k)
		}
	}
	for _, k := range []string{"FEATURE_FLAG", "SESSION_TIMEOUT"} {
		if _, ok := got[k]; !ok {
			t.Errorf("%s was stripped but is not a database variable", k)
		}
	}
}

func TestPreviewForkEnvPolicyMatrix(t *testing.T) {
	tests := []struct {
		name         string
		fork         bool
		allowSecrets bool
		wantEnv      bool
	}{
		{"same repo inherits", false, false, true},
		{"same repo with fork secrets on inherits", false, true, true},
		{"fork gets nothing by default", true, false, false},
		{"fork gets env only with explicit opt in", true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := spec.Service{Env: map[string]spec.EnvVar{"API_TOKEN": {Secret: true}, "MODE": {Value: "x"}}}
			vars := previewEnvVars{Fork: tt.fork, Policy: store.PreviewAppSettings{AllowForkSecrets: tt.allowSecrets, DatabaseStrategy: store.PreviewDatabaseStrategyNone}}
			applyPreviewPolicy(&svc, vars, "")
			if got := len(svc.Env) > 0; got != tt.wantEnv {
				t.Errorf("env present = %v, want %v (%v)", got, tt.wantEnv, svc.Env)
			}
		})
	}
}

func TestPreviewIdleSleepMinutes(t *testing.T) {
	t.Setenv(envPreviewIdleSleep, "")
	if got := previewIdleSleepMinutes(store.PreviewAppSettings{}); got != defaultPreviewIdleSleep {
		t.Errorf("default = %d, want %d", got, defaultPreviewIdleSleep)
	}
	t.Setenv(envPreviewIdleSleep, "0")
	if got := previewIdleSleepMinutes(store.PreviewAppSettings{}); got != 0 {
		t.Errorf("env 0 = %d, want 0 (never sleep)", got)
	}
	if got := previewIdleSleepMinutes(store.PreviewAppSettings{IdleSleepMinutes: 15}); got != 15 {
		t.Errorf("app override = %d, want 15", got)
	}
}

func TestPreviewExpiry(t *testing.T) {
	updated := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	p := store.PreviewEnvironment{UpdatedAt: updated.Format(time.RFC3339Nano)}
	ttl := 24 * time.Hour

	got, ok := previewExpiry(p, ttl)
	if !ok || !got.Equal(updated.Add(ttl)) {
		t.Fatalf("plain expiry = %v ok=%v, want %v", got, ok, updated.Add(ttl))
	}
	p.ExtendedUntil = updated.Add(72 * time.Hour).Format(time.RFC3339Nano)
	if got, _ := previewExpiry(p, ttl); !got.Equal(updated.Add(72 * time.Hour)) {
		t.Errorf("extended expiry = %v, want the extension", got)
	}
	p.ExtendedUntil = updated.Add(time.Hour).Format(time.RFC3339Nano)
	if got, _ := previewExpiry(p, ttl); !got.Equal(updated.Add(ttl)) {
		t.Errorf("an extension shorter than the TTL must not shorten it, got %v", got)
	}
	if _, ok := previewExpiry(store.PreviewEnvironment{UpdatedAt: "garbage"}, ttl); ok {
		t.Error("an unreadable timestamp must report not ok")
	}
}

func TestPreviewLimitsFor_PerAppCapOverridesPlatform(t *testing.T) {
	rt := &Router{previewLimits: PreviewLimits{MaxPerApp: 5, MaxTotal: 20}}
	if got := rt.previewLimitsFor(store.PreviewAppSettings{}); got.MaxPerApp != 5 {
		t.Errorf("platform cap = %d, want 5", got.MaxPerApp)
	}
	if got := rt.previewLimitsFor(store.PreviewAppSettings{MaxPreviews: 2}); got.MaxPerApp != 2 || got.MaxTotal != 20 {
		t.Errorf("per-app cap = %+v, want MaxPerApp 2 and the platform total kept", got)
	}
}

func TestRenderPreviewComment_CleanupNoteAndLogsLink(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 30, 0, 0, time.UTC)
	p := store.PreviewEnvironment{PreviewAppID: "web-pr-7", HeadSHA: "0123456789", Domain: "web-pr-7.apps.example.com"}
	x := previewCommentExtras{LogsURL: "https://cp.example.com/apps/web-pr-7/logs", ExpiresAt: now.Add(48 * time.Hour)}

	ready := renderPreviewCommentWith(p, previewCommentReady, "", now, x)
	for _, want := range []string{"Build logs:** https://cp.example.com/apps/web-pr-7/logs", "Cleanup:", "2026-09-28 12:30 UTC"} {
		if !strings.Contains(ready, want) {
			t.Errorf("ready comment missing %q:\n%s", want, ready)
		}
	}
	failed := renderPreviewCommentWith(p, previewCommentFailed, "boom", now, x)
	if !strings.Contains(failed, "Build logs:") || strings.Contains(failed, "Cleanup:") {
		t.Errorf("failed comment should link logs without a cleanup promise:\n%s", failed)
	}
}

// lifecycleFixture is a GitHub-backed preview app with an apps base domain, a
// fake DNS provider and automatic DNS on, the exit-criterion setup.
type lifecycleFixture struct {
	rt      *Router
	db      *store.DB
	secret  string
	builder *sequencedBuilder
	dns     *fakeDNSManager
	fake    *fakeGitHubAppClient
}

func newLifecycleFixture(t *testing.T) *lifecycleFixture {
	t.Helper()
	rt, secret, builder, fake := setUpPreviewAppWithGitHubNotifications(t, true)
	db := previewDB(t, rt)
	ctx := context.Background()

	settings, err := db.GetIngressSettings(ctx)
	if err != nil {
		t.Fatalf("GetIngressSettings() error = %v", err)
	}
	settings.AppsBaseDomain = "apps.example.com"
	if err := db.UpdateIngressSettings(ctx, settings); err != nil {
		t.Fatalf("UpdateIngressSettings() error = %v", err)
	}
	if err := db.SetDomainAutomationRaw(ctx, `{"auto_dns":true}`); err != nil {
		t.Fatalf("SetDomainAutomationRaw() error = %v", err)
	}

	mgr := &fakeDNSManager{}
	rt.publicHost = "198.51.100.20"
	rt.detectPublicIPs = func(context.Context) []string { return nil }
	rt.domainAuto.resolveTarget = func(_ context.Context, domain string) (*dnsTarget, error) {
		if !strings.HasSuffix(domain, "example.com") {
			return nil, dnsrecords.ErrZoneNotFound
		}
		return &dnsTarget{Manager: mgr, Provider: "fake", Zone: "example.com."}, nil
	}
	rt.domainAuto.proxy = func(context.Context, *http.Request) *proxyIntegrationView { return nil }
	return &lifecycleFixture{rt: rt, db: db, secret: secret, builder: builder, dns: mgr, fake: fake}
}

func (f *lifecycleFixture) send(action string, n int, sha string) *httptest.ResponseRecorder {
	return sendPullRequestWebhook(f.rt, f.secret, githubPullRequestBody(action, n, sha, "main"))
}

func commentBodies(c *fakePRComments) []string {
	out := make([]string, 0, len(c.comments))
	for _, cm := range c.comments {
		out = append(out, cm.Body)
	}
	return out
}

func hasRecord(recs []libdns.Record, name string) bool {
	for _, r := range recs {
		if strings.EqualFold(r.RR().Name, name) {
			return true
		}
	}
	return false
}

// TestPreviewLifecycle_FakeProviderEndToEnd is the exit criterion: opening a
// pull request deploys a preview on its own domain with a DNS record, a
// comment and safe defaults; a push updates it; closing removes all of it.
func TestPreviewLifecycle_FakeProviderEndToEnd(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()

	if rec := f.send("opened", 42, "sha1"); rec.Code != http.StatusOK {
		t.Fatalf("open: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	const host = "web-pr-42.apps.example.com"

	p, err := f.db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42)
	if err != nil {
		t.Fatalf("preview row missing: %v", err)
	}
	if p.Status != store.PreviewStatusActive || p.Domain != host {
		t.Fatalf("preview = status %q domain %q, want active on %q", p.Status, p.Domain, host)
	}
	if !hasRecord(f.dns.recs, "web-pr-42.apps") {
		t.Errorf("no DNS record created for the preview, records = %v", f.dns.recs)
	}
	if hidden, _ := f.db.IsDomainHidden(ctx, host); !hidden {
		t.Error("preview domain must be hidden from search engines by default")
	}
	if len(f.builder.calls) != 1 {
		t.Fatalf("builds = %d, want 1", len(f.builder.calls))
	}
	res := f.builder.calls[0].Service.Resources
	if res == nil || res.Memory != defaultPreviewMemory || res.CPU != defaultPreviewCPU {
		t.Errorf("preview resources = %+v, want the smaller preview defaults", res)
	}
	if sleep, err := f.db.GetAppSleep(ctx, "web-pr-42"); err != nil || sleep.IdleMinutes != defaultPreviewIdleSleep {
		t.Errorf("idle sleep = %+v (err %v), want %d minutes", sleep, err, defaultPreviewIdleSleep)
	}
	if len(f.fake.commentCalls) != 1 {
		t.Fatalf("comment calls = %d, want one comment for the whole lifecycle so far", len(f.fake.commentCalls))
	}
	body := commentBodies(&f.fake.prComments)
	if len(body) != 1 || !strings.Contains(body[0], host) || !strings.Contains(body[0], "Cleanup:") {
		t.Errorf("comment = %v, want the preview URL and a cleanup note", body)
	}

	if rec := f.send("synchronize", 42, "sha2"); rec.Code != http.StatusOK {
		t.Fatalf("push: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(f.builder.calls) != 2 || f.builder.calls[1].CommitSHA != "sha2" {
		t.Fatalf("push did not redeploy the new head: %+v", f.builder.calls)
	}
	if recs := commentBodies(&f.fake.prComments); len(recs) != 1 {
		t.Errorf("comments = %d, want the single comment edited in place", len(recs))
	}

	if rec := f.send("closed", 42, "sha2"); rec.Code != http.StatusOK {
		t.Fatalf("close: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if _, err := f.db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42); err == nil {
		t.Error("preview row still present after close")
	}
	if _, err := f.db.GetDesiredService(ctx, "web-pr-42"); err == nil {
		t.Error("preview service still present after close")
	}
	if hasRecord(f.dns.recs, "web-pr-42.apps") {
		t.Errorf("DNS record not removed on close, records = %v", f.dns.recs)
	}
}

// failingDNSManager fails record deletion, the half-succeeded teardown case.
type failingDNSManager struct{ fakeDNSManager }

func (m *failingDNSManager) DeleteRecords(context.Context, string, []libdns.Record) ([]libdns.Record, error) {
	return nil, context.DeadlineExceeded
}

func TestPreviewLifecycle_TeardownWithDNSFailureKeepsRowForRetry(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	if rec := f.send("opened", 7, "sha1"); rec.Code != http.StatusOK {
		t.Fatalf("open: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	bad := &failingDNSManager{fakeDNSManager{recs: f.dns.recs}}
	f.rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Manager: bad, Provider: "fake", Zone: "example.com."}, nil
	}
	if rec := f.send("closed", 7, "sha1"); rec.Code != http.StatusMultiStatus {
		t.Fatalf("close with a DNS failure: status = %d, want 207, body = %s", rec.Code, rec.Body.String())
	}
	p, err := f.db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 7)
	if err != nil {
		t.Fatalf("row must survive a partial teardown: %v", err)
	}
	if p.Status != store.PreviewStatusFailed || !strings.Contains(p.StatusReason, "dns:") {
		t.Errorf("row = status %q reason %q, want failed naming the dns record", p.Status, p.StatusReason)
	}

	f.rt.domainAuto.resolveTarget = func(context.Context, string) (*dnsTarget, error) {
		return &dnsTarget{Manager: f.dns, Provider: "fake", Zone: "example.com."}, nil
	}
	status, msg := f.rt.teardownPreviewRecord(ctx, *p)
	if status != http.StatusOK {
		t.Fatalf("retry teardown: status = %d, msg = %s", status, msg)
	}
	if hasRecord(f.dns.recs, "web-pr-7.apps") {
		t.Error("DNS record still present after the retry")
	}
}

func TestPreviewLifecycle_ForkGetsNoEnvAndNoDatabase(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	s := store.DefaultPreviewAppSettings("web")
	s.AllowForkPreviews = true
	if err := f.db.SavePreviewAppSettings(ctx, s); err != nil {
		t.Fatalf("save policy: %v", err)
	}
	prod := store.DesiredService{Name: "web", Image: "x", Env: map[string]string{"API_KEY": "prod-key", "DATABASE_URL": "postgres://prod"}}
	if err := f.db.SaveDesiredService(ctx, prod); err != nil {
		t.Fatalf("seed production env: %v", err)
	}

	fork := githubPullRequestBodyFrom("opened", 9, "sha1", "main", "mallory/web")
	if rec := sendPullRequestWebhook(f.rt, f.secret, fork); rec.Code != http.StatusOK {
		t.Fatalf("fork open: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(f.builder.calls) != 1 {
		t.Fatalf("builds = %d, want the allowed fork deployed once", len(f.builder.calls))
	}
	env := f.builder.calls[0].Service.Env
	for k := range env {
		if !strings.HasPrefix(k, "PREVIEW_") {
			t.Errorf("fork preview inherited %q from production", k)
		}
	}
}

func TestPreviewLifecycle_SameRepoDropsDatabaseEnvByDefault(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	prod := store.DesiredService{Name: "web", Image: "x", Env: map[string]string{"DATABASE_URL": "postgres://prod", "LOG_LEVEL": "debug"}}
	if err := f.db.SaveDesiredService(ctx, prod); err != nil {
		t.Fatalf("seed production env: %v", err)
	}
	if rec := f.send("opened", 11, "sha1"); rec.Code != http.StatusOK {
		t.Fatalf("open: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	env := f.builder.calls[0].Service.Env
	if _, ok := env["DATABASE_URL"]; ok {
		t.Error("preview inherited the production DATABASE_URL")
	}
	if env["LOG_LEVEL"].Value != "debug" {
		t.Errorf("non-database env was not inherited: %v", env)
	}
}

func TestPreviewLifecycle_PerAppCapEvictsOldest(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	s := store.DefaultPreviewAppSettings("web")
	s.MaxPreviews = 1
	if err := f.db.SavePreviewAppSettings(ctx, s); err != nil {
		t.Fatalf("save policy: %v", err)
	}
	f.rt.previewLimits = PreviewLimits{MaxPerApp: 10}

	f.send("opened", 1, "sha1")
	f.send("opened", 2, "sha2")

	if _, err := f.db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 1); err == nil {
		t.Error("the oldest preview should have been evicted at the per-app cap of 1")
	}
	if p, err := f.db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 2); err != nil || p.Status != store.PreviewStatusActive {
		t.Errorf("newest preview = %+v err %v, want active", p, err)
	}
}

func TestPreviewLifecycle_ExtendAndPolicyRoutes(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	cookie := loginTestSession(t, f.rt, f.db)
	f.send("opened", 5, "sha1")

	call := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		f.rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	rec := call(http.MethodPost, "/api/v1/apps/web/previews/5/extend", `{"hours":48}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("extend: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var res previewEnvironmentResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res.Extended {
		t.Fatalf("extend response = %+v err %v, want extended", res, err)
	}
	row, _ := f.db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 5)
	if row.ExtendedUntil == "" {
		t.Error("extension was not stored")
	}
	if rec := call(http.MethodPost, "/api/v1/apps/web/previews/5/extend", `{"hours":0}`); rec.Code != http.StatusBadRequest {
		t.Errorf("extend 0h: status = %d, want 400", rec.Code)
	}
	if rec := call(http.MethodPost, "/api/v1/apps/web/previews/99/extend", `{"hours":1}`); rec.Code != http.StatusNotFound {
		t.Errorf("extend unknown PR: status = %d, want 404", rec.Code)
	}

	rec = call(http.MethodPut, "/api/v1/apps/web/preview-policy", `{"database_strategy":"fresh","max_previews":3,"memory_limit":"512Mi","allow_indexing":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("policy: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var pol previewPolicyResource
	if err := json.Unmarshal(rec.Body.Bytes(), &pol); err != nil {
		t.Fatalf("decode policy: %v", err)
	}
	if pol.DatabaseStrategy != "fresh" || pol.MaxPerApp != 3 || pol.EffectiveMemory != "512Mi" || !pol.AllowIndexing {
		t.Errorf("policy = %+v", pol)
	}
	for _, bad := range []string{`{"database_strategy":"prod"}`, `{"max_previews":500}`, `{"memory_limit":"1Ki"}`, `{"gate_basic_auth":true}`} {
		if rec := call(http.MethodPut, "/api/v1/apps/web/preview-policy", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("policy %s: status = %d, want 400", bad, rec.Code)
		}
	}
}

func TestPreviewLifecycle_SweepHonoursExtension(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx := context.Background()
	f.rt.previewTTL = time.Hour
	f.send("opened", 3, "sha1")
	f.send("opened", 4, "sha2")

	old := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339Nano)
	for _, n := range []int{3, 4} {
		p, _ := f.db.GetPreviewEnvironmentByAppAndPR(ctx, "web", n)
		p.UpdatedAt = old
		if err := f.db.UpdatePreviewEnvironment(ctx, *p); err != nil {
			t.Fatalf("age preview: %v", err)
		}
	}
	p4, _ := f.db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 4)
	if err := f.db.SetPreviewEnvironmentExtendedUntil(ctx, p4.ID, time.Now().Add(6*time.Hour).UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatalf("extend: %v", err)
	}

	swept, err := f.rt.SweepStalePreviewEnvironments(ctx)
	if err != nil || swept != 1 {
		t.Fatalf("swept = %d err = %v, want only the un-extended preview removed", swept, err)
	}
	if _, err := f.db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 4); err != nil {
		t.Errorf("extended preview was swept: %v", err)
	}
}
