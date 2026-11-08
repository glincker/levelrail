package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/store"
)

func procfileCheckout(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// pushProcfileRepo seeds an app (image empty means never deployed) and pushes to it.
func pushProcfileRepo(t *testing.T, image string, files map[string]string) (*fakeBuilder, *httptest.ResponseRecorder) {
	t.Helper()
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: image, Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	if err := secrets.SetValue(context.Background(), store.GitSourceSecretsKey("web"), gitSourceSecretKey, "shh"); err != nil {
		t.Fatalf("seed webhook secret: %v", err)
	}
	if err := db.SaveGitSource(context.Background(), store.GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatalf("seed git source: %v", err)
	}
	rt.gitSourceFetch = newFakeGitSourceFetch(&[]fakeGitSourceFetchCall{}, procfileCheckout(t, files), new(bool), nil)
	fb := &fakeBuilder{tag: "img:sha1", multiOutcomes: []deploy.ServiceOutcome{{ServiceKey: "web", ServiceName: "web-web", Image: "img", Warnings: []string{"warning release (release): not run"}}}}
	rt.builder = fb

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("shh"), body))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return fb, rec
}

func TestGitPushProcfileOnlyFansOut(t *testing.T) {
	fb, rec := pushProcfileRepo(t, "", map[string]string{"Procfile": "web: node server.js\nworker: node jobs.js\n"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fb.multiCalls != 1 || fb.calls != 0 {
		t.Fatalf("multiCalls = %d, calls = %d, want the fan-out once", fb.multiCalls, fb.calls)
	}
	if len(fb.lastMultiReq.Services) != 0 || fb.lastMultiReq.SourceDir == "" {
		t.Errorf("request should leave Services empty for DeploySpec to translate: %+v", fb.lastMultiReq)
	}
	if !strings.Contains(rec.Body.String(), "warning: warning release") {
		t.Errorf("translation findings missing from the response: %q", rec.Body.String())
	}
}

func TestGitPushWithoutProcfileKeepsSingleService(t *testing.T) {
	fb, rec := pushProcfileRepo(t, "", map[string]string{"README.md": "x"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fb.multiCalls != 0 || fb.calls != 1 {
		t.Fatalf("multiCalls = %d, calls = %d, want the single-service route", fb.multiCalls, fb.calls)
	}
}

func TestGitPushDeployedAppKeepsSingleServiceDespiteProcfile(t *testing.T) {
	fb, _ := pushProcfileRepo(t, "img:v1", map[string]string{"Procfile": "web: node server.js\n"})
	if fb.multiCalls != 0 || fb.calls != 1 {
		t.Fatalf("multiCalls = %d, calls = %d, want the single-service route", fb.multiCalls, fb.calls)
	}
}

func TestProcfileWebPortSettingDefaultsToTranslator(t *testing.T) {
	rt, _ := newTestRouter(t)
	if got := rt.procfileWebPort(context.Background()); got != 0 && got != 8080 {
		t.Fatalf("procfileWebPort = %d, want 0 or the 8080 default", got)
	}
}

func TestDeploySpecResponseCarriesWarnings(t *testing.T) {
	builder := &fakeBuilder{tag: "img:sha", multiOutcomes: []deploy.ServiceOutcome{{ServiceKey: "web", ServiceName: "myapp-web", Image: "img", Warnings: []string{"warning release (release): not run"}}}}
	rt, db := newTestRouterWithBuilder(t, builder, newFakeFetch("/tmp/checkout", nil))
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/myapp/deploy-spec", multiDeployBody()))
	var resp deploySpecResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one", resp.Warnings)
	}
}
