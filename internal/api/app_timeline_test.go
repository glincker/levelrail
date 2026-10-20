package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func newTimelineRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithDeploySafety(db, nil), WithSecretSetter(secrets.NewManager(db, mk)))
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, Env: map[string]string{"A": "1"}}); err != nil {
		t.Fatal(err)
	}
	return rt, db, cookie
}

type failingDeclarerStore struct{ *store.DB }

func (failingDeclarerStore) SetServiceSecretEnvDeclared(context.Context, string, string, bool) (bool, error) {
	return false, errors.New("disk full")
}

func TestSecretsDeclareFailureIsReported(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)
	tlJSON(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/secrets/TOKEN", `{"value":"v"}`, nil)
	rt.apps = failingDeclarerStore{db}

	if code := tlJSON(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/secrets/OTHER", `{"value":"v"}`, nil); code != http.StatusInternalServerError {
		t.Fatalf("set with failing declare = %d, want 500", code)
	}
	if code := tlJSON(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/secrets/TOKEN", "", nil); code != http.StatusInternalServerError {
		t.Fatalf("delete with failing undeclare = %d, want 500", code)
	}
	if ok, _ := db.HasSecretValue(context.Background(), "web", "TOKEN"); !ok {
		t.Fatal("value deleted although the key is still declared")
	}
}

func tlJSON(t *testing.T, rt *Router, cookie *http.Cookie, method, target, body string, out any) int {
	t.Helper()
	rec := serve(rt, authedRequest(t, cookie, method, target, body))
	if out != nil && rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, target, rec.Body.String(), err)
		}
	}
	return rec.Code
}

func putApp(t *testing.T, rt *Router, cookie *http.Cookie, body string) {
	t.Helper()
	if code := tlJSON(t, rt, cookie, http.MethodPut, "/api/v1/apps/web", body, nil); code != http.StatusOK {
		t.Fatalf("PUT app = %d", code)
	}
}

func TestTimelineRecordsEventsWithKeyNamesOnly(t *testing.T) {
	rt, _, cookie := newTimelineRouter(t)
	putApp(t, rt, cookie, `{"image":"nginx:1","port":80,"env":{"A":"1","B":"hunter2"}}`)
	putApp(t, rt, cookie, `{"image":"nginx:1","port":8080,"replicas":3,"env":{"A":"1","B":"hunter2"},"domains":["a.example.com"]}`)
	tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/restart", "", nil)
	tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/stop", "", nil)
	tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/start", "", nil)

	var tl timelineResponse
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/timeline", "", &tl); code != http.StatusOK {
		t.Fatalf("timeline = %d", code)
	}
	kinds := map[string]timelineItem{}
	for _, it := range tl.Items {
		kinds[it.Kind] = it
	}
	for _, k := range []string{"env_change", "config_change", "scale", "restart", "suspend", "resume"} {
		if _, ok := kinds[k]; !ok {
			t.Fatalf("missing %s in %+v", k, tl.Items)
		}
	}
	env := kinds["env_change"]
	if !strings.Contains(env.Title, "B") || strings.Contains(env.Title+env.Detail, "hunter2") {
		t.Fatalf("env event = %+v", env)
	}
	if cfg := kinds["config_change"]; !strings.Contains(cfg.Detail, "port 80 to 8080") || !strings.Contains(cfg.Detail, "+a.example.com") {
		t.Fatalf("config event = %+v", cfg)
	}
	if kinds["scale"].Title != "Scaled 1 to 3 replicas" {
		t.Fatalf("scale title = %q", kinds["scale"].Title)
	}
	for _, it := range tl.Items {
		if it.Actor == "" || it.Status != "info" {
			t.Fatalf("event item %+v", it)
		}
	}
}

