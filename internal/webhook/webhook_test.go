package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeAttemptStore is a hand-written fake for AttemptStore, the same
// pattern every other fake in this file already uses.
type fakeAttemptStore struct {
	mu      sync.Mutex
	saved   []store.DeployAttempt
	saveErr error

	finishedID     string
	finishedStatus string
	finishedErrMsg string
	finishErr      error

	// recorder, when set, lets SaveDeployAttempt check whether
	// Recorder.Start already registered the attempt it's about to save:
	// see TestBeginDeployAttempt_StartRunsBeforeAttemptIsSaveable.
	recorder          *deploylog.Recorder
	startedBeforeSave bool
	lastAttemptedID   string // set on every call, success or failure
}

func (f *fakeAttemptStore) SaveDeployAttempt(_ context.Context, a store.DeployAttempt) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastAttemptedID = a.ID
	if f.recorder != nil {
		if _, _, unsubscribe, ok := f.recorder.Snapshot(a.ID); ok {
			f.startedBeforeSave = true
			unsubscribe()
		}
	}
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saved = append(f.saved, a)
	return nil
}

func (f *fakeAttemptStore) FinishDeployAttempt(_ context.Context, id, status string, _ time.Time, errMsg string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finishedID = id
	f.finishedStatus = status
	f.finishedErrMsg = errMsg
	return f.finishErr
}

func (f *fakeAttemptStore) snapshot() ([]store.DeployAttempt, string, string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.saved, f.finishedID, f.finishedStatus, f.finishedErrMsg
}

// fakeDeployNotifier is a hand-written fake for DeployNotifier, the same
// pattern fakeAttemptStore above already uses: records every Dispatch
// call so a test can assert on what would have been sent, without a
// real HTTP server or SMTP connection.
type fakeDeployNotifier struct {
	mu    sync.Mutex
	calls []deployNotifierCall
}

type deployNotifierCall struct {
	resourceID string
	ev         alerting.DeployOutcome
}

func (f *fakeDeployNotifier) Dispatch(_ context.Context, resourceID string, ev alerting.DeployOutcome) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, deployNotifierCall{resourceID: resourceID, ev: ev})
}

func (f *fakeDeployNotifier) snapshot() []deployNotifierCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]deployNotifierCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// fakeDeployer is a hand-written fake, not a mocking framework, matching
// the pattern already used in internal/deploy and
// internal/reconcile/application's own tests.
type fakeDeployer struct {
	tag string
	err error

	calls   int
	lastReq deploy.Request
}

func (f *fakeDeployer) Deploy(_ context.Context, req deploy.Request, progress func(build.ProgressEvent)) (string, error) {
	f.calls++
	f.lastReq = req
	if progress != nil {
		progress(build.ProgressEvent{Step: "fake", Completed: true})
	}
	if f.err != nil {
		return "", f.err
	}
	return f.tag, nil
}

