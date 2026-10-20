package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/platformimport/fakecoolify"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const appImportBase = "/api/v1/migration/apps"

type appImportHarness struct {
	t      *testing.T
	rt     *Router
	db     *store.DB
	cookie *http.Cookie
	fake   *fakecoolify.Server
	src    *httptest.Server
	setter *fakeSecretSetter
}

func newAppImportHarness(t *testing.T) *appImportHarness {
	t.Helper()
	t.Setenv(platformimport.AllowLoopbackEnv, "true")
	fake := fakecoolify.New()
	src := httptest.NewServer(fake.Handler())
	t.Cleanup(src.Close)
	db := openTestDB(t)
	setter := &fakeSecretSetter{}
	rt := NewRouter(nil, testBrand(), db, WithSecretSetter(setter), WithBuilder(&fakeBuilder{tag: "img:sha"}), WithGitSourceSecrets(newFakeGitSourceSecrets()))
	rt.gitSourceFetch = func(context.Context, string, string, string) (string, func(), error) {
		return t.TempDir(), func() {}, nil
	}
	return &appImportHarness{t: t, rt: rt, db: db, cookie: loginTestSession(t, rt, db), fake: fake, src: src, setter: setter}
}

func (h *appImportHarness) call(method, path, body string) *httptest.ResponseRecorder {
	h.t.Helper()
	return serve(h.rt, authedRequest(h.t, h.cookie, method, path, body))
}

func (h *appImportHarness) connectBody(extra string) string {
	return `{"platform":"coolify","url":"` + h.src.URL + `","token":"` + fakecoolify.Token + `","allow_loopback":true` + extra + `}`
}

func (h *appImportHarness) view(rec *httptest.ResponseRecorder) appImportSessionResource {
	h.t.Helper()
	var v appImportSessionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		h.t.Fatalf("decode view: %v: %s", err, rec.Body.String())
	}
	return v
}

func (v appImportSessionResource) item(name string) appImportItemResource {
	for _, it := range v.Items {
		if it.Name == name {
			return it
		}
	}
	return appImportItemResource{}
}

func (h *appImportHarness) noLeaks(body string) {
	h.t.Helper()
	for _, leak := range []string{fakecoolify.Token, fakecoolify.WebSecret, fakecoolify.WebDBPassword} {
		if strings.Contains(body, leak) {
			h.t.Fatalf("response leaks %q: %s", leak, body)
		}
	}
}

func (h *appImportHarness) ready(app string) {
	h.t.Helper()
	conds := []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: reconcile.ConditionTrue, Reason: "Deployed"}}
	if err := h.db.UpsertConditions(context.Background(), applicationControllerName(app), conds); err != nil {
		h.t.Fatal(err)
	}
}

