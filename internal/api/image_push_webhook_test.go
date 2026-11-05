package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func ghcrBody(action, owner, name, tag, digest string) string {
	return `{"action":"` + action + `","package":{"name":"` + name + `","namespace":"` + owner + `","package_type":"CONTAINER",` +
		`"package_version":{"version":"` + digest + `","container_metadata":{"tag":{"name":"` + tag + `","digest":"` + digest + `"}}}}}`
}

func TestParseGHCRPush(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantRepo   string
		wantIgnore bool
		wantErr    bool
	}{
		{"published tag", ghcrBody("published", "Acme", "Web", "latest", newDigest), "ghcr.io/acme/web", false, false},
		{"deleted is ignored", ghcrBody("deleted", "acme", "web", "latest", newDigest), "", true, false},
		{"untagged is ignored", ghcrBody("published", "acme", "web", "", newDigest), "", true, false},
		{"bad digest", ghcrBody("published", "acme", "web", "latest", "sha256:zz"), "", false, true},
		{"not json", "{", "", false, true},
		{"no package", `{"action":"published"}`, "", false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, ignore, err := parseGHCRPush([]byte(tc.body))
			if (err != nil) != tc.wantErr || (ignore != "") != tc.wantIgnore || p.Repo != tc.wantRepo {
				t.Fatalf("got %+v ignore=%q err=%v", p, ignore, err)
			}
			if tc.wantRepo != "" && (p.Digest != newDigest || p.Tag != "latest") {
				t.Errorf("push = %+v", p)
			}
		})
	}
}

func TestParseDockerHubPush(t *testing.T) {
	p, ok := parseDockerHubPush([]byte(`{"push_data":{"tag":"1"},"repository":{"repo_name":"acme/web"}}`))
	if !ok || p.Repo != "docker.io/acme/web" || p.Tag != "1" || p.Digest != "" {
		t.Fatalf("push = %+v ok=%v", p, ok)
	}
	p, ok = parseDockerHubPush([]byte(`{"push_data":{"tag":"1"},"repository":{"repo_name":"nginx"}}`))
	if !ok || p.Repo != "docker.io/library/nginx" {
		t.Fatalf("official image = %+v ok=%v", p, ok)
	}
	if _, ok = parseDockerHubPush([]byte(`{"push_data":{},"repository":{}}`)); ok {
		t.Error("empty payload must not parse")
	}
}

func TestVerifyGitHubSignature(t *testing.T) {
	body := []byte(`{"a":1}`)
	mac := hmac.New(sha256.New, []byte("s3"))
	mac.Write(body)
	good := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if !verifyGitHubSignature("s3", good, body) {
		t.Error("valid signature rejected")
	}
	for _, bad := range []string{"", "sha256=", good[:len(good)-2] + "00", "sha1=" + good[7:]} {
		if verifyGitHubSignature("s3", bad, body) {
			t.Errorf("signature %q accepted", bad)
		}
	}
	if verifyGitHubSignature("other", good, body) {
		t.Error("wrong secret accepted")
	}
}

type pushHarness struct {
	rt     *Router
	db     *store.DB
	cookie *http.Cookie
	secret *fakeGitSourceSecrets
	path   string
	hook   imageUpdateWebhookResource
}

func newPushHarness(t *testing.T, image string) *pushHarness {
	t.Helper()
	db := openTestDB(t)
	secrets := newFakeGitSourceSecrets()
	rt := NewRouter(discardLogger(), testBrand(), db, WithDeploySafety(db, stubResolver{digest: newDigest}), WithGitSourceSecrets(secrets))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: image, Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetImageAutoUpdate(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/auto-update/webhook", ""))
	h := &pushHarness{rt: rt, db: db, cookie: cookie, secret: secrets}
	if err := json.Unmarshal(rec.Body.Bytes(), &h.hook); err != nil || h.hook.Token == "" {
		t.Fatalf("rotate = %s, %v", rec.Body.String(), err)
	}
	h.path = h.hook.Path
	return h
}

func (h *pushHarness) post(body string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, h.path, bytes.NewReader([]byte(body)))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return serve(h.rt, req)
}

func (h *pushHarness) signed(body string) map[string]string {
	mac := hmac.New(sha256.New, []byte(h.hook.SigningSecret))
	mac.Write([]byte(body))
	return map[string]string{headerGitHubEvent: "package", headerGitHubSig256: "sha256=" + hex.EncodeToString(mac.Sum(nil))}
}

