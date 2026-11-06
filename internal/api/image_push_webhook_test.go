package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
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

type mutableResolver struct {
	mu     sync.Mutex
	digest string
	// gate, when set, holds every resolution until it is closed or sent to.
	gate chan struct{}
	// entered is signaled once per resolution that reads the digest, before the gate.
	entered chan struct{}
	calls   []time.Time
}

func (m *mutableResolver) set(d string) {
	m.mu.Lock()
	m.digest = d
	m.mu.Unlock()
}

func (m *mutableResolver) ResolveImage(ctx context.Context, ref string, auth *docker.RegistryAuth, fresh bool) (docker.ResolvedImage, error) {
	m.mu.Lock()
	d, gate, entered := m.digest, m.gate, m.entered
	m.calls = append(m.calls, time.Now())
	m.mu.Unlock()
	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
		}
	}
	return stubResolver{digest: d}.ResolveImage(ctx, ref, auth, fresh)
}

type pushHarness struct {
	resolver *mutableResolver
	rt       *Router
	db       *store.DB
	cookie   *http.Cookie
	secret   *fakeGitSourceSecrets
	path     string
	hook     imageUpdateWebhookResource
}

func newPushHarness(t *testing.T, image string) *pushHarness {
	t.Helper()
	return newPushHarnessWith(t, image, true)
}

// newPushHarnessWith with rotate=false leaves a hook that was set up before signing secrets and rotators existed.
func newPushHarnessWith(t *testing.T, image string, rotate bool) *pushHarness {
	t.Helper()
	db := openTestDB(t)
	secrets := newFakeGitSourceSecrets()
	resolver := &mutableResolver{digest: newDigest}
	rt := NewRouter(discardLogger(), testBrand(), db, WithDeploySafety(db, resolver), WithGitSourceSecrets(secrets))
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: image, Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetImageAutoUpdate(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	h := &pushHarness{rt: rt, db: db, cookie: cookie, secret: secrets, resolver: resolver}
	if !rotate {
		if err := db.SetImageAutoUpdateWebhookHash(ctx, "web", hashToken("legacy")); err != nil {
			t.Fatal(err)
		}
		h.path = imageUpdateHookPrefix + "web/legacy"
		return h
	}
	h.rotate(t, cookie, "")
	return h
}

func (h *pushHarness) rotate(t *testing.T, cookie *http.Cookie, bearer string) {
	t.Helper()
	var req *http.Request
	if bearer != "" {
		req = httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/auto-update/webhook", nil)
		req.Header.Set("Authorization", "Bearer "+bearer)
	} else {
		req = authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/auto-update/webhook", "")
	}
	rec := serve(h.rt, req)
	h.hook = imageUpdateWebhookResource{}
	if err := json.Unmarshal(rec.Body.Bytes(), &h.hook); err != nil || h.hook.Token == "" {
		t.Fatalf("rotate = %d %s, %v", rec.Code, rec.Body.String(), err)
	}
	h.path = h.hook.Path
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

func (h *pushHarness) waitIdle(t *testing.T) {
	t.Helper()
	slot := h.rt.registryPushSlot("web")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		slot.mu.Lock()
		idle := !slot.running && slot.next == nil
		slot.mu.Unlock()
		if idle {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("push slot never went idle")
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
	h.waitIdle(t)
	pending, err := h.db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web")
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending approvals = %d, %v; want exactly one (a coalesced rerun must not file a second)", len(pending), err)
	}
}

func TestOwnerlessRotationClearsTheRecordedRotator(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	h.protect(t)
	plain := "ownerless-ci-secret"
	if err := h.db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_ci", Name: "ci", TokenHash: hashToken(plain), Abilities: []string{AbilityRead, AbilityWriteSensitive}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	h.rotate(t, nil, plain)
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body, h.signed(body)); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	h.waitIdle(t)
	if pending, _ := h.db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web"); len(pending) != 0 {
		t.Fatalf("an approval naming the earlier person was filed after an ownerless rotation: %+v", pending)
	}
	if u, _ := h.db.GetImageAutoUpdate(context.Background(), "web"); !strings.Contains(u.LastResult, "rotate the registry webhook once") {
		t.Errorf("last result = %q", u.LastResult)
	}
}