func TestTimelineMergesDeploysAndPaginatesStably(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	for i, img := range []string{"web:1", "web:2", "web:1"} {
		status := store.DeployAttemptStatusSucceeded
		if i == 2 {
			status = store.DeployAttemptStatusRunning
		}
		if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{
			ID: "att" + string(rune('a'+i)), ServiceName: "web", Image: img, Source: store.DeployAttemptSourceImage,
			Status: status, StartedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := db.AddAppEvent(ctx, store.AppEvent{AppName: "web", Kind: store.AppEventRestart, Actor: "system", Title: "Restarted", CreatedAt: base.Add(time.Duration(i)*time.Minute + 30*time.Second)}); err != nil {
			t.Fatal(err)
		}
	}

	var all timelineResponse
	tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/timeline?limit=100", "", &all)
	if len(all.Items) != 6 || all.NextCursor != "" {
		t.Fatalf("items = %d cursor = %q", len(all.Items), all.NextCursor)
	}
	top := all.Items[1]
	if top.Kind != "rollback" || top.Status != "in_progress" || top.Ref == nil || top.Ref.ID != "attc" {
		t.Fatalf("newest = %+v, want in-progress rollback of attc", top)
	}
	if all.Items[3].Kind != "deploy" || all.Items[3].Status != "succeeded" {
		t.Fatalf("fourth = %+v", all.Items[3])
	}

	var seen []string
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		var page timelineResponse
		target := "/api/v1/apps/web/timeline?limit=2"
		if cursor != "" {
			target += "&before=" + cursor
		}
		tlJSON(t, rt, cookie, http.MethodGet, target, "", &page)
		for _, it := range page.Items {
			seen = append(seen, it.ID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != 6 {
		t.Fatalf("paged ids = %v", seen)
	}
	for i, it := range all.Items {
		if seen[i] != it.ID {
			t.Fatalf("page order %v differs from full order at %d", seen, i)
		}
	}
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/timeline?before=garbage", "", nil); code != http.StatusBadRequest {
		t.Fatalf("bad cursor = %d", code)
	}
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/nope/timeline", "", nil); code != http.StatusNotFound {
		t.Fatalf("unknown app = %d", code)
	}
}

func snapshotFor(t *testing.T, db *store.DB, secretValues map[string]string) {
	t.Helper()
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	fields := store.AppliedFields(*svc)
	fields[store.AppliedFieldSecretKeys] = ""
	if err := db.SaveAppliedConfig(context.Background(), store.AppliedConfig{
		ServiceName: "web", Release: application.ContainerName("web", application.NameImage(*svc), svc.RestartNonce),
		EnvHashes: store.AppEnvHashes("web", svc.Env, secretValues), Fields: fields, AppliedAt: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
}

func pending(t *testing.T, rt *Router, cookie *http.Cookie) pendingChangesResource {
	t.Helper()
	var p pendingChangesResource
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/pending-changes", "", &p); code != http.StatusOK {
		t.Fatalf("pending = %d", code)
	}
	return p
}

func TestPendingChangesEnvSecretConfigThenApply(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)
	snapshotFor(t, db, nil)
	if p := pending(t, rt, cookie); p.Pending || len(p.Changes) != 0 || p.ApplyAction != "restart" {
		t.Fatalf("clean app pending = %+v", p)
	}

	putApp(t, rt, cookie, `{"image":"nginx:1","port":9090,"env":{"A":"2","C":"x"}}`)
	if code := tlJSON(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/secrets/TOKEN", `{"value":"s3cr3t-value"}`, nil); code != http.StatusNoContent {
		t.Fatalf("set secret = %d", code)
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/pending-changes", ""))
	if strings.Contains(rec.Body.String(), "s3cr3t-value") {
		t.Fatalf("secret value leaked: %s", rec.Body.String())
	}
	var p pendingChangesResource
	_ = json.Unmarshal(rec.Body.Bytes(), &p)
	byKind := map[string]pendingChange{}
	for _, c := range p.Changes {
		byKind[c.Kind] = c
	}
	if got := strings.Join(byKind["env"].Keys, ","); got != "A,C" {
		t.Fatalf("env keys = %q in %+v", got, p)
	}
	if got := strings.Join(byKind["secret"].Keys, ","); got != "TOKEN" {
		t.Fatalf("secret keys = %q in %+v", got, p)
	}
	if got := strings.Join(byKind["config"].Keys, ","); got != "port" {
		t.Fatalf("config keys = %q in %+v", got, p)
	}
	if !p.Pending || byKind["env"].Since == "" {
		t.Fatalf("pending = %+v", p)
	}

	var applied applyPendingResult
	if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/apply-pending", "", &applied); code != http.StatusAccepted {
		t.Fatalf("apply = %d", code)
	}
	svc, _ := db.GetDesiredService(context.Background(), "web")
	if svc.RestartNonce == "" {
		t.Fatal("apply did not restart the app")
	}
	if p := pending(t, rt, cookie); !p.Pending {
		t.Fatalf("mid-restart the old release is still the live one, want pending: %+v", p)
	}
	snapshotFor(t, db, map[string]string{"TOKEN": "s3cr3t-value"})
	if p := pending(t, rt, cookie); p.Pending {
		t.Fatalf("still pending once the new release is live: %+v", p)
	}
	var tl timelineResponse
	tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/timeline", "", &tl)
	if tl.Items[0].Kind != "restart" || tl.Items[0].Title != "Applied pending changes" {
		t.Fatalf("newest event = %+v", tl.Items[0])
	}
}

func TestPendingChangesHostPortAndLegacySecret(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)
	snapshotFor(t, db, nil)
	putApp(t, rt, cookie, `{"image":"nginx:1","port":80,"env":{"A":"1"},"host_port":18080}`)
	if p := pending(t, rt, cookie); len(p.Changes) != 1 || p.Changes[0].Kind != "config" || p.Changes[0].Keys[0] != "host_port" {
		t.Fatalf("host_port change pending = %+v", p)
	}

	rt2, _, cookie2 := newTimelineRouter(t)
	tlJSON(t, rt2, cookie2, http.MethodPut, "/api/v1/apps/web/secrets/TOKEN", `{"value":"v"}`, nil)
	if p := pending(t, rt2, cookie2); !p.Pending || p.Changes[0].Kind != "secret" {
		t.Fatalf("no-snapshot secret change pending = %+v", p)
	}
}