func TestAppImportFullFlow(t *testing.T) {
	h := newAppImportHarness(t)
	ctx := context.Background()

	// A database moved earlier, recognized by its name.
	if err := h.db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main-db", Engine: "postgres", Version: "16"}); err != nil {
		t.Fatal(err)
	}

	rec := h.call(http.MethodPost, appImportBase+"/sessions", h.connectBody(""))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session: %d %s", rec.Code, rec.Body.String())
	}
	h.noLeaks(rec.Body.String())
	v := h.view(rec)
	id := v.ID
	if len(v.Items) != 7 {
		t.Fatalf("items = %d, want 5 apps, compose and one service: %+v", len(v.Items), v.Counts)
	}
	checks := []struct{ name, verdict string }{
		{"searxng", "ready"}, {"web", "ready-with-notes"}, {"memgraph", "ready-with-notes"},
		{"go-api", "needs-attention"}, {"report-worker", "needs-attention"},
		{"collab-server", "unsupported"}, {"plausible", "unsupported"},
	}
	for _, c := range checks {
		if got := v.item(c.name).Entry.Verdict; got != c.verdict {
			t.Errorf("%s verdict = %q, want %q (%+v)", c.name, got, c.verdict, v.item(c.name).Entry.Findings)
		}
	}
	if web := v.item("web"); len(web.Entry.Databases) != 1 || web.Entry.Databases[0].Target != "main-db" || web.Entry.Env.Secret != 2 || web.Entry.Env.Plain != 2 {
		t.Errorf("web entry: %+v", web.Entry)
	}
	if len(v.Suggested) != 1 || v.Suggested[0].From != fakecoolify.PGUUID {
		t.Fatalf("suggested mappings = %+v", v.Suggested)
	}
	if mem := v.item("memgraph"); len(mem.Entry.Volumes) != 1 || !mem.Entry.Volumes[0].SizeKnown {
		t.Errorf("memgraph volume size missing: %+v", mem.Entry.Volumes)
	}

	// Preflight blocks staging while the database hostname is unmapped.
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("stage without mapping = %d: %s", rec.Code, rec.Body.String())
	}
	if svcs, _ := h.db.ListDesiredServices(ctx); len(svcs) != 0 {
		t.Fatalf("a blocked stage created %d apps", len(svcs))
	}

	// Select the apps to import and apply the suggested mapping.
	sel := `["` + v.item("searxng").SourceID + `","` + v.item("web").SourceID + `","` + v.item("memgraph").SourceID + `"]`
	rec = h.call(http.MethodPut, appImportBase+"/sessions/"+id+"/plan",
		`{"selected":`+sel+`,"mappings":[{"from":"`+fakecoolify.PGUUID+`","to":"`+v.Suggested[0].To+`"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put plan: %d %s", rec.Code, rec.Body.String())
	}
	h.noLeaks(rec.Body.String())
	v = h.view(rec)
	if v.Preflight == nil || !v.Preflight.CanStage || len(v.Diff) != 1 || v.Diff[0].Key != "DATABASE_URL" {
		t.Fatalf("preflight/diff: %+v", v.Preflight)
	}
	if !strings.Contains(v.Diff[0].After, v.Suggested[0].To) || strings.Contains(v.Diff[0].After, fakecoolify.WebDBPassword) {
		t.Errorf("diff after = %q", v.Diff[0].After)
	}

	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("stage: %d %s", rec.Code, rec.Body.String())
	}
	h.noLeaks(rec.Body.String())
	v = h.view(rec)
	for _, n := range []string{"searxng", "web", "memgraph"} {
		if v.item(n).State != store.AppImportStaged {
			t.Errorf("%s state = %s (%s)", n, v.item(n).State, v.item(n).Reason)
		}
	}

	web, err := h.db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case !web.Suspended:
		t.Error("staged app must not be started")
	case len(web.Domains) != 0:
		t.Errorf("staged app must not be routed: %v", web.Domains)
	case web.Labels[platformimport.LabelSession] != id:
		t.Errorf("labels = %v", web.Labels)
	case !strings.Contains(web.Env["DATABASE_URL"], v.Suggested[0].To) && web.Env["DATABASE_URL"] != "":
		t.Errorf("plain env was not rewritten: %v", web.Env)
	}
	if got := h.setter.sets; len(got) < 2 {
		t.Errorf("secrets stored = %+v", got)
	}
	foundRewritten := false
	for _, s := range h.setter.sets {
		if s.service == "web" && s.key == "DATABASE_URL" && strings.Contains(s.value, v.Suggested[0].To) && strings.Contains(s.value, fakecoolify.WebDBPassword) {
			foundRewritten = true
		}
	}
	if !foundRewritten {
		t.Errorf("DATABASE_URL secret not stored rewritten: %+v", h.setter.sets)
	}
	if len(web.DatabaseEnv) != 1 {
		t.Errorf("web should join the database network via a connection: %+v", web.DatabaseEnv)
	}
	if gs, err := h.db.GetGitSource(ctx, "web"); err != nil || gs.RepoURL != "https://github.com/acme/web" || gs.BuildType != "dockerfile" {
		t.Errorf("git source: %+v %v", gs, err)
	}
	if mg, _ := h.db.GetDesiredService(ctx, "memgraph"); len(mg.Volumes) != 1 || mg.Volumes[0].Name != "app-memgraph-"+platformimport.SanitizeName(fakecoolify.AppMemgraph+"-data") {
		t.Errorf("memgraph volumes: %+v", mg.Volumes)
	}

	// Resume is idempotent: nothing is created twice.
	before := len(h.setter.sets)
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`)
	if rec.Code != http.StatusOK || len(h.setter.sets) != before {
		t.Fatalf("re-stage = %d, secrets %d -> %d", rec.Code, before, len(h.setter.sets))
	}

	// Verify: image app readiness comes from the conditions.
	h.ready("searxng")
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/verify", `{"items":["searxng"]}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("verify: %d %s", rec.Code, rec.Body.String())
	}
	waitFor(t, 10*time.Second, func() bool {
		items, _ := h.db.ListAppImportItems(ctx, id)
		for _, it := range items {
			if it.SourceName == "searxng" {
				return it.State == store.AppImportVerified
			}
		}
		return false
	})
	if sx, _ := h.db.GetDesiredService(ctx, "searxng"); !sx.Suspended {
		t.Error("a verified app must be stopped again until cutover")
	}

	// Volumes: commands use the original source volume name; copied is the operator's call.
	rec = h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/volumes?source=root@coolify.example.com", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "/var/lib/docker/volumes/"+fakecoolify.AppMemgraph+"-data/_data/") {
		t.Fatalf("volume guide: %d %s", rec.Code, rec.Body.String())
	}
	rec = h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/volumes?source=bad", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad ssh target = %d", rec.Code)
	}

	// Cutover guidance is read-only and does not attach domains.
	rec = h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/cutover?target_ip=203.0.113.9", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "search.example.com") {
		t.Fatalf("cutover: %d %s", rec.Code, rec.Body.String())
	}
	if sx, _ := h.db.GetDesiredService(ctx, "searxng"); len(sx.Domains) != 0 {
		t.Fatal("cutover guidance must not attach domains")
	}

	// Enable routing is explicit.
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/items/"+v.item("searxng").SourceID+"/route", `{"enable":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("route: %d %s", rec.Code, rec.Body.String())
	}
	if sx, _ := h.db.GetDesiredService(ctx, "searxng"); sx.Suspended || len(sx.Domains) != 1 || sx.Domains[0] != "search.example.com" {
		t.Errorf("routed app: suspended=%v domains=%v", sx.Suspended, sx.Domains)
	}

	// Receipt carries no secret.
	rec = h.call(http.MethodGet, appImportBase+"/sessions/"+id+"/receipt", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("receipt: %d", rec.Code)
	}
	h.noLeaks(rec.Body.String())
	if !strings.Contains(rec.Body.String(), `"rewritten"`) || !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Errorf("receipt body/headers: %s", rec.Body.String())
	}

	// Rollback removes only what this session created.
	if err := h.db.SaveDesiredService(ctx, store.DesiredService{Name: "bystander", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/rollback", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("rollback: %d %s", rec.Code, rec.Body.String())
	}
	for _, n := range []string{"web", "searxng", "memgraph"} {
		if _, err := h.db.GetDesiredService(ctx, n); err == nil {
			t.Errorf("%s survived rollback", n)
		}
	}
	if _, err := h.db.GetDesiredService(ctx, "bystander"); err != nil {
		t.Errorf("rollback removed an unrelated app: %v", err)
	}
	if _, err := h.db.GetDesiredDatabase(ctx, "main-db"); err != nil {
		t.Errorf("rollback removed a database: %v", err)
	}

	if v := h.fake.Violations(); len(v) != 0 {
		t.Fatalf("the source received non-GET requests: %v", v)
	}
	if len(h.fake.Requests()) == 0 {
		t.Fatal("the fake source was never called")
	}
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("condition not met in time")
}

