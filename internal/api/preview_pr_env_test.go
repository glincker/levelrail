package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeVolumeManager struct {
	volumes   map[string]bool
	removed   []string
	removeErr error
}

func newFakeVolumeManager(names ...string) *fakeVolumeManager {
	f := &fakeVolumeManager{volumes: map[string]bool{}}
	for _, n := range names {
		f.volumes[n] = true
	}
	return f
}

func (f *fakeVolumeManager) ListNamedVolumes(context.Context) ([]docker.NamedVolume, error) {
	var out []docker.NamedVolume
	for n := range f.volumes {
		out = append(out, docker.NamedVolume{Name: n})
	}
	return out, nil
}

func (f *fakeVolumeManager) RemoveVolume(_ context.Context, name string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	delete(f.volumes, name)
	f.removed = append(f.removed, name)
	return nil
}

func withPreviewSecrets(t *testing.T, rt *Router) {
	t.Helper()
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	rt.secrets = secrets.NewManager(previewDB(t, rt), mk)
}

func prEnvRequest(t *testing.T, rt *Router, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	cookie := loginTestSession(t, rt, previewDB(t, rt))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestPRPreviewEnvRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/previews/42/env"},
		{http.MethodPost, "/api/v1/apps/web/previews/42/env"},
		{http.MethodDelete, "/api/v1/apps/web/previews/42/env/prenv_x"},
	})
}

func TestPRPreviewEnvAPI(t *testing.T) {
	rt, _, _, _ := setUpPreviewApp(t)
	withPreviewSecrets(t, rt)
	if err := previewDB(t, rt).SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	tests := []struct {
		name, method, path, body string
		wantStatus               int
	}{
		{"bad pr number", http.MethodPost, "/api/v1/apps/web/previews/abc/env", `{"key":"A","value":"1"}`, http.StatusBadRequest},
		{"missing key", http.MethodPost, "/api/v1/apps/web/previews/42/env", `{"value":"1"}`, http.StatusBadRequest},
		{"secret without value", http.MethodPost, "/api/v1/apps/web/previews/42/env", `{"key":"A","secret":true}`, http.StatusBadRequest},
		{"unknown app", http.MethodPost, "/api/v1/apps/ghost/previews/42/env", `{"key":"A","value":"1"}`, http.StatusNotFound},
		{"delete unknown id", http.MethodDelete, "/api/v1/apps/web/previews/42/env/prenv_nope", "", http.StatusNotFound},
		{"plain ok", http.MethodPost, "/api/v1/apps/web/previews/42/env", `{"key":"FLAG","value":"on"}`, http.StatusOK},
		{"secret ok", http.MethodPost, "/api/v1/apps/web/previews/42/env", `{"key":"API_KEY","value":"sk-live-123","secret":true}`, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := prEnvRequest(t, rt, tt.method, tt.path, tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "sk-live-123") {
				t.Errorf("response echoed a secret value: %s", rec.Body.String())
			}
		})
	}

	list := prEnvRequest(t, rt, http.MethodGet, "/api/v1/apps/web/previews/42/env", "")
	if list.Code != http.StatusOK || strings.Contains(list.Body.String(), "sk-live-123") || !strings.Contains(list.Body.String(), `"FLAG"`) {
		t.Fatalf("list = %d %s, want both keys and no secret value", list.Code, list.Body.String())
	}
	other := prEnvRequest(t, rt, http.MethodGet, "/api/v1/apps/web/previews/43/env", "")
	if strings.Contains(other.Body.String(), "FLAG") {
		t.Errorf("PR 43 list leaked PR 42 overrides: %s", other.Body.String())
	}
}

