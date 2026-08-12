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
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
)

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
	b, err := json.Marshal(pushEvent{Ref: ref, After: after})
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

func newTestHandler(cfg Config, deployer Deployer, fetch fetchFunc) *Handler {
	h := New(cfg, deployer, discardLogger())
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
	h := New(testConfig(), &fakeDeployer{}, nil)
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