func TestHookRotatedBeforeThisChangeFailsClosedInProtectedEnvironment(t *testing.T) {
	h := newPushHarnessWith(t, "ghcr.io/acme/web:latest@"+oldDigest, false)
	h.protect(t)
	if err := h.db.SetImageAutoUpdate(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body, map[string]string{headerGitHubEvent: "package"}); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
	}
	h.waitIdle(t)
	if pending, _ := h.db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web"); len(pending) != 0 {
		t.Fatalf("approval filed for a hook with no recorded rotator: %+v", pending)
	}
	if u, _ := h.db.GetImageAutoUpdate(context.Background(), "web"); !strings.Contains(u.LastResult, "rotate the registry webhook once") {
		t.Errorf("last result = %q", u.LastResult)
	}
}

func TestNewerPushSupersedesOlderPendingApproval(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	h.protect(t)
	second := "sha256:" + strings.Repeat("4", 64)
	for _, d := range []string{newDigest, second} {
		h.resolver.set(d)
		body := ghcrBody("published", "acme", "web", "latest", d)
		if rec := h.post(body, h.signed(body)); rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d", rec.Code)
		}
		h.waitIdle(t)
		time.Sleep(50 * time.Millisecond)
	}
	pending, _ := h.db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web")
	if len(pending) != 1 || !strings.HasSuffix(pending[0].Image, second) {
		t.Fatalf("pending approvals = %+v, want only the newest digest", pending)
	}
}

func TestStagedSecretIsAcceptedUntilRotationFinishes(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	if err := h.secret.SetValue(context.Background(), imageUpdateSecretNS+"web", imageUpdateSecretPendingKey, "staged"); err != nil {
		t.Fatal(err)
	}
	body := ghcrBody("published", "acme", "web", "other", newDigest)
	mac := hmac.New(sha256.New, []byte("staged"))
	mac.Write([]byte(body))
	sig := map[string]string{headerGitHubEvent: "package", headerGitHubSig256: "sha256=" + hex.EncodeToString(mac.Sum(nil))}
	if rec := h.post(body, sig); rec.Code != http.StatusAccepted {
		t.Errorf("staged secret = %d, want accepted", rec.Code)
	}
	sig[headerGitHubSig256] = "sha256=" + strings.Repeat("0", 64)
	if rec := h.post(body, sig); rec.Code != http.StatusNotFound {
		t.Errorf("bad signature = %d, want 404", rec.Code)
	}
}

func TestRepeatedSignedDigestIsDebounced(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body, h.signed(body)); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	h.waitIdle(t)
	rec := h.post(body, h.signed(body))
	if !strings.Contains(rec.Body.String(), "checked recently") {
		t.Fatalf("replayed delivery = %s, want debounced", rec.Body.String())
	}
}