func TestAppImportPartialFailureResume(t *testing.T) {
	h := newAppImportHarness(t)
	ctx := context.Background()
	v := h.view(h.call(http.MethodPost, appImportBase+"/sessions", h.connectBody("")))
	id := v.ID
	plan := `{"selected":["` + v.item("searxng").SourceID + `","` + v.item("web").SourceID + `"],"mappings":[{"from":"` + fakecoolify.PGUUID + `","to":"pg-main"}]}`
	if rec := h.call(http.MethodPut, appImportBase+"/sessions/"+id+"/plan", plan); rec.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", rec.Code, rec.Body.String())
	}

	h.setter.err = errors.New("secret store is down")
	v = h.view(h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`))
	if v.item("searxng").State != store.AppImportStaged {
		t.Errorf("one failure must not stop the others: searxng = %s", v.item("searxng").State)
	}
	if web := v.item("web"); web.State != store.AppImportStageFailed || web.Reason == "" {
		t.Errorf("web = %s %q", web.State, web.Reason)
	}

	h.setter.err = nil
	v = h.view(h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`))
	if v.item("web").State != store.AppImportStaged || v.item("searxng").State != store.AppImportStaged {
		t.Errorf("resume: web=%s searxng=%s", v.item("web").State, v.item("searxng").State)
	}
	svcs, _ := h.db.ListDesiredServices(ctx)
	if len(svcs) != 2 {
		t.Errorf("apps after resume = %d, want 2", len(svcs))
	}
}