func (h *pushHarness) waitImage(t *testing.T, want string) store.DesiredService {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		svc, err := h.db.GetDesiredService(context.Background(), "web")
		if err != nil {
			t.Fatal(err)
		}
		if svc.Image == want || time.Now().After(deadline) {
			return *svc
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestRotateReturnsSigningSecretOnce(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	if h.hook.SigningSecret == "" {
		t.Fatal("rotate must return a signing secret when secrets are configured")
	}
	rec := serve(h.rt, authedRequest(t, h.cookie, http.MethodGet, "/api/v1/apps/web/auto-update", ""))
	var res imageAutoUpdateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || !res.HasSigningSecret {
		t.Fatalf("get = %s, %v", rec.Body.String(), err)
	}
	if strings.Contains(rec.Body.String(), h.hook.SigningSecret) {
		t.Error("GET leaked the signing secret")
	}
}

func TestGHCRPushDeploysPinnedDigest(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body, h.signed(body)); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	svc := h.waitImage(t, "ghcr.io/acme/web:latest@"+newDigest)
	if docker.ImageDigestOf(svc.Image) != newDigest {
		t.Fatalf("image = %q, want pinned to the pushed digest", svc.Image)
	}
	var got []store.AuditEntry
	deadline := time.Now().Add(5 * time.Second)
	for len(got) == 0 && time.Now().Before(deadline) {
		got, _ = h.db.ListAuditEntries(context.Background(), 5, nil, store.AuditEntryFilter{Action: AuditActionRegistryPushDeploy})
		time.Sleep(20 * time.Millisecond)
	}
	if len(got) != 1 || !strings.Contains(got[0].Detail, newDigest) || !strings.Contains(got[0].Detail, `"tag":"latest"`) {
		t.Fatalf("audit rows = %+v", got)
	}
	if strings.Contains(got[0].Path, h.hook.Token) {
		t.Error("audit path leaked the URL token")
	}
}

func TestGHCRPushRejections(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body, map[string]string{headerGitHubEvent: "package"}); rec.Code != http.StatusNotFound {
		t.Errorf("unsigned with secret set = %d, want 404", rec.Code)
	}
	other := ghcrBody("published", "acme", "other", "latest", newDigest)
	if rec := h.post(other, h.signed(other)); rec.Code != http.StatusNotFound {
		t.Errorf("foreign repository = %d, want 404", rec.Code)
	}
	wrongTag := ghcrBody("published", "acme", "web", "edge", newDigest)
	rec := h.post(wrongTag, h.signed(wrongTag))
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "ignored") {
		t.Errorf("other tag = %d %s, want ignored 202", rec.Code, rec.Body.String())
	}
	ping := h.signed(`{}`)
	ping[headerGitHubEvent] = "ping"
	if rec := h.post(`{}`, ping); rec.Code != http.StatusOK {
		t.Errorf("ping = %d, want 200", rec.Code)
	}
	svc := h.waitImage(t, "never")
	if docker.ImageDigestOf(svc.Image) != oldDigest {
		t.Errorf("rejected deliveries changed the image to %q", svc.Image)
	}
}

func TestDockerHubPushResolvesDigestAndRequiresRepoMatch(t *testing.T) {
	h := newPushHarness(t, "acme/web:1@"+oldDigest)
	if rec := h.post(`{"push_data":{"tag":"1"},"repository":{"repo_name":"evil/web"}}`, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign repo = %d, want 404", rec.Code)
	}
	if rec := h.post(`{"push_data":{"tag":"1"},"repository":{"repo_name":"acme/web"}}`, nil); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	svc := h.waitImage(t, "acme/web:1@"+newDigest)
	if svc.Image != "acme/web:1@"+newDigest {
		t.Fatalf("image = %q, want the resolved digest pinned", svc.Image)
	}
}