func TestPRPreviewEnv_AppliedAndNeverEchoed(t *testing.T) {
	rt, secret, builder, f := setUpPreviewAppWithGitHubNotifications(t, true)
	withPreviewSecrets(t, rt)
	var logs bytes.Buffer
	rt.logger = slog.New(slog.NewTextHandler(&logs, nil))
	db := previewDB(t, rt)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Env: map[string]string{"FLAG": "off", "KEEP": "me"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	for _, body := range []string{`{"key":"FLAG","value":"on"}`, `{"key":"API_KEY","value":"sk-live-123","secret":true}`} {
		if rec := prEnvRequest(t, rt, http.MethodPost, "/api/v1/apps/web/previews/42/env", body); rec.Code != http.StatusOK {
			t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
		}
	}

	if rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main")); rec.Code != http.StatusOK {
		t.Fatalf("opened: %d %s", rec.Code, rec.Body.String())
	}

	env := builder.calls[0].Service.Env
	if env["FLAG"].Value != "on" || env["API_KEY"].Value != "sk-live-123" || env["KEEP"].Value != "me" {
		t.Errorf("Env = %+v, want FLAG=on, API_KEY decrypted, KEEP inherited", env)
	}
	var comment string
	for _, c := range f.prComments.comments {
		comment += c.Body
	}
	for _, want := range []string{"**Status:** ready", "**Deploy time:**", "**Env overrides:** 2"} {
		if !strings.Contains(comment, want) {
			t.Errorf("comment missing %q:\n%s", want, comment)
		}
	}
	for _, leak := range []string{"sk-live-123", "API_KEY"} {
		if strings.Contains(comment, leak) || strings.Contains(logs.String(), leak) {
			t.Errorf("%q leaked into the PR comment or logs", leak)
		}
	}
}

func TestRenderPreviewComment_RichFields(t *testing.T) {
	p := store.PreviewEnvironment{PreviewAppID: "web-pr-7", Domain: "pr-7.example.com", HeadSHA: "0123456789"}
	x := previewCommentExtras{LogsURL: "https://lr.example.com/apps/web-pr-7/logs", DeployTook: 83_400_000_000, EnvOverrides: 3}
	tests := []struct {
		state string
		want  []string
		deny  []string
	}{
		{previewCommentReady, []string{"**URL:** https://pr-7.example.com", "**Build logs:** https://lr.example.com/apps/web-pr-7/logs", "**Deploy time:** 1m23s", "**Env overrides:** 3"}, nil},
		{previewCommentFailed, []string{"**Deploy time:**", "**Build logs:**"}, []string{"**URL:**"}},
		{previewCommentRemoved, nil, []string{"**Deploy time:**", "**Env overrides:**", "**Build logs:**"}},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := renderPreviewCommentWith(p, tt.state, "", time.Date(2026, 9, 26, 12, 30, 0, 0, time.UTC), x)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q:\n%s", w, got)
				}
			}
			for _, d := range tt.deny {
				if strings.Contains(got, d) {
					t.Errorf("should not contain %q:\n%s", d, got)
				}
			}
		})
	}
}

