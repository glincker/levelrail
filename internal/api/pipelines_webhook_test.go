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
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
)

const webhookPipelineYAML = "version: 1\nname: ci\non: { webhook: {} }\nparameters: [{ name: env, type: choice, choices: [staging, prod], default: staging }]\njobs:\n  t:\n    image: alpine\n    steps:\n      - run: echo hi\n"

type hookEnv struct {
	rt     *Router
	db     *store.DB
	runner *fakePipelineRunner
	cookie *http.Cookie
	secret *fakeGitSourceSecrets
}

func newHookEnv(t *testing.T, opts ...Option) *hookEnv {
	t.Helper()
	db := openTestDB(t)
	runner := &fakePipelineRunner{}
	secrets := newFakeGitSourceSecrets()
	rt := NewRouter(discardLogger(), testBrand(), db, append([]Option{WithPipelines(db, runner), WithGitSourceSecrets(secrets)}, opts...)...)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	now := time.Now().UTC()
	if _, err := db.SavePipeline(context.Background(), store.Pipeline{ID: "p1", AppName: "web", Name: "ci", YAML: webhookPipelineYAML, Enabled: true, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	return &hookEnv{rt: rt, db: db, runner: runner, cookie: cookie, secret: secrets}
}

func (e *hookEnv) do(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, req)
	return rec
}

func (e *hookEnv) createTrigger(t *testing.T, hmacMode bool) pipelineWebhookResource {
	t.Helper()
	body := `{"hmac":false}`
	if hmacMode {
		body = `{"hmac":true}`
	}
	rec := e.do(authedRequest(t, e.cookie, http.MethodPut, "/api/v1/apps/web/pipelines/ci/webhook", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("create trigger: %d %s", rec.Code, rec.Body.String())
	}
	var res pipelineWebhookResource
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func hookRequest(path, contentType, body string, hdr map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return req
}

func hookSign(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func TestPipelineWebhookTokenAuthAndPayload(t *testing.T) {
	e := newHookEnv(t)
	tr := e.createTrigger(t, false)
	if tr.Token == "" || !strings.HasSuffix(tr.Path, "/"+tr.Token) {
		t.Fatalf("token shown once is missing: %+v", tr)
	}
	stored, _ := e.db.GetPipelineWebhookTrigger(context.Background(), "p1")
	if stored.TokenHash == tr.Token || stored.TokenHash != hashToken(tr.Token) {
		t.Errorf("token must be stored hashed, got %q", stored.TokenHash)
	}
	if rec := e.do(authedRequest(t, e.cookie, http.MethodGet, "/api/v1/apps/web/pipelines/ci/webhook", "")); strings.Contains(rec.Body.String(), tr.Token) {
		t.Error("GET leaked the token")
	}

	tests := []struct {
		name   string
		path   string
		ct     string
		body   string
		status int
	}{
		{"ok json", tr.Path, "application/json", `{"ref":"main"}`, http.StatusAccepted},
		{"ok with charset", tr.Path, "application/json; charset=utf-8", `{"a":1}`, http.StatusAccepted},
		{"wrong token", pipelineHookPrefix + "p1/not-the-token", "application/json", `{}`, http.StatusNotFound},
		{"unknown pipeline", pipelineHookPrefix + "nope/" + tr.Token, "application/json", `{}`, http.StatusNotFound},
		{"missing content type", tr.Path, "", `{}`, http.StatusUnsupportedMediaType},
		{"bad content type", tr.Path, "application/xml", `<a/>`, http.StatusUnsupportedMediaType},
		{"invalid json", tr.Path, "application/json", `{nope`, http.StatusBadRequest},
		{"oversized body", tr.Path, "text/plain", strings.Repeat("x", defaultPipelineWebhookMaxBody+1), http.StatusRequestEntityTooLarge},
		{"undeclared parameter", tr.Path + "?bogus=1", "application/json", `{}`, http.StatusBadRequest},
		{"bad choice parameter", tr.Path + "?env=dev", "application/json", `{}`, http.StatusBadRequest},
		{"good parameter", tr.Path + "?env=prod", "application/json", `{}`, http.StatusAccepted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.do(hookRequest(tc.path, tc.ct, tc.body, nil))
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
	var last pipeline.StartOptions
	for _, o := range e.runner.opts {
		last = o
	}
	if last.Trigger != pipeline.TriggerWebhook || last.Inputs["env"] != "prod" || string(last.Payload) != "{}" || last.PayloadContentType != "application/json" {
		t.Errorf("last start options = %+v", last)
	}
}

func TestPipelineWebhookDisabledTriggerAndPipeline(t *testing.T) {
	e := newHookEnv(t)
	tr := e.createTrigger(t, false)
	e.do(authedRequest(t, e.cookie, http.MethodPut, "/api/v1/apps/web/pipelines/ci/webhook", `{"enabled":false}`))
	if rec := e.do(hookRequest(tr.Path, "application/json", `{}`, nil)); rec.Code != http.StatusNotFound {
		t.Errorf("disabled trigger = %d, want uniform 404", rec.Code)
	}
	e.do(authedRequest(t, e.cookie, http.MethodPut, "/api/v1/apps/web/pipelines/ci/webhook", `{"enabled":true}`))
	p, _ := e.db.GetPipeline(context.Background(), "p1")
	p.YAML = apiPipelineYAML
	_, _ = e.db.SavePipeline(context.Background(), p)
	if rec := e.do(hookRequest(tr.Path, "application/json", `{}`, nil)); rec.Code != http.StatusConflict {
		t.Errorf("pipeline without on.webhook = %d, want 409: %s", rec.Code, rec.Body.String())
	}
}

func TestPipelineWebhookHMACMatrix(t *testing.T) {
	e := newHookEnv(t)
	tr := e.createTrigger(t, true)
	if tr.Secret == "" {
		t.Fatal("hmac secret not returned once")
	}
	body := `{"event":"deploy"}`
	tests := []struct {
		name   string
		hdr    map[string]string
		status int
	}{
		{"valid", map[string]string{pipelineSignatureHeader: hookSign(tr.Secret, body)}, http.StatusAccepted},
		{"valid upper-case hex", map[string]string{pipelineSignatureHeader: "sha256=" + strings.ToUpper(strings.TrimPrefix(hookSign(tr.Secret, body), "sha256="))}, http.StatusAccepted},
		{"missing header", nil, http.StatusUnauthorized},
		{"wrong secret", map[string]string{pipelineSignatureHeader: hookSign("other", body)}, http.StatusUnauthorized},
		{"signature of another body", map[string]string{pipelineSignatureHeader: hookSign(tr.Secret, `{"event":"other"}`)}, http.StatusUnauthorized},
		{"no scheme prefix", map[string]string{pipelineSignatureHeader: strings.TrimPrefix(hookSign(tr.Secret, body), "sha256=")}, http.StatusUnauthorized},
		{"empty signature", map[string]string{pipelineSignatureHeader: "sha256="}, http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if rec := e.do(hookRequest(tr.Path, "application/json", body, tc.hdr)); rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
	n := len(e.runner.started)
	if n != 2 {
		t.Errorf("runs started = %d, want 2 (only the validly signed requests)", n)
	}
	secretText, _ := e.secret.Resolve(context.Background(), store.PipelineWebhookSecretsKey("p1"), pipelineHMACSecretKey)
	if secretText != tr.Secret {
		t.Error("hmac secret is not held in the secrets store")
	}
}

func TestPipelineWebhookIdempotencyAndReplay(t *testing.T) {
	e := newHookEnv(t)
	tr := e.createTrigger(t, false)
	hdr := map[string]string{"Idempotency-Key": "abc-123"}
	first := e.do(hookRequest(tr.Path, "application/json", `{"n":1}`, hdr))
	second := e.do(hookRequest(tr.Path, "application/json", `{"n":1}`, hdr))
	if first.Code != http.StatusAccepted || second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"duplicate"`) {
		t.Fatalf("retry handling: %d then %d %s", first.Code, second.Code, second.Body.String())
	}
	if len(e.runner.started) != 1 {
		t.Fatalf("a retried delivery started %d runs, want 1", len(e.runner.started))
	}
	if rec := e.do(hookRequest(tr.Path, "application/json", `{"n":2}`, map[string]string{"Idempotency-Key": "other"})); rec.Code != http.StatusAccepted {
		t.Errorf("distinct key = %d", rec.Code)
	}
	if rec := e.do(hookRequest(tr.Path, "application/json", `{}`, map[string]string{"Idempotency-Key": strings.Repeat("k", maxIdempotencyKeyLen+1)})); rec.Code != http.StatusBadRequest {
		t.Errorf("overlong key = %d", rec.Code)
	}

	rec := e.do(authedRequest(t, e.cookie, http.MethodGet, "/api/v1/apps/web/pipelines/ci/webhook/deliveries", ""))
	var list []pipelineWebhookDeliveryResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) < 3 {
		t.Fatalf("deliveries = %s (%v)", rec.Body.String(), err)
	}
	if strings.Contains(rec.Body.String(), `"n":1`) {
		t.Error("delivery list exposes payload bodies")
	}
	var started, rejected string
	for _, d := range list {
		switch {
		case d.Outcome == store.WebhookOutcomeStarted && d.IdempotencyKey == "abc-123":
			started = d.ID
		case d.Outcome == store.WebhookOutcomeRejected:
			rejected = d.ID
		}
	}
	runsBefore := len(e.runner.started)
	rec = e.do(authedRequest(t, e.cookie, http.MethodPost, "/api/v1/apps/web/pipelines/ci/webhook/deliveries/"+started+"/replay", ""))
	if rec.Code != http.StatusAccepted || len(e.runner.started) != runsBefore+1 {
		t.Fatalf("replay = %d %s", rec.Code, rec.Body.String())
	}
	if got := e.runner.opts[len(e.runner.opts)-1]; string(got.Payload) != `{"n":1}` || !strings.HasPrefix(got.Actor, "replay:") {
		t.Errorf("replayed options = %+v", got)
	}
	if rejected != "" {
		rec = e.do(authedRequest(t, e.cookie, http.MethodPost, "/api/v1/apps/web/pipelines/ci/webhook/deliveries/"+rejected+"/replay", ""))
		if rec.Code != http.StatusConflict {
			t.Errorf("replaying a rejected delivery = %d, want 409", rec.Code)
		}
	}
	if rec := e.do(authedRequest(t, e.cookie, http.MethodPost, "/api/v1/apps/web/pipelines/ci/webhook/deliveries/nope/replay", "")); rec.Code != http.StatusNotFound {
		t.Errorf("unknown delivery = %d", rec.Code)
	}
}

func TestPipelineWebhookRotateInvalidatesOldToken(t *testing.T) {
	e := newHookEnv(t)
	old := e.createTrigger(t, false)
	rec := e.do(authedRequest(t, e.cookie, http.MethodPost, "/api/v1/apps/web/pipelines/ci/webhook/rotate", ""))
	var fresh pipelineWebhookResource
	_ = json.Unmarshal(rec.Body.Bytes(), &fresh)
	if rec.Code != http.StatusOK || fresh.Token == "" || fresh.Token == old.Token {
		t.Fatalf("rotate = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(hookRequest(old.Path, "application/json", `{}`, nil)); rec.Code != http.StatusNotFound {
		t.Errorf("old token = %d, want 404", rec.Code)
	}
	if rec := e.do(hookRequest(fresh.Path, "application/json", `{}`, nil)); rec.Code != http.StatusAccepted {
		t.Errorf("new token = %d", rec.Code)
	}
	if rec := e.do(authedRequest(t, e.cookie, http.MethodDelete, "/api/v1/apps/web/pipelines/ci/webhook", "")); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := e.do(hookRequest(fresh.Path, "application/json", `{}`, nil)); rec.Code != http.StatusNotFound {
		t.Errorf("deleted trigger = %d, want 404", rec.Code)
	}
}

func TestPipelineWebhookRateLimited(t *testing.T) {
	e := newHookEnv(t, WithWebhookRateLimit(2))
	tr := e.createTrigger(t, false)
	var codes []int
	for range 4 {
		codes = append(codes, e.do(hookRequest(tr.Path, "application/json", `{}`, nil)).Code)
	}
	if codes[0] != http.StatusAccepted || codes[3] != http.StatusTooManyRequests {
		t.Errorf("codes = %v, want the limiter to trip by the fourth call", codes)
	}
}

func TestPipelineWebhookNeedsSecretStorageForHMAC(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithPipelines(db, &fakePipelineRunner{}))
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	now := time.Now().UTC()
	_, _ = db.SavePipeline(context.Background(), store.Pipeline{ID: "p1", AppName: "web", Name: "ci", YAML: webhookPipelineYAML, Enabled: true, CreatedAt: now, UpdatedAt: now})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/pipelines/ci/webhook", `{"hmac":true}`))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("hmac without secret storage = %d, want 501", rec.Code)
	}
}

func TestPipelineArtifactRoutes(t *testing.T) {
	e := newHookEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_, _ = e.db.CreatePipelineRun(ctx, store.PipelineRun{ID: "r1", PipelineID: "p1", AppName: "web", TriggerKind: "manual", Definition: apiPipelineYAML, CreatedAt: now})
	_ = e.db.SavePipelineArtifact(ctx, store.PipelineArtifact{ID: "a1", RunID: "r1", JobKey: "build", Name: "build", SizeBytes: 4, SHA256: "ab", StoragePath: "r1/a1.tar.gz", CreatedAt: now, ExpiresAt: now.Add(time.Hour)})
	rec := e.do(authedRequest(t, e.cookie, http.MethodGet, "/api/v1/apps/web/pipeline-runs/r1/artifacts", ""))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"job":"build"`) {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(authedRequest(t, e.cookie, http.MethodGet, "/api/v1/apps/other/pipeline-runs/r1/artifacts", "")); rec.Code != http.StatusNotFound {
		t.Errorf("cross-app list = %d", rec.Code)
	}
	if rec := e.do(authedRequest(t, e.cookie, http.MethodGet, "/api/v1/apps/web/pipeline-runs/r1/artifacts/a1/download", "")); rec.Code != http.StatusNotImplemented {
		t.Errorf("download without storage = %d, want 501", rec.Code)
	}
	if rec := e.do(httptest.NewRequest(http.MethodGet, "/api/v1/apps/web/pipeline-runs/r1/artifacts/a1/download", bytes.NewReader(nil))); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous download = %d, want 401", rec.Code)
	}
}

func TestPipelineRefAndCommitInputChecks(t *testing.T) {
	good := [][2]string{{"", ""}, {"main", "abc1234"}, {"refs/heads/feat/x-1", strings.Repeat("a", 64)}}
	for _, g := range good {
		if err := validatePipelineRefSHA(g[0], g[1]); err != nil {
			t.Errorf("%q %q rejected: %v", g[0], g[1], err)
		}
	}
	bad := [][2]string{
		{"-x", ""}, {"a..b", ""}, {"a b", ""}, {"a~1", ""}, {"a:b", ""}, {"a\\nb", ""}, {"a/", ""}, {"x.lock", ""},
		{strings.Repeat("a", 300), ""}, {"main", "abc12"}, {"main", "zzzzzzz"}, {"main", strings.Repeat("a", 65)},
	}
	for _, b := range bad {
		if err := validatePipelineRefSHA(b[0], b[1]); !errors.Is(err, pipeline.ErrInvalidInput) {
			t.Errorf("%q %q accepted", b[0], b[1])
		}
	}
}