func TestAppImportNeedsReconnectAndRejectsForeignSource(t *testing.T) {
	h := newAppImportHarness(t)
	rec := h.call(http.MethodPost, appImportBase+"/sessions", h.connectBody(""))
	id := h.view(rec).ID

	h.rt.appImportLive.forget(id)
	rec = h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/stage", `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("stage without token = %d", rec.Code)
	}
	rec = h.call(http.MethodGet, appImportBase+"/sessions/"+id, "")
	v := h.view(rec)
	if v.Connected || len(v.Items) != 7 {
		t.Errorf("stored view: connected=%v items=%d", v.Connected, len(v.Items))
	}
	other := `{"platform":"coolify","url":"http://127.0.0.1:1","token":"x","allow_loopback":true}`
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/connect", other); rec.Code != http.StatusBadRequest {
		t.Errorf("connect to another source = %d", rec.Code)
	}
	if rec := h.call(http.MethodPost, appImportBase+"/sessions/"+id+"/connect", h.connectBody("")); rec.Code != http.StatusOK {
		t.Errorf("reconnect = %d: %s", rec.Code, rec.Body.String())
	}
	if v := h.fake.Violations(); len(v) != 0 {
		t.Fatalf("non-GET to source: %v", v)
	}
}

func TestAppImportSourceNetworkPolicyAndTokenHandling(t *testing.T) {
	h := newAppImportHarness(t)
	t.Setenv(platformimport.AllowLoopbackEnv, "")
	cases := []struct{ name, body string }{
		{"loopback opt-in needs the env", h.connectBody("")},
		{"loopback blocked by default", `{"platform":"coolify","url":"` + h.src.URL + `","token":"t"}`},
		{"metadata address", `{"platform":"coolify","url":"http://169.254.169.254","token":"t"}`},
	}
	for _, c := range cases {
		rec := h.call(http.MethodPost, appImportBase+"/plan", c.body)
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusBadGateway {
			t.Errorf("%s: status %d", c.name, rec.Code)
		}
		if strings.Contains(rec.Body.String(), fakecoolify.Token) {
			t.Errorf("%s: token echoed", c.name)
		}
	}
	if len(h.fake.Requests()) != 0 {
		t.Errorf("a blocked source was contacted: %v", h.fake.Requests())
	}
}

func TestAppImportPlanIsStateless(t *testing.T) {
	h := newAppImportHarness(t)
	rec := h.call(http.MethodPost, appImportBase+"/plan", h.connectBody(`,"only":["searxng"]`))
	if rec.Code != http.StatusOK {
		t.Fatalf("plan: %d %s", rec.Code, rec.Body.String())
	}
	h.noLeaks(rec.Body.String())
	v := h.view(rec)
	if !v.item("searxng").Selected || v.item("web").Selected {
		t.Errorf("only filter ignored: %+v", v.Items)
	}
	if sessions, _ := h.db.ListAppImportSessions(context.Background()); len(sessions) != 0 {
		t.Error("plan stored a session")
	}
	if svcs, _ := h.db.ListDesiredServices(context.Background()); len(svcs) != 0 {
		t.Error("plan created apps")
	}
}

func TestAppImportRedactedSecretsNeedAttention(t *testing.T) {
	h := newAppImportHarness(t)
	h.fake.RedactSensitive = true
	rec := h.call(http.MethodPost, appImportBase+"/plan", h.connectBody(""))
	v := h.view(rec)
	web := v.item("web")
	if web.Entry.Verdict != "needs-attention" || web.Entry.Env.Empty != 1 {
		t.Errorf("web: %+v", web.Entry)
	}
}