func TestPreviewCleanup_Idempotent(t *testing.T) {
	setup := func(t *testing.T) (*Router, *store.DB, string, *fakeVolumeManager) {
		rt, db, secret, runtime := setUpPreviewAppWithDatabase(t)
		withPreviewSecrets(t, rt)
		if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		if rec := prEnvRequest(t, rt, http.MethodPost, "/api/v1/apps/web/previews/42/env", `{"key":"API_KEY","value":"sk-live-123","secret":true}`); rec.Code != http.StatusOK {
			t.Fatalf("set: %d", rec.Code)
		}
		sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
		preview, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
		if err != nil {
			t.Fatalf("preview: %v", err)
		}
		tracked, err := db.GetPreviewEphemeralDatabaseByPreviewAndKey(context.Background(), preview.ID, "main")
		if err != nil {
			t.Fatalf("tracked db: %v", err)
		}
		runtime.seed(database.ContainerName(tracked.DatabaseName), true)
		allVols := databaseVolumeNames(tracked.DatabaseName)
		vols := newFakeVolumeManager(append(allVols, "web-pr-42-data", "shared-data")...)
		rt.orphanedVolumes = vols
		// The preview mounts its own volume plus one a production service also mounts.
		if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web-pr-42", Image: "img", Port: 80, Volumes: []store.ServiceVolume{{Name: "web-pr-42-data", ContainerPath: "/d"}, {Name: "shared-data", ContainerPath: "/s"}}}); err != nil {
			t.Fatalf("preview volumes: %v", err)
		}
		if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80, Volumes: []store.ServiceVolume{{Name: "shared-data", ContainerPath: "/s"}}}); err != nil {
			t.Fatalf("prod volumes: %v", err)
		}
		return rt, db, secret, vols
	}
	assertClean := func(t *testing.T, rt *Router, db *store.DB, vols *fakeVolumeManager) {
		t.Helper()
		ctx := context.Background()
		if _, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42); !errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
			t.Errorf("preview row err = %v, want not found", err)
		}
		if o, _ := db.ListPRPreviewEnvOverrides(ctx, "web", 42); len(o) != 0 {
			t.Errorf("overrides left behind: %+v", o)
		}
		if ok, _ := rt.secrets.Exists(ctx, store.PRPreviewEnvSecretsKey("web", 42), "API_KEY"); ok {
			t.Error("override ciphertext left behind")
		}
		if vols.volumes["web-pr-42-data"] {
			t.Error("preview volume left behind")
		}
		if !vols.volumes["shared-data"] {
			t.Error("a volume a remaining service mounts was removed")
		}
		for n := range vols.volumes {
			if strings.HasPrefix(n, "db-web-pr-42-db-") {
				t.Errorf("ephemeral database volume %q left behind", n)
			}
		}
	}

	t.Run("webhook delivered twice", func(t *testing.T) {
		rt, db, secret, vols := setup(t)
		closed := githubPullRequestBody("closed", 42, "sha1", "main")
		for i := 0; i < 2; i++ {
			if rec := sendPullRequestWebhook(rt, secret, closed); rec.Code != http.StatusOK {
				t.Fatalf("delivery %d: %d %s", i+1, rec.Code, rec.Body.String())
			}
		}
		assertClean(t, rt, db, vols)
	})

	t.Run("half succeeded then retried", func(t *testing.T) {
		rt, db, secret, vols := setup(t)
		vols.removeErr = errors.New("volume busy")
		closed := githubPullRequestBody("closed", 42, "sha1", "main")
		if rec := sendPullRequestWebhook(rt, secret, closed); rec.Code != http.StatusMultiStatus {
			t.Fatalf("failing close: %d %s", rec.Code, rec.Body.String())
		}
		if _, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42); err != nil {
			t.Fatalf("preview row must survive a partial teardown for the retry: %v", err)
		}
		vols.removeErr = nil
		if rec := sendPullRequestWebhook(rt, secret, closed); rec.Code != http.StatusOK {
			t.Fatalf("retry: %d %s", rec.Code, rec.Body.String())
		}
		assertClean(t, rt, db, vols)
	})

	t.Run("reopened after close starts clean", func(t *testing.T) {
		rt, db, secret, vols := setup(t)
		sendPullRequestWebhook(rt, secret, githubPullRequestBody("closed", 42, "sha1", "main"))
		if rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("reopened", 42, "sha2", "main")); rec.Code != http.StatusOK {
			t.Fatalf("reopen: %d %s", rec.Code, rec.Body.String())
		}
		p, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
		if err != nil || p.HeadSHA != "sha2" {
			t.Fatalf("reopened preview = %+v, %v, want a fresh row at sha2", p, err)
		}
		if o, _ := db.ListPRPreviewEnvOverrides(context.Background(), "web", 42); len(o) != 0 {
			t.Errorf("reopened PR inherited old overrides: %+v", o)
		}
		_ = vols
	})

	t.Run("close with no preview still clears overrides", func(t *testing.T) {
		rt, db, secret, _ := setUpPreviewApp(t)
		withPreviewSecrets(t, rt)
		if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
			t.Fatalf("seed: %v", err)
		}
		prEnvRequest(t, rt, http.MethodPost, "/api/v1/apps/web/previews/99/env", `{"key":"A","value":"1"}`)
		if rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("closed", 99, "sha1", "main")); rec.Code != http.StatusOK {
			t.Fatalf("close: %d", rec.Code)
		}
		if o, _ := db.ListPRPreviewEnvOverrides(context.Background(), "web", 99); len(o) != 0 {
			t.Errorf("overrides for a PR that never had a preview survived its close: %+v", o)
		}
	})
}