func TestRegistryPushToProtectedEnvironmentFilesApproval(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	ctx := context.Background()
	if err := h.db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "p", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := h.db.SaveEnvironment(ctx, store.Environment{ID: "env_prod", ProjectID: "proj_1", Name: "production", Protected: true, CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := h.db.SetServiceEnvironment(ctx, "web", "env_prod"); err != nil {
		t.Fatal(err)
	}
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	for i := 0; i < 2; i++ {
		if rec := h.post(body, h.signed(body)); rec.Code != http.StatusAccepted {
			t.Fatalf("delivery %d = %d", i, rec.Code)
		}
		time.Sleep(300 * time.Millisecond)
	}
	pending, err := h.db.ListDeployApprovals(ctx, store.DeployApprovalStatusPending, "web")
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending approvals = %+v, %v; want exactly one despite the redelivery", pending, err)
	}
	if pending[0].Image != "ghcr.io/acme/web:latest@"+newDigest || pending[0].RequestedByType != actorTypeRegistryPush {
		t.Errorf("approval = %+v", pending[0])
	}
	svc, _ := h.db.GetDesiredService(ctx, "web")
	if docker.ImageDigestOf(svc.Image) != oldDigest {
		t.Errorf("protected app deployed silently: %q", svc.Image)
	}
}

func (h *pushHarness) lastAuditDetail(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rows, _ := h.db.ListAuditEntries(context.Background(), 5, nil, store.AuditEntryFilter{Action: AuditActionRegistryPushDeploy})
		if len(rows) > 0 {
			return rows[0].Detail
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no registry push audit row")
	return ""
}

func TestForgedPayloadDigestIsNotDeployed(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	forged := "sha256:" + strings.Repeat("3", 64)
	body := ghcrBody("published", "acme", "web", "latest", forged)
	if rec := h.post(body, h.signed(body)); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	detail := h.lastAuditDetail(t)
	if !strings.Contains(detail, forged) || !strings.Contains(detail, "not what the registry serves") {
		t.Errorf("audit detail = %s", detail)
	}
	svc, _ := h.db.GetDesiredService(context.Background(), "web")
	if docker.ImageDigestOf(svc.Image) != oldDigest {
		t.Errorf("forged digest changed the image to %q", svc.Image)
	}
}

func TestHeaderlessBodyCannotSpoofAnotherRegistry(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	rec := h.post(`{"push_data":{"tag":"latest"},"repository":{"repo_name":"ghcr.io/acme/web"}}`, nil)
	if strings.Contains(rec.Body.String(), "deploy started") {
		t.Fatalf("headerless ghcr.io body was treated as a push: %d %s", rec.Code, rec.Body.String())
	}
	time.Sleep(200 * time.Millisecond)
	rows, _ := h.db.ListAuditEntries(context.Background(), 5, nil, store.AuditEntryFilter{Action: AuditActionRegistryPushDeploy})
	if len(rows) != 0 {
		t.Errorf("a push audit row exists for a spoofed delivery: %+v", rows)
	}
}

func (h *pushHarness) protect(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if err := h.db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "p", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := h.db.SaveEnvironment(ctx, store.Environment{ID: "env_prod", ProjectID: "proj_1", Name: "production", Protected: true, CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := h.db.SetServiceEnvironment(ctx, "web", "env_prod"); err != nil {
		t.Fatal(err)
	}
}

func TestRotatorCannotApproveThePushApproval(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	h.protect(t)
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body, h.signed(body)); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	var pending []store.DeployApproval
	deadline := time.Now().Add(5 * time.Second)
	for len(pending) == 0 && time.Now().Before(deadline) {
		pending, _ = h.db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web")
		time.Sleep(20 * time.Millisecond)
	}
	if len(pending) != 1 {
		t.Fatalf("pending = %d", len(pending))
	}
	rec := serve(h.rt, authedRequest(t, h.cookie, http.MethodPost, "/api/v1/deploy-approvals/"+pending[0].ID+"/approve", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("approve by the person who rotated the hook = %d %s, want 403", rec.Code, rec.Body.String())
	}
}

func TestConcurrentPushesFileOneApproval(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	h.protect(t)
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.post(body, h.signed(body))
		}()
	}
	wg.Wait()
	time.Sleep(500 * time.Millisecond)
	pending, err := h.db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web")
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending approvals = %d, %v; want exactly one", len(pending), err)
	}
}

func TestPushOutsideUpdateWindowWaits(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	ctx := context.Background()
	if err := h.db.SetImageAutoUpdateWindow(ctx, "web", "0 3 * * *", 1); err != nil {
		t.Fatal(err)
	}
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	rec := h.post(body, h.signed(body))
	if rec.Code != http.StatusAccepted || !strings.Contains(rec.Body.String(), "update window") {
		t.Fatalf("response = %d %s", rec.Code, rec.Body.String())
	}
	if u, _ := h.db.GetImageAutoUpdate(ctx, "web"); u.LastResult != imageUpdateWaiting {
		t.Errorf("last result = %q", u.LastResult)
	}
}
