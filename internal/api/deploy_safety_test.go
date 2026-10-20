package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type stubResolver struct {
	digest string
	err    error
}

func (s stubResolver) ResolveImage(_ context.Context, ref string, _ *docker.RegistryAuth, requireFresh bool) (docker.ResolvedImage, error) {
	if s.err != nil {
		if requireFresh {
			return docker.ResolvedImage{}, s.err
		}
		return docker.ResolvedImage{Ref: ref, Source: docker.ImageSourceNone, RegistryErr: s.err}, nil
	}
	return docker.ResolvedImage{Ref: docker.PinImageRef(ref, s.digest), Digest: s.digest, Source: docker.ImageSourceRegistry}, nil
}

func newSafetyRouter(t *testing.T, resolver docker.ImageResolver) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithDeploySafety(db, resolver))
	cookie := loginTestSession(t, rt, db)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	return rt, db, cookie
}

func serve(rt *Router, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, r)
	return rec
}

func TestTriggerDeployPinsDigestAndReportsIt(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{digest: "sha256:abc"})
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"nginx:latest"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var app appResource
	_ = json.Unmarshal(rec.Body.Bytes(), &app)
	if app.Image != "nginx:latest@sha256:abc" || app.ImageDigest != "sha256:abc" {
		t.Fatalf("app = %+v", app)
	}
	attempts, _ := db.ListDeployAttempts(context.Background(), "web")
	if len(attempts) != 1 || attempts[0].ImageDigest != "sha256:abc" || attempts[0].DigestReason != store.DigestReasonResolved || attempts[0].Sequence == 0 {
		t.Fatalf("attempts = %+v", attempts)
	}
	list := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts", ""))
	if !strings.Contains(list.Body.String(), `"image_digest":"sha256:abc"`) {
		t.Fatalf("deploy-attempts body lacks digest: %s", list.Body.String())
	}
}

func TestTriggerDeployPullFailsWhenRegistryDown(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, stubResolver{err: errors.New("registry down")})
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"nginx:latest","pull":true}`))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	svc, _ := db.GetDesiredService(context.Background(), "web")
	if svc.Image != "nginx:1" {
		t.Fatalf("a failed fresh pull must not change desired state, image = %q", svc.Image)
	}
}

func alwaysFrozen() string {
	return `{"windows":[{"cron":"* * * * *","duration":"2h","reason":"release week"}]}`
}

func TestDeployFreezeBlocksManualDeployUnlessOverridden(t *testing.T) {
	rt, db, cookie := newSafetyRouter(t, nil)
	put := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/deploy-freeze", alwaysFrozen()))
	if put.Code != http.StatusOK || !strings.Contains(put.Body.String(), `"frozen":true`) {
		t.Fatalf("put = %d %s", put.Code, put.Body.String())
	}

	blocked := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"nginx:2"}`))
	if blocked.Code != http.StatusLocked {
		t.Fatalf("frozen deploy status = %d", blocked.Code)
	}
	noReason := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"nginx:2","override_freeze":true}`))
	if noReason.Code != http.StatusBadRequest {
		t.Fatalf("override without reason status = %d", noReason.Code)
	}
	ok := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys", `{"image":"nginx:2","override_freeze":true,"override_reason":"hotfix CVE"}`))
	if ok.Code != http.StatusAccepted {
		t.Fatalf("override status = %d: %s", ok.Code, ok.Body.String())
	}
	attempts, _ := db.ListDeployAttempts(context.Background(), "web")
	if len(attempts) != 1 || attempts[0].Reason != "FreezeOverride: hotfix CVE" {
		t.Fatalf("attempts = %+v", attempts)
	}
}

func TestDeployFreezeValidationAndGlobal(t *testing.T) {
	rt, _, cookie := newSafetyRouter(t, nil)
	bad := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/deploy-freeze", `{"windows":[{"cron":"nope","duration":"1h"}]}`))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("bad cron status = %d", bad.Code)
	}
	global := serve(rt, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/deploy-freeze", alwaysFrozen()))
	if global.Code != http.StatusOK {
		t.Fatalf("global put = %d %s", global.Code, global.Body.String())
	}
	app := serve(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-freeze", ""))
	var res deployFreezeResource
	_ = json.Unmarshal(app.Body.Bytes(), &res)
	if !res.Status.Frozen || len(res.Inherited) != 1 || len(res.Windows) != 0 {
		t.Fatalf("app view = %+v", res)
	}
}

func TestGitPushDuringFreezeIsHeld(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, nil)
	if _, err := db.ReplaceDeployFreezeWindows(context.Background(), store.DeployFreezeScopeApp("web"), []store.DeployFreezeWindow{{Cron: "* * * * *", Duration: 2 * time.Hour}}); err != nil {
		t.Fatal(err)
	}
	rt.builder = nil
	status, msg := rt.processGitPushWebhookPayload(context.Background(), "web", store.GitSource{Branch: "main"}, []byte(`{"ref":"refs/heads/main","before":"aaa","after":"bbb"}`), http.Header{})
	if status != http.StatusAccepted || !strings.HasPrefix(msg, "held:") {
		t.Fatalf("status %d msg %q", status, msg)
	}
	held, _ := db.ListHeldDeployAttempts(context.Background())
	if len(held) != 1 || held[0].CommitSHA != "bbb" || !strings.Contains(held[0].HeldRequest, `"before":"aaa"`) {
		t.Fatalf("held = %+v", held)
	}
}