func TestPendingChangesSecretRotationAndLegacyDirtyLatch(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)
	tlJSON(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/secrets/TOKEN", `{"value":"one"}`, nil)
	snapshotFor(t, db, map[string]string{"TOKEN": "one"})
	if p := pending(t, rt, cookie); p.Pending {
		t.Fatalf("unrotated secret pending: %+v", p)
	}
	tlJSON(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/secrets/TOKEN", `{"value":"two","overwrite_locked":true}`, nil)
	p := pending(t, rt, cookie)
	if len(p.Changes) != 1 || p.Changes[0].Kind != "secret" || p.Changes[0].Keys[0] != "TOKEN" {
		t.Fatalf("rotation pending = %+v", p)
	}

	rt2, db2, cookie2 := newTimelineRouter(t)
	putApp(t, rt2, cookie2, `{"image":"nginx:1","port":80,"env":{"A":"9"}}`)
	svc, _ := db2.GetDesiredService(context.Background(), "web")
	if !svc.EnvDirty {
		t.Fatal("test setup: env_dirty expected")
	}
	if p := pending(t, rt2, cookie2); !p.Pending || p.Changes[0].Kind != "env" {
		t.Fatalf("legacy latch pending = %+v", p)
	}
}

func TestSecretsSetDeclaresKeyAndDeleteUndeclares(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)
	tlJSON(t, rt, cookie, http.MethodPut, "/api/v1/apps/web/secrets/TOKEN", `{"value":"v"}`, nil)
	svc, _ := db.GetDesiredService(context.Background(), "web")
	if got := store.SecretEnvNames(svc.SecretEnv); len(got) != 1 || got[0] != "TOKEN" {
		t.Fatalf("declared = %v", got)
	}
	var app appResource
	tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web", "", &app)
	if len(app.SecretEnv) != 1 {
		t.Fatalf("GET secret_env = %v", app.SecretEnv)
	}

	// An ordinary save must not wipe the declaration.
	putApp(t, rt, cookie, `{"image":"nginx:1","port":80,"env":{"A":"1"}}`)
	svc, _ = db.GetDesiredService(context.Background(), "web")
	if len(svc.SecretEnv) != 1 {
		t.Fatalf("PUT dropped secret_env: %+v", svc.SecretEnv)
	}
	// secret_env round trips and can be cleared explicitly.
	putApp(t, rt, cookie, `{"image":"nginx:1","port":80,"env":{"A":"1"},"secret_env":["TOKEN","OTHER"]}`)
	svc, _ = db.GetDesiredService(context.Background(), "web")
	if got := store.SecretEnvNames(svc.SecretEnv); len(got) != 2 {
		t.Fatalf("PUT secret_env = %v", got)
	}
	if code := tlJSON(t, rt, cookie, http.MethodPut, "/api/v1/apps/web", `{"image":"nginx:1","port":80,"secrets":{"X":"y"}}`, nil); code != http.StatusBadRequest {
		t.Fatalf("PUT with secret values = %d, want 400", code)
	}

	if code := tlJSON(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/secrets/TOKEN", "", nil); code != http.StatusNoContent {
		t.Fatalf("delete = %d", code)
	}
	svc, _ = db.GetDesiredService(context.Background(), "web")
	if got := store.SecretEnvNames(svc.SecretEnv); len(got) != 1 || got[0] != "OTHER" {
		t.Fatalf("after delete = %v", got)
	}
	if ok, _ := db.HasSecretValue(context.Background(), "web", "TOKEN"); ok {
		t.Fatal("value survived delete")
	}
	if code := tlJSON(t, rt, cookie, http.MethodDelete, "/api/v1/apps/web/secrets/NOPE", "", nil); code != http.StatusNotFound {
		t.Fatalf("delete unknown = %d", code)
	}
}