func sign(secret, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func pushBody(t *testing.T, ref, after string) []byte {
	t.Helper()
	b, err := json.Marshal(PushEvent{Ref: ref, After: after})
	if err != nil {
		t.Fatalf("marshal push event: %v", err)
	}
	return b
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// TestVerifySignature is the security-critical table: valid, invalid
// (wrong secret), missing header, and a tampered payload against a
// signature computed over the original (untampered) body, the case that
// actually exercises whether comparison is happening against the right
// bytes rather than just "is a header present."
func TestVerifySignature(t *testing.T) {
	secret := []byte("shared-secret")
	body := []byte(`{"ref":"refs/heads/main","after":"abc123"}`)
	validSig := sign(secret, body)

	tests := []struct {
		name   string
		secret []byte
		body   []byte
		header string
		want   bool
	}{
		{
			name:   "valid signature",
			secret: secret,
			body:   body,
			header: validSig,
			want:   true,
		},
		{
			name:   "invalid signature, wrong secret",
			secret: secret,
			body:   body,
			header: sign([]byte("different-secret"), body),
			want:   false,
		},
		{
			name:   "missing header",
			secret: secret,
			body:   body,
			header: "",
			want:   false,
		},
		{
			name:   "tampered payload, stale signature",
			secret: secret,
			body:   []byte(`{"ref":"refs/heads/main","after":"evil-sha"}`),
			header: validSig, // signature computed over the original body
			want:   false,
		},
		{
			name:   "missing sha256= prefix",
			secret: secret,
			body:   body,
			header: strings.TrimPrefix(validSig, "sha256="),
			want:   false,
		},
		{
			name:   "non-hex signature",
			secret: secret,
			body:   body,
			header: "sha256=not-hex-data",
			want:   false,
		},
		{
			name:   "empty configured secret always rejects",
			secret: nil,
			body:   body,
			header: sign([]byte(""), body),
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{cfg: Config{Secret: tt.secret}, log: discardLogger()}
			got := h.verifySignature(tt.body, tt.header)
			if got != tt.want {
				t.Errorf("verifySignature() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestVerifySignature_UsesConstantTimeCompare proves hmac.Equal, not a
// short-circuiting byte comparison, is actually what backs the check: a
// signature that shares a long correct prefix but diverges at the very
// last byte must be rejected exactly like one that diverges at the
// first byte. A naive `bytes.Equal`-based check would behave the same
// here in a unit test (both return false either way), so this test
// doesn't prove timing safety by itself, it's a correctness check that
// partial-prefix matches are never treated as valid, which is the
// behavioral property constant-time comparison is required to preserve
// without a fast-exit optimization creeping back in during a refactor.
func TestVerifySignature_UsesConstantTimeCompare(t *testing.T) {
	secret := []byte("shared-secret")
	body := []byte(`{"ref":"refs/heads/main","after":"abc123"}`)
	valid := sign(secret, body)
	validHex := strings.TrimPrefix(valid, "sha256=")

	// Flip the last hex character so the signature differs only in its
	// final byte.
	last := validHex[len(validHex)-1]
	flipped := byte('0')
	if last == '0' {
		flipped = '1'
	}
	almostValid := "sha256=" + validHex[:len(validHex)-1] + string(flipped)

	h := &Handler{cfg: Config{Secret: secret}, log: discardLogger()}
	if h.verifySignature(body, almostValid) {
		t.Error("verifySignature() = true for a signature differing in only its last byte, want false")
	}
}

func testConfig() Config {
	return Config{
		Secret:      []byte("shared-secret"),
		RepoURL:     "https://example.invalid/repo.git",
		Branch:      "main",
		ServiceName: "web",
		Service:     spec.Service{Build: spec.Build{Type: spec.BuildDockerfile}, Port: 3000},
		ImageRepo:   "levelrail/web",
	}
}

// newTestHandler builds a Handler with attempt tracking disabled
// (nil AttemptStore, nil Recorder): the majority of this file's tests
// exercise signature verification and dispatch logic, orthogonal to
// attempt tracking, and deployAttemptEnabled's own doc comment
// establishes that nil/nil is a supported, fully-functional
// configuration (the pre-existing build.SlogProgress behavior), not a
// degraded one. Tests that specifically cover attempt tracking build
// their own Handler via newTestHandlerWithAttempts below.
func newTestHandler(cfg Config, deployer Deployer, fetch fetchFunc) *Handler {
	h := New(cfg, deployer, nil, nil, nil, discardLogger())
	h.fetch = fetch
	return h
}

func newTestHandlerWithAttempts(cfg Config, deployer Deployer, fetch fetchFunc, attempts AttemptStore, recorder *deploylog.Recorder) *Handler {
	h := New(cfg, deployer, attempts, recorder, nil, discardLogger())
	h.fetch = fetch
	return h
}

func newTestHandlerWithNotifier(cfg Config, deployer Deployer, fetch fetchFunc, attempts AttemptStore, recorder *deploylog.Recorder, notifier DeployNotifier) *Handler {
	h := New(cfg, deployer, attempts, recorder, notifier, discardLogger())
	h.fetch = fetch
	return h
}

func doPush(t *testing.T, h *Handler, secret, body []byte, sigHeader string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(string(body)))
	if sigHeader != "" {
		req.Header.Set("X-Hub-Signature-256", sigHeader)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	_ = secret
	return rec
}

func TestServeHTTP_MissingSignature_Rejected(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	h := newTestHandler(cfg, deployer, func(context.Context, string, string) (string, func(), error) {
		t.Fatal("fetch should not be called when signature verification fails")
		return "", nil, nil
	})

	body := pushBody(t, "refs/heads/main", "sha1")
	rec := doPush(t, h, cfg.Secret, body, "")

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if deployer.calls != 0 {
		t.Errorf("deployer called %d times, want 0", deployer.calls)
	}
}

func TestServeHTTP_TamperedPayload_Rejected(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	h := newTestHandler(cfg, deployer, func(context.Context, string, string) (string, func(), error) {
		t.Fatal("fetch should not be called when signature verification fails")
		return "", nil, nil
	})

	originalBody := pushBody(t, "refs/heads/main", "sha1")
	sigOverOriginal := sign(cfg.Secret, originalBody)

	// Attacker swaps the target commit after the signature was computed.
	tamperedBody := pushBody(t, "refs/heads/main", "attacker-controlled-sha")
	rec := doPush(t, h, cfg.Secret, tamperedBody, sigOverOriginal)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if deployer.calls != 0 {
		t.Errorf("deployer called %d times, want 0", deployer.calls)
	}
}

func TestServeHTTP_MalformedPayload_Rejected(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	h := newTestHandler(cfg, deployer, func(context.Context, string, string) (string, func(), error) {
		t.Fatal("fetch should not be called for a malformed payload")
		return "", nil, nil
	})

	body := []byte("not json at all")
	rec := doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if deployer.calls != 0 {
		t.Errorf("deployer called %d times, want 0", deployer.calls)
	}
}

func TestServeHTTP_MissingAfterField_Rejected(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	h := newTestHandler(cfg, deployer, func(context.Context, string, string) (string, func(), error) {
		t.Fatal("fetch should not be called for a payload missing 'after'")
		return "", nil, nil
	})

	body := pushBody(t, "refs/heads/main", "")
	rec := doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// TestServeHTTP_WrongBranch_AcceptedButNotTriggered is the dispatch-logic
// case the task calls out explicitly: a push to a non-target branch must
// return 200 (so GitHub doesn't retry) without invoking the deployer or
// the git fetch at all.
func TestServeHTTP_WrongBranch_AcceptedButNotTriggered(t *testing.T) {
	cfg := testConfig() // Branch: "main"
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	fetchCalled := false
	h := newTestHandler(cfg, deployer, func(context.Context, string, string) (string, func(), error) {
		fetchCalled = true
		return "", func() {}, nil
	})

	body := pushBody(t, "refs/heads/feature-x", "sha1")
	rec := doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if deployer.calls != 0 {
		t.Errorf("deployer called %d times, want 0 for a non-target branch", deployer.calls)
	}
	if fetchCalled {
		t.Error("fetch was called for a non-target branch push, want no fetch")
	}
	if !strings.Contains(rec.Body.String(), "ignored") {
		t.Errorf("response body = %q, want it to say the push was ignored", rec.Body.String())
	}
}

func TestServeHTTP_DefaultBranch_WhenConfigBranchEmpty(t *testing.T) {
	cfg := testConfig()
	cfg.Branch = ""
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	h := newTestHandler(cfg, deployer, func(_ context.Context, _, _ string) (string, func(), error) {
		return "/tmp/whatever", func() {}, nil
	})

	body := pushBody(t, "refs/heads/main", "sha1")
	rec := doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if deployer.calls != 1 {
		t.Errorf("deployer called %d times, want 1 (default branch %q should match refs/heads/main)", deployer.calls, DefaultBranch)
	}
}

func TestServeHTTP_TargetBranch_TriggersDeploy(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}

	var fetchCalledWith struct {
		repoURL string
		sha     string
	}
	cleanupCalled := false
	h := newTestHandler(cfg, deployer, func(_ context.Context, repoURL, sha string) (string, func(), error) {
		fetchCalledWith.repoURL = repoURL
		fetchCalledWith.sha = sha
		return "/tmp/checkout-dir", func() { cleanupCalled = true }, nil
	})

	body := pushBody(t, "refs/heads/main", "sha1")
	rec := doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fetchCalledWith.repoURL != cfg.RepoURL {
		t.Errorf("fetch called with repoURL = %q, want %q", fetchCalledWith.repoURL, cfg.RepoURL)
	}
	if fetchCalledWith.sha != "sha1" {
		t.Errorf("fetch called with sha = %q, want %q", fetchCalledWith.sha, "sha1")
	}
	if deployer.calls != 1 {
		t.Fatalf("deployer called %d times, want 1", deployer.calls)
	}
	if deployer.lastReq.ServiceName != cfg.ServiceName {
		t.Errorf("Request.ServiceName = %q, want %q", deployer.lastReq.ServiceName, cfg.ServiceName)
	}
	if deployer.lastReq.SourceDir != "/tmp/checkout-dir" {
		t.Errorf("Request.SourceDir = %q, want %q", deployer.lastReq.SourceDir, "/tmp/checkout-dir")
	}
	if deployer.lastReq.CommitSHA != "sha1" {
		t.Errorf("Request.CommitSHA = %q, want %q", deployer.lastReq.CommitSHA, "sha1")
	}
	if deployer.lastReq.ImageRepo != cfg.ImageRepo {
		t.Errorf("Request.ImageRepo = %q, want %q", deployer.lastReq.ImageRepo, cfg.ImageRepo)
	}
	if !cleanupCalled {
		t.Error("cleanup was not called after a successful deploy")
	}
}

func TestServeHTTP_FetchFailure_ReturnsServerErrorAndDoesNotDeploy(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	h := newTestHandler(cfg, deployer, func(context.Context, string, string) (string, func(), error) {
		return "", nil, errors.New("clone failed: some internal detail about disk paths")
	})

	body := pushBody(t, "refs/heads/main", "sha1")
	rec := doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if deployer.calls != 0 {
		t.Errorf("deployer called %d times, want 0 when fetch fails", deployer.calls)
	}
	if strings.Contains(rec.Body.String(), "disk paths") {
		t.Errorf("response body leaked internal error detail: %q", rec.Body.String())
	}
}

func TestServeHTTP_DeployFailure_ReturnsServerErrorNotLeaky(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{err: errors.New("buildkit: internal socket path /var/run/secret-thing failed")}
	cleanupCalled := false
	h := newTestHandler(cfg, deployer, func(context.Context, string, string) (string, func(), error) {
		return "/tmp/checkout-dir", func() { cleanupCalled = true }, nil
	})

	body := pushBody(t, "refs/heads/main", "sha1")
	rec := doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if deployer.calls != 1 {
		t.Errorf("deployer called %d times, want 1", deployer.calls)
	}
	if strings.Contains(rec.Body.String(), "secret-thing") {
		t.Errorf("response body leaked internal error detail: %q", rec.Body.String())
	}
	if !cleanupCalled {
		t.Error("cleanup was not called after a failed deploy, checkout dir would leak")
	}
}

func TestNew_NilLoggerDefaultsToSlogDefault(t *testing.T) {
	h := New(testConfig(), &fakeDeployer{}, nil, nil, nil, nil)
	if h.log == nil {
		t.Fatal("New() with a nil logger left h.log nil, want it defaulted")
	}
	if h.fetch == nil {
		t.Error("New() left fetch unset, want it defaulted to cloneAndCheckout")
	}
}

func TestServeHTTP_BodyTooLarge_Rejected(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	h := newTestHandler(cfg, deployer, func(context.Context, string, string) (string, func(), error) {
		t.Fatal("fetch should not be called for an oversized body")
		return "", nil, nil
	})

	// One byte over the cap. Content doesn't need to be valid JSON or
	// carry a valid signature: the size check runs before either.
	oversized := make([]byte, maxPayloadBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", strings.NewReader(string(oversized)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if deployer.calls != 0 {
		t.Errorf("deployer called %d times, want 0", deployer.calls)
	}
}

func TestConfig_Branch_Default(t *testing.T) {
	c := Config{}
	if got := c.branch(); got != DefaultBranch {
		t.Errorf("branch() = %q, want %q", got, DefaultBranch)
	}
	if got := c.targetRef(); got != "refs/heads/"+DefaultBranch {
		t.Errorf("targetRef() = %q, want %q", got, "refs/heads/"+DefaultBranch)
	}
}

func TestConfig_Branch_Configured(t *testing.T) {
	c := Config{Branch: "release"}
	if got := c.branch(); got != "release" {
		t.Errorf("branch() = %q, want %q", got, "release")
	}
	if got := c.targetRef(); got != "refs/heads/release" {
		t.Errorf("targetRef() = %q, want %q", got, "refs/heads/release")
	}
}

// TestServeHTTP_TargetBranch_RecordsDeployAttempt_Succeeded is the third
// of this task's three real trigger paths: an unattended git push must
// mint and save a deploy_attempts row just like the two HTTP-triggered
// paths in internal/api, closing exactly the gap
// docs-local/research/deploy-attempt-id-and-log-persistence.md frames as
// the reason persistence matters most ("this product's core loop is
// unattended webhook deploys").
func TestServeHTTP_TargetBranch_RecordsDeployAttempt_Succeeded(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	attempts := &fakeAttemptStore{}
	recorder := deploylog.NewRecorder(nil, discardLogger())
	h := newTestHandlerWithAttempts(cfg, deployer, func(_ context.Context, _, _ string) (string, func(), error) {
		return "/tmp/checkout-dir", func() {}, nil
	}, attempts, recorder)

	body := pushBody(t, "refs/heads/main", "sha1")
	rec := doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	saved, finishedID, finishedStatus, finishedErrMsg := attempts.snapshot()
	if len(saved) != 1 {
		t.Fatalf("SaveDeployAttempt called %d times, want 1", len(saved))
	}
	if saved[0].ServiceName != cfg.ServiceName {
		t.Errorf("ServiceName = %q, want %q", saved[0].ServiceName, cfg.ServiceName)
	}
	if saved[0].Image != cfg.ImageRepo+":sha1" {
		t.Errorf("Image = %q, want %q", saved[0].Image, cfg.ImageRepo+":sha1")
	}
	if saved[0].CommitSHA != "sha1" {
		t.Errorf("CommitSHA = %q, want %q", saved[0].CommitSHA, "sha1")
	}
	if saved[0].Source != store.DeployAttemptSourceWebhook {
		t.Errorf("Source = %q, want %q", saved[0].Source, store.DeployAttemptSourceWebhook)
	}
	if saved[0].Status != store.DeployAttemptStatusRunning {
		t.Errorf("initial Status = %q, want %q", saved[0].Status, store.DeployAttemptStatusRunning)
	}
	if finishedID != saved[0].ID {
		t.Errorf("FinishDeployAttempt id = %q, want the same id SaveDeployAttempt was called with (%q)", finishedID, saved[0].ID)
	}
	if finishedStatus != store.DeployAttemptStatusSucceeded {
		t.Errorf("finished Status = %q, want %q", finishedStatus, store.DeployAttemptStatusSucceeded)
	}
	if finishedErrMsg != "" {
		t.Errorf("finished error = %q, want empty for a succeeded attempt", finishedErrMsg)
	}
}

func TestServeHTTP_DeployFailure_RecordsDeployAttempt_Failed(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{err: errors.New("buildkit: boom")}
	attempts := &fakeAttemptStore{}
	recorder := deploylog.NewRecorder(nil, discardLogger())
	h := newTestHandlerWithAttempts(cfg, deployer, func(_ context.Context, _, _ string) (string, func(), error) {
		return "/tmp/checkout-dir", func() {}, nil
	}, attempts, recorder)

	body := pushBody(t, "refs/heads/main", "sha1")
	rec := doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	_, _, finishedStatus, finishedErrMsg := attempts.snapshot()
	if finishedStatus != store.DeployAttemptStatusFailed {
		t.Errorf("finished Status = %q, want %q", finishedStatus, store.DeployAttemptStatusFailed)
	}
	if finishedErrMsg != "buildkit: boom" {
		t.Errorf("finished error = %q, want %q", finishedErrMsg, "buildkit: boom")
	}
}

// TestBeginDeployAttempt_StartRunsBeforeAttemptIsSaveable proves
// beginDeployAttempt calls Recorder.Start strictly before
// SaveDeployAttempt makes the attempt ID visible to a GET: a client can
// only ever discover an ID after SaveDeployAttempt returns, so if Start
// ran after that, a request landing right then would miss the live tail.
func TestBeginDeployAttempt_StartRunsBeforeAttemptIsSaveable(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	recorder := deploylog.NewRecorder(nil, discardLogger())
	attempts := &fakeAttemptStore{recorder: recorder}
	h := newTestHandlerWithAttempts(cfg, deployer, func(_ context.Context, _, _ string) (string, func(), error) {
		return "/tmp/checkout-dir", func() {}, nil
	}, attempts, recorder)

	body := pushBody(t, "refs/heads/main", "sha1")
	rec := doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	if !attempts.startedBeforeSave {
		t.Error("recorder.Snapshot(id) was not yet ok during SaveDeployAttempt: Start ran after the row became visible to a GET")
	}
}

// TestBeginDeployAttempt_SaveFails_RecorderDoesNotLeak proves a failed
// SaveDeployAttempt cleans up the Recorder.Start registration made just
// before it, rather than leaving a permanently active, never-Finished
// entry behind.
func TestBeginDeployAttempt_SaveFails_RecorderDoesNotLeak(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	recorder := deploylog.NewRecorder(nil, discardLogger())
	attempts := &fakeAttemptStore{recorder: recorder, saveErr: errors.New("db write failed")}
	h := newTestHandlerWithAttempts(cfg, deployer, func(_ context.Context, _, _ string) (string, func(), error) {
		return "/tmp/checkout-dir", func() {}, nil
	}, attempts, recorder)

	body := pushBody(t, "refs/heads/main", "sha1")
	doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	if deployer.calls != 1 {
		t.Fatalf("deployer called %d times, want 1 (attempt-tracking failures must not block the deploy)", deployer.calls)
	}
	attempts.mu.Lock()
	id := attempts.lastAttemptedID
	attempts.mu.Unlock()
	if id == "" {
		t.Fatal("SaveDeployAttempt was never called")
	}
	if _, _, unsubscribe, ok := recorder.Snapshot(id); ok {
		unsubscribe()
		t.Error("recorder still has an active entry for id after SaveDeployAttempt failed: Start's registration leaked instead of being cleaned up by Finish")
	}
}

// TestServeHTTP_WrongBranch_DoesNotRecordDeployAttempt: a push that
// never triggers a deploy at all must not create history for one.
func TestServeHTTP_WrongBranch_DoesNotRecordDeployAttempt(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	attempts := &fakeAttemptStore{}
	recorder := deploylog.NewRecorder(nil, discardLogger())
	h := newTestHandlerWithAttempts(cfg, deployer, func(context.Context, string, string) (string, func(), error) {
		t.Fatal("fetch should not be called for a non-target branch")
		return "", nil, nil
	}, attempts, recorder)

	body := pushBody(t, "refs/heads/feature-x", "sha1")
	doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	saved, _, _, _ := attempts.snapshot()
	if len(saved) != 0 {
		t.Errorf("SaveDeployAttempt called %d times, want 0 for an ignored push", len(saved))
	}
}

// TestServeHTTP_TargetBranch_DispatchesSucceededNotification: a
// successful push-triggered deploy fires exactly one deploy-outcome
// notification, after the deploy_attempts row is already recorded as
// succeeded (wave-2 roadmap item #5).
func TestServeHTTP_TargetBranch_DispatchesSucceededNotification(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	attempts := &fakeAttemptStore{}
	recorder := deploylog.NewRecorder(nil, discardLogger())
	notifier := &fakeDeployNotifier{}
	h := newTestHandlerWithNotifier(cfg, deployer, func(_ context.Context, _, _ string) (string, func(), error) {
		return "/tmp/checkout-dir", func() {}, nil
	}, attempts, recorder, notifier)

	body := pushBody(t, "refs/heads/main", "sha1")
	doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	calls := notifier.snapshot()
	if len(calls) != 1 {
		t.Fatalf("Dispatch called %d times, want 1", len(calls))
	}
	call := calls[0]
	if call.resourceID != "service:"+cfg.ServiceName {
		t.Errorf("resourceID = %q, want %q", call.resourceID, "service:"+cfg.ServiceName)
	}
	if call.ev.AppName != cfg.ServiceName {
		t.Errorf("ev.AppName = %q, want %q", call.ev.AppName, cfg.ServiceName)
	}
	if call.ev.Image != cfg.ImageRepo+":sha1" {
		t.Errorf("ev.Image = %q, want %q", call.ev.Image, cfg.ImageRepo+":sha1")
	}
	if !call.ev.Succeeded {
		t.Error("ev.Succeeded = false, want true for a successful deploy")
	}
	if call.ev.Error != "" {
		t.Errorf("ev.Error = %q, want empty for a succeeded attempt", call.ev.Error)
	}
}

// TestServeHTTP_DeployFailure_DispatchesFailedNotification: a failed
// push-triggered deploy fires a deploy-outcome notification too, with
// Succeeded=false and the deploy error carried through, not just a
// succeeded one.
func TestServeHTTP_DeployFailure_DispatchesFailedNotification(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{err: errors.New("buildkit: boom")}
	attempts := &fakeAttemptStore{}
	recorder := deploylog.NewRecorder(nil, discardLogger())
	notifier := &fakeDeployNotifier{}
	h := newTestHandlerWithNotifier(cfg, deployer, func(_ context.Context, _, _ string) (string, func(), error) {
		return "/tmp/checkout-dir", func() {}, nil
	}, attempts, recorder, notifier)

	body := pushBody(t, "refs/heads/main", "sha1")
	doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	calls := notifier.snapshot()
	if len(calls) != 1 {
		t.Fatalf("Dispatch called %d times, want 1", len(calls))
	}
	if calls[0].ev.Succeeded {
		t.Error("ev.Succeeded = true, want false for a failed deploy")
	}
	if calls[0].ev.Error != "buildkit: boom" {
		t.Errorf("ev.Error = %q, want %q", calls[0].ev.Error, "buildkit: boom")
	}
}

// TestServeHTTP_AttemptTrackingDisabled_NeverDispatches: notifier is
// configured but attempts/recorder are not, so beginDeployAttempt falls
// back to its noop finish (deployAttemptEnabled's own doc comment): no
// deploy_attempts row is ever recorded, and per Handler.notifier's own
// doc comment this closure is the only place Dispatch is ever called,
// so it must not fire either.
func TestServeHTTP_AttemptTrackingDisabled_NeverDispatches(t *testing.T) {
	cfg := testConfig()
	deployer := &fakeDeployer{tag: "levelrail/web:sha1"}
	notifier := &fakeDeployNotifier{}
	h := newTestHandlerWithNotifier(cfg, deployer, func(_ context.Context, _, _ string) (string, func(), error) {
		return "/tmp/checkout-dir", func() {}, nil
	}, nil, nil, notifier)

	body := pushBody(t, "refs/heads/main", "sha1")
	doPush(t, h, cfg.Secret, body, sign(cfg.Secret, body))

	if calls := notifier.snapshot(); len(calls) != 0 {
		t.Errorf("Dispatch called %d times, want 0 when attempt tracking is disabled", len(calls))
	}
}