func TestAutoUpdateFailsClosedWhenEnvironmentLookupFails(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "nginx:1@" + oldDigest, Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "p", CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveEnvironment(ctx, store.Environment{ID: "env_prod", ProjectID: "proj_1", Name: "production", Protected: true, CreatedAt: "2026-08-20T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetServiceEnvironment(ctx, "web", "env_prod"); err != nil {
		t.Fatal(err)
	}
	rt.environments = failingEnvironmentLookup{EnvironmentStore: rt.environments, failID: "env_prod"}
	got, err := rt.CheckImageUpdate(ctx, "web")
	if err != nil || !strings.Contains(got, "could not check whether the environment is protected") {
		t.Fatalf("result = %q, %v", got, err)
	}
	svc, _ := db.GetDesiredService(ctx, "web")
	if docker.ImageDigestOf(svc.Image) != oldDigest {
		t.Errorf("app redeployed to %q despite the failed protection lookup", svc.Image)
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

// waitPendingImages waits for exactly one pending approval and returns its image.
func (h *pushHarness) waitPendingImages(t *testing.T) []string {
	t.Helper()
	var images []string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pending, _ := h.db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web")
		images = images[:0]
		for _, a := range pending {
			images = append(images, a.Image)
		}
		if len(images) == 1 {
			return images
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("pending approvals = %v, want exactly one", images)
	return nil
}

func TestQueuedPushRunsAfterTheCurrentOneAndIsAuditedAsItsOwnDelivery(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	h.protect(t)
	second := "sha256:" + strings.Repeat("4", 64)
	gate, entered := make(chan struct{}), make(chan struct{}, 4)
	h.resolver.mu.Lock()
	h.resolver.gate, h.resolver.entered = gate, entered
	h.resolver.mu.Unlock()

	body1 := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body1, h.signed(body1)); !strings.Contains(rec.Body.String(), "deploy started") {
		t.Fatalf("first = %s", rec.Body.String())
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("first run never reached the registry")
	}
	h.resolver.set(second)
	body2 := ghcrBody("published", "acme", "web", "latest", second)
	req := httptest.NewRequest(http.MethodPost, h.path, bytes.NewReader([]byte(body2)))
	for k, v := range h.signed(body2) {
		req.Header.Set(k, v)
	}
	req.RemoteAddr = "203.0.113.9:4000"
	if rec := serve(h.rt, req); !strings.Contains(rec.Body.String(), "queued") {
		t.Fatalf("second = %s", rec.Body.String())
	}
	close(gate)
	h.waitIdle(t)

	images := h.waitPendingImages(t)
	if !strings.HasSuffix(images[0], second) {
		t.Fatalf("pending = %v, want the queued push's digest (the newest)", images)
	}
	rows, _ := h.db.ListAuditEntries(context.Background(), 10, nil, store.AuditEntryFilter{Action: AuditActionRegistryPushDeploy})
	var sawQueued bool
	for _, r := range rows {
		if strings.Contains(r.Detail, second) && strings.HasPrefix(r.RemoteAddr, "203.0.113.9") {
			sawQueued = true
		}
	}
	if !sawQueued {
		t.Errorf("no audit row names the queued delivery's address: %+v", rows)
	}
	sup, _ := h.db.ListAuditEntries(context.Background(), 10, nil, store.AuditEntryFilter{Action: AuditActionRegistryPushSupersede})
	if len(sup) != 1 || !strings.Contains(sup[0].Detail, "approval_id") {
		t.Errorf("supersede audit rows = %+v", sup)
	}
}

// withHook stamps a hand-built push with the URL hash the harness hook is live on, as intake does.
func withHook(h *pushHarness, p *registryPush) *registryPush {
	p.hookHash = hashToken(h.hook.Token)
	return p
}

func TestQueuedRunRechecksTheAppBeforeDeploying(t *testing.T) {
	newReq := func(h *pushHarness) *http.Request {
		return httptest.NewRequest(http.MethodPost, h.path, nil).WithContext(withRegistryPushActor(context.Background(), registryProviderGHCR, "web"))
	}
	push := func() *registryPush {
		return &registryPush{Provider: registryProviderGHCR, Repo: "ghcr.io/acme/web", Tag: "latest", Digest: newDigest, signed: true}
	}
	t.Run("stopped", func(t *testing.T) {
		h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
		if err := h.db.UpdateServiceSuspended(context.Background(), "web", true); err != nil {
			t.Fatal(err)
		}
		if out, _ := h.rt.runRegistryPushOnce(newReq(h), "web", withHook(h, push())); !strings.Contains(out, "app is stopped") {
			t.Errorf("outcome = %q", out)
		}
	})
	t.Run("tag changed", func(t *testing.T) {
		h := newPushHarness(t, "ghcr.io/acme/web:stable@"+oldDigest)
		if out, _ := h.rt.runRegistryPushOnce(newReq(h), "web", withHook(h, push())); !strings.Contains(out, "not the one this app follows") {
			t.Errorf("outcome = %q", out)
		}
	})
	t.Run("window closed", func(t *testing.T) {
		h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
		if err := h.db.SetImageAutoUpdateWindow(context.Background(), "web", "0 3 * * *", 1); err != nil {
			t.Fatal(err)
		}
		if out, _ := h.rt.runRegistryPushOnce(newReq(h), "web", withHook(h, push())); !strings.Contains(out, "update window") {
			t.Errorf("outcome = %q", out)
		}
	})
	t.Run("store error fails closed", func(t *testing.T) {
		h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
		h.rt.imageAutoUpdates = failingAutoUpdateGet{h.rt.imageAutoUpdates}
		if out, _ := h.rt.runRegistryPushOnce(newReq(h), "web", withHook(h, push())); !strings.Contains(out, "could not check the update window") {
			t.Errorf("outcome = %q", out)
		}
	})
	t.Run("nothing deployed", func(t *testing.T) {
		h := newPushHarness(t, "ghcr.io/acme/web:stable@"+oldDigest)
		h.rt.runRegistryPushOnce(newReq(h), "web", withHook(h, push()))
		if svc, _ := h.db.GetDesiredService(context.Background(), "web"); docker.ImageDigestOf(svc.Image) != oldDigest {
			t.Errorf("image = %q", svc.Image)
		}
	})
}

type failingAutoUpdateGet struct{ ImageAutoUpdateStore }

func (failingAutoUpdateGet) GetImageAutoUpdate(context.Context, string) (store.ImageAutoUpdate, error) {
	return store.ImageAutoUpdate{}, errors.New("injected failure")
}

type failingHashSave struct{ ImageAutoUpdateStore }

func (failingHashSave) SetImageAutoUpdateWebhookHash(context.Context, string, string) error {
	return errors.New("injected failure")
}

type failingMainSecret struct{ GitSourceSecrets }

func (f failingMainSecret) SetValue(ctx context.Context, ns, key, v string) error {
	if key == imageUpdateSecretKey {
		return errors.New("injected failure")
	}
	return f.GitSourceSecrets.SetValue(ctx, ns, key, v)
}

func TestBackToBackUnsignedPushesBothReachTheAppButAreSpaced(t *testing.T) {
	h := newPushHarness(t, "acme/web:1@"+oldDigest)
	h.protect(t)
	if _, err := h.rt.runtimeSettings.Set(context.Background(), runtimesettings.KeyDeploysRegistryPushDebounce, json.RawMessage(`"2s"`), "test"); err != nil {
		t.Fatal(err)
	}
	second := "sha256:" + strings.Repeat("5", 64)
	hub := `{"push_data":{"tag":"1"},"repository":{"repo_name":"acme/web"}}`
	if rec := h.post(hub, nil); rec.Code != http.StatusAccepted {
		t.Fatalf("first = %d", rec.Code)
	}
	h.waitIdle(t)
	h.resolver.set(second)
	if rec := h.post(hub, nil); strings.Contains(rec.Body.String(), "ignored") {
		t.Fatalf("second push was dropped: %s", rec.Body.String())
	}
	h.waitIdle(t)
	images := h.waitPendingImages(t)
	if !strings.HasSuffix(images[0], second) {
		t.Fatalf("pending = %v, want the second push's digest", images)
	}
	h.resolver.mu.Lock()
	calls := append([]time.Time(nil), h.resolver.calls...)
	h.resolver.mu.Unlock()
	if len(calls) < 2 || calls[1].Sub(calls[0]) < 900*time.Millisecond {
		t.Fatalf("registry lookups were not spaced by the debounce: %v", calls)
	}
}

func TestRotationFailureLeavesTheWorkingHookIntact(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:other@"+oldDigest)
	oldPath, oldSecret := h.path, h.hook.SigningSecret
	h.rt.imageAutoUpdates = failingHashSave{h.rt.imageAutoUpdates}
	rec := serve(h.rt, authedRequest(t, h.cookie, http.MethodPost, "/api/v1/apps/web/auto-update/webhook", ""))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("rotate = %d, want 500", rec.Code)
	}
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	mac := hmac.New(sha256.New, []byte(oldSecret))
	mac.Write([]byte(body))
	req := httptest.NewRequest(http.MethodPost, oldPath, bytes.NewReader([]byte(body)))
	req.Header.Set(headerGitHubEvent, "package")
	req.Header.Set(headerGitHubSig256, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	if got := serve(h.rt, req); got.Code != http.StatusAccepted {
		t.Errorf("old URL and secret after a failed rotation = %d, want 202", got.Code)
	}
}

func TestFailedPromotionStillReturnsTheNewHook(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:other@"+oldDigest)
	oldSecret := h.hook.SigningSecret
	h.rt.gitSourceSecrets = failingMainSecret{h.secret}
	h.rotate(t, h.cookie, "")
	if h.hook.SigningSecret == "" || h.hook.SigningSecret == oldSecret {
		t.Fatalf("rotate must return the new secret even when promotion fails, got %q", h.hook.SigningSecret)
	}
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body, h.signed(body)); rec.Code != http.StatusAccepted {
		t.Errorf("new secret = %d, want accepted through the staged key", rec.Code)
	}
}

// blockingHashSave holds the first hash save until released, so a second rotation can race it.
type blockingHashSave struct {
	ImageAutoUpdateStore
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (b *blockingHashSave) SetImageAutoUpdateWebhookHash(ctx context.Context, name, hash string) error {
	if b.calls.Add(1) == 1 {
		close(b.started)
		<-b.release
	}
	return b.ImageAutoUpdateStore.SetImageAutoUpdateWebhookHash(ctx, name, hash)
}

func TestConcurrentRotationsRecordTheLiveRotator(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	ctx := context.Background()
	bob := storeUserWithAbilitiesForTest(t, h.db, "bob@example.com", []string{AbilityRead, AbilityWriteSensitive})
	if err := h.db.SaveAPIToken(ctx, store.APIToken{ID: "tok_bob", Name: "bob", TokenHash: hashToken("bob-secret"), Abilities: []string{AbilityRead, AbilityWriteSensitive}, CreatedAt: time.Now(), OwnerUserID: bob.ID}); err != nil {
		t.Fatal(err)
	}
	blocker := &blockingHashSave{ImageAutoUpdateStore: h.rt.imageAutoUpdates, started: make(chan struct{}), release: make(chan struct{})}
	h.rt.imageAutoUpdates = blocker
	aliceReq := authedRequest(t, h.cookie, http.MethodPost, "/api/v1/apps/web/auto-update/webhook", "")
	bobReq := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/auto-update/webhook", nil)
	bobReq.Header.Set("Authorization", "Bearer bob-secret")
	alice, _ := h.rt.currentSessionUserID(aliceReq)

	type result struct{ path string }
	aliceDone, bobDone := make(chan result, 1), make(chan result, 1)
	go func() {
		var res imageUpdateWebhookResource
		_ = json.Unmarshal(serve(h.rt, aliceReq).Body.Bytes(), &res)
		aliceDone <- result{res.Path}
	}()
	<-blocker.started
	go func() {
		var res imageUpdateWebhookResource
		_ = json.Unmarshal(serve(h.rt, bobReq).Body.Bytes(), &res)
		bobDone <- result{res.Path}
	}()
	time.Sleep(200 * time.Millisecond)
	close(blocker.release)
	paths := []string{(<-aliceDone).path, (<-bobDone).path}

	live := -1
	for i, p := range paths {
		if p != "" && serve(h.rt, httptest.NewRequest(http.MethodPost, p, nil)).Code == http.StatusAccepted {
			live = i
		}
	}
	if live < 0 {
		t.Fatalf("no returned URL is live: %v", paths)
	}
	rotator, err := h.rt.pushRotator(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	want := alice
	if live == 1 {
		want = bob.ID
	}
	if rotator != want {
		t.Fatalf("rotated_by = %q, but the live URL belongs to %q", rotator, want)
	}
}

func (h *pushHarness) bobRotates(t *testing.T) *http.Request {
	t.Helper()
	bob := storeUserWithAbilitiesForTest(t, h.db, "bob2@example.com", []string{AbilityRead, AbilityWrite, AbilityWriteSensitive, AbilityDeploy})
	if err := h.db.SaveAPIToken(context.Background(), store.APIToken{ID: "tok_bob2", Name: "bob2", TokenHash: hashToken("bob2-secret"), Abilities: []string{AbilityRead, AbilityWrite, AbilityWriteSensitive, AbilityDeploy}, CreatedAt: time.Now(), OwnerUserID: bob.ID}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/auto-update/webhook", nil)
	req.Header.Set("Authorization", "Bearer bob2-secret")
	return req
}

func TestFailedHashSaveRestoresTheRotatorAndTheSecretSlot(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	before, err := h.rt.pushRotator(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	req := h.bobRotates(t)
	h.rt.imageAutoUpdates = failingHashSave{h.rt.imageAutoUpdates}
	if rec := serve(h.rt, req); rec.Code != http.StatusInternalServerError {
		t.Fatalf("rotate = %d, want 500", rec.Code)
	}
	after, err := h.rt.pushRotator(context.Background(), "web")
	if err != nil || after != before {
		t.Fatalf("rotator after a failed rotation = %q, %v; want the live hook's %q", after, err, before)
	}
	ctx := context.Background()
	if got, _ := h.secret.Resolve(ctx, imageUpdateSecretNS+"web", imageUpdateSecretKey); got != h.hook.SigningSecret {
		t.Errorf("live signing secret changed to %q", got)
	}
	if got, _ := h.secret.Resolve(ctx, imageUpdateSecretNS+"web", imageUpdateSecretPendingKey); got != pushPendingNone {
		t.Errorf("staged slot = %q after a failed rotation, want it empty", got)
	}
}

func TestFailedRotationKeepsASecretThatIsStillStaged(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:other@"+oldDigest)
	// Rotation N: the URL is saved but promotion fails, so the secret the user configured is only staged.
	h.rt.gitSourceSecrets = failingMainSecret{h.secret}
	h.rotate(t, h.cookie, "")
	staged := h.hook.SigningSecret
	// Rotation N+1 fails at its hash save: the staged secret of rotation N must survive the rollback.
	h.rt.imageAutoUpdates = failingHashSave{h.rt.imageAutoUpdates}
	if rec := serve(h.rt, authedRequest(t, h.cookie, http.MethodPost, "/api/v1/apps/web/auto-update/webhook", "")); rec.Code != http.StatusInternalServerError {
		t.Fatalf("second rotate = %d, want 500", rec.Code)
	}
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	mac := hmac.New(sha256.New, []byte(staged))
	mac.Write([]byte(body))
	req := httptest.NewRequest(http.MethodPost, h.path, bytes.NewReader([]byte(body)))
	req.Header.Set(headerGitHubEvent, "package")
	req.Header.Set(headerGitHubSig256, "sha256="+hex.EncodeToString(mac.Sum(nil)))
	if got := serve(h.rt, req); got.Code != http.StatusAccepted {
		t.Fatalf("delivery signed with the still-staged secret = %d, want 202", got.Code)
	}
}

func TestApprovalKeepsItsFilingRequesterAcrossARotation(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	h.protect(t)
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body, h.signed(body)); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	h.waitIdle(t)
	h.waitPendingImages(t)
	pending, _ := h.db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web")
	if len(pending) != 1 || !pending[0].RequesterRecorded || pending[0].RequesterUserID == "" {
		t.Fatalf("the requester must be stored on the approval at filing time: %+v", pending)
	}
	// Someone else rotates the hook; the approval still names the person who set the hook up when it was filed.
	bobReq := h.bobRotates(t)
	if rec := serve(h.rt, bobReq); rec.Code != http.StatusOK {
		t.Fatalf("bob rotate = %d", rec.Code)
	}
	rec := serve(h.rt, authedRequest(t, h.cookie, http.MethodPost, "/api/v1/deploy-approvals/"+pending[0].ID+"/approve", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("the person who rotated before filing approved it after another rotation = %d, want 403", rec.Code)
	}
}

func TestFailedRotationDoesNotForceSignaturesOnAnUnsignedHook(t *testing.T) {
	h := newPushHarnessWith(t, "ghcr.io/acme/web:other@"+oldDigest, false)
	h.rt.imageAutoUpdates = failingHashSave{h.rt.imageAutoUpdates}
	if rec := serve(h.rt, authedRequest(t, h.cookie, http.MethodPost, "/api/v1/apps/web/auto-update/webhook", "")); rec.Code != http.StatusInternalServerError {
		t.Fatalf("rotate = %d, want 500", rec.Code)
	}
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body, map[string]string{headerGitHubEvent: "package"}); rec.Code != http.StatusAccepted {
		t.Fatalf("unsigned delivery on the still-live URL = %d %s, want 202", rec.Code, rec.Body.String())
	}
}

func TestQueuedRunStopsWhenOptedOutOrRotatedMeanwhile(t *testing.T) {
	req := func(h *pushHarness) *http.Request {
		return httptest.NewRequest(http.MethodPost, h.path, nil).WithContext(withRegistryPushActor(context.Background(), registryProviderGHCR, "web"))
	}
	newPush := func(h *pushHarness) *registryPush {
		return withHook(h, &registryPush{Provider: registryProviderGHCR, Repo: "ghcr.io/acme/web", Tag: "latest", Digest: newDigest, signed: true})
	}
	t.Run("opted out", func(t *testing.T) {
		h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
		p := newPush(h)
		if err := h.db.SetImageAutoUpdate(context.Background(), "web", false); err != nil {
			t.Fatal(err)
		}
		if out, _ := h.rt.runRegistryPushOnce(req(h), "web", p); !strings.Contains(out, "turned off or the webhook was rotated") {
			t.Errorf("outcome = %q", out)
		}
	})
	t.Run("rotated", func(t *testing.T) {
		h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
		p := newPush(h)
		h.rotate(t, h.cookie, "")
		if out, _ := h.rt.runRegistryPushOnce(req(h), "web", p); !strings.Contains(out, "turned off or the webhook was rotated") {
			t.Errorf("outcome = %q", out)
		}
	})
}

func TestDebounceWaitIsBoundedWhenTheLastCheckIsInTheFuture(t *testing.T) {
	h := newPushHarness(t, "acme/web:1@"+oldDigest)
	ctx := context.Background()
	if _, err := h.rt.runtimeSettings.Set(ctx, runtimesettings.KeyDeploysRegistryPushDebounce, json.RawMessage(`"1s"`), "test"); err != nil {
		t.Fatal(err)
	}
	if err := h.db.RecordImageAutoUpdateCheck(ctx, "web", "x", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	start := time.Now()
	h.rt.waitOutPushDebounce(deadline, "web", registryPush{})
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("waited %s for a last check an hour in the future", elapsed)
	}
}

func TestSupersedeSurvivesTheRunDeadline(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	h.protect(t)
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body, h.signed(body)); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	h.waitIdle(t)
	h.waitPendingImages(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodPost, h.path, nil).WithContext(ctx)
	h.rt.supersedePushApprovals(req, "web", "ghcr.io/acme/web:latest@sha256:"+strings.Repeat("9", 64), registryPush{Provider: registryProviderGHCR})
	if pending, _ := h.db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web"); len(pending) != 0 {
		t.Fatalf("older approval stayed pending after a run deadline: %+v", pending)
	}
}

func TestStagedSecretPromotesOnlyWhenItIsTheOneVerified(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	ctx := context.Background()
	live := h.hook.SigningSecret
	if err := h.secret.SetValue(ctx, imageUpdateSecretNS+"web", imageUpdateSecretPendingKey, "staged"); err != nil {
		t.Fatal(err)
	}
	h.rt.promoteStagedSecret(ctx, "web", "something-else")
	if got, _ := h.secret.Resolve(ctx, imageUpdateSecretNS+"web", imageUpdateSecretKey); got != live {
		t.Fatalf("promoted a secret that was not the staged one: %q", got)
	}
	h.rt.promoteStagedSecret(ctx, "web", "staged")
	if got, _ := h.secret.Resolve(ctx, imageUpdateSecretNS+"web", imageUpdateSecretKey); got != "staged" {
		t.Fatalf("staged secret not promoted: %q", got)
	}
	if got, _ := h.secret.Resolve(ctx, imageUpdateSecretNS+"web", imageUpdateSecretPendingKey); got != pushPendingNone {
		t.Fatalf("staged slot not emptied: %q", got)
	}
}

func TestUnsignedDeliveryDoesNotDisplaceAQueuedSignedOne(t *testing.T) {
	var s pushSlot
	s.queue(registryPush{Tag: "a", signed: true})
	s.queue(registryPush{Tag: "b"})
	if s.next.Tag != "a" || !s.next.signed {
		t.Fatalf("next = %+v, want the signed delivery kept", s.next)
	}
	s.queue(registryPush{Tag: "c", signed: true})
	if s.next.Tag != "c" {
		t.Fatalf("a newer signed delivery must replace the queued one, next = %+v", s.next)
	}
}

func TestPersonRotatingAfterAnOwnerlessRotationRestoresApprovals(t *testing.T) {
	h := newPushHarness(t, "ghcr.io/acme/web:latest@"+oldDigest)
	h.protect(t)
	plain := "ownerless-ci-secret-2"
	if err := h.db.SaveAPIToken(context.Background(), store.APIToken{ID: "tok_ci2", Name: "ci", TokenHash: hashToken(plain), Abilities: []string{AbilityRead, AbilityWriteSensitive}, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	h.rotate(t, nil, plain)
	h.rotate(t, h.cookie, "")
	body := ghcrBody("published", "acme", "web", "latest", newDigest)
	if rec := h.post(body, h.signed(body)); rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d", rec.Code)
	}
	h.waitIdle(t)
	h.waitPendingImages(t)
}