func TestPutAppKeepsFieldsItCannotExpress(t *testing.T) {
	rt, db, cookie := newTimelineRouter(t)
	svc, _ := db.GetDesiredService(context.Background(), "web")
	svc.Volumes = []store.ServiceVolume{{Name: "data", ContainerPath: "/data"}}
	svc.Entrypoint = []string{"/entry"}
	svc.PullPolicy = "always"
	if err := db.SaveDesiredService(context.Background(), *svc); err != nil {
		t.Fatal(err)
	}
	putApp(t, rt, cookie, `{"image":"nginx:1","port":80,"env":{"A":"1"}}`)
	got, _ := db.GetDesiredService(context.Background(), "web")
	if len(got.Volumes) != 1 || len(got.Entrypoint) != 1 || got.PullPolicy != "always" {
		t.Fatalf("PUT reset stored fields: %+v", got)
	}
}

type localOnlyResolver struct{}

func (localOnlyResolver) ResolveImage(_ context.Context, ref string, _ *docker.RegistryAuth, _ bool) (docker.ResolvedImage, error) {
	return docker.ResolvedImage{Ref: ref, LocalID: "sha256:local1", Source: docker.ImageSourceCache, RegistryErr: errors.New("not found")}, nil
}

func TestCreateAppRecordsDigestAndLocalImageReason(t *testing.T) {
	tests := []struct {
		name         string
		resolver     docker.ImageResolver
		wantDigest   string
		wantReason   string
		wantNonBlank bool
	}{
		{"registry digest", stubResolver{digest: "sha256:abc"}, "sha256:abc", store.DigestReasonResolved, false},
		{"local only image", localOnlyResolver{}, "sha256:local1", store.DigestReasonLocalImage, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db, cookie := newSafetyRouter(t, tt.resolver)
			var app appResource
			if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps", `{"name":"fresh","image":"mine:dev","port":80}`, &app); code != http.StatusCreated {
				t.Fatalf("create = %d", code)
			}
			if app.ImageDigest != tt.wantDigest {
				t.Fatalf("response digest = %q, want %q", app.ImageDigest, tt.wantDigest)
			}
			attempts, _ := db.ListDeployAttempts(context.Background(), "fresh")
			if len(attempts) != 1 || attempts[0].ImageDigest != tt.wantDigest || attempts[0].DigestReason != tt.wantReason {
				t.Fatalf("attempts = %+v", attempts)
			}
			if tt.wantNonBlank && !strings.HasPrefix(attempts[0].Reason, "LocalImage") {
				t.Fatalf("reason = %q", attempts[0].Reason)
			}
		})
	}
}
