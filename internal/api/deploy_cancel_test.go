package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func postCancelDeploy(t *testing.T, rt *Router, cookie *http.Cookie, appName, attemptID string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/"+appName+"/deploys/"+attemptID+"/cancel", ""))
	return rec
}

// TestHandleCancelDeploy_CancelsRunningBuild proves the whole chain works
// end to end: the goroutine's context really is the one wired into
// rt.buildCancels, cancelling it actually unblocks fakeBuilder.Deploy (via
// blockOnCtx), and the deploy_attempts row lands "cancelled", not
// "failed".
func TestHandleCancelDeploy_CancelsRunningBuild(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fb.blockOnCtx = true
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	_, resp := postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main"}`)
	fb.awaitCall(t)

	rec := postCancelDeploy(t, rt, cookie, "web", resp.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var cancelResp cancelDeployResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &cancelResp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if cancelResp.ID != resp.ID {
		t.Errorf("ID = %q, want %q", cancelResp.ID, resp.ID)
	}

	a := awaitDeployAttemptFinished(t, db, resp.ID)
	if a.Status != store.DeployAttemptStatusCancelled {
		t.Errorf("Status = %q, want %q", a.Status, store.DeployAttemptStatusCancelled)
	}
	if a.Error == "" {
		t.Error("Error is empty, want a note that the attempt was cancelled")
	}

	// The registry entry must be gone once the goroutine's own defer runs
	// (awaitDeployAttemptFinished already proved the goroutine finished),
	// so a second cancel call on the same, now-finished attempt is a 409,
	// not a silently-accepted no-op.
	rec2 := postCancelDeploy(t, rt, cookie, "web", resp.ID)
	if rec2.Code != http.StatusConflict {
		t.Errorf("second cancel: status = %d, want %d; body = %s", rec2.Code, http.StatusConflict, rec2.Body.String())
	}
}

// TestHandleCancelDeploy_UnknownAttempt covers an attempt ID that was
// never minted at all: 404, the same "absence, not a cross-app leak"
// shape handleDeployLogStream already establishes for a bad deploy ID.
func TestHandleCancelDeploy_UnknownAttempt(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	rec := postCancelDeploy(t, rt, cookie, "web", "dep_does-not-exist")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestHandleCancelDeploy_AppNotFound covers a real attempt ID scoped to a
// different app name than the URL names: treated the same as unknown, not
// a cross-app leak.
func TestHandleCancelDeploy_AppNotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := postCancelDeploy(t, rt, cookie, "does-not-exist", "dep_whatever")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestHandleCancelDeploy_AlreadyFinished covers an attempt that ran to
// completion on its own before the cancel call arrived: 409, distinct
// from the "never existed" 404 above via the store's own FinishedAt.
func TestHandleCancelDeploy_AlreadyFinished(t *testing.T) {
	fb := newFakeBuilder("levelrail/web:abc123", nil)
	fetch := newFakeFetch(t.TempDir(), nil)
	rt, db := newTestRouterWithBuilder(t, fb, fetch)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	_, resp := postTriggerBuildAccepted(t, rt, cookie, `{"repo_url":"https://example.com/web.git","ref":"main"}`)
	awaitDeployAttemptFinished(t, db, resp.ID)

	rec := postCancelDeploy(t, rt, cookie, "web", resp.ID)
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already finished") {
		t.Errorf("body = %s, want a message noting the attempt already finished", rec.Body.String())
	}
}

// TestHandleCancelDeploy_NonCancelableSource covers a still-running
// attempt whose source this feature deliberately does not support
// cancelling (webhook, plain image-tag deploy, compose, promote): a
// deploy_attempts row with no matching rt.buildCancels entry, since only
// handleTriggerBuild (builds.go) ever registers one. 409, naming the real
// source, not a silent no-op and not a 404 (the row does exist).
func TestHandleCancelDeploy_NonCancelableSource(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedWebApp(t, db)

	if err := db.SaveDeployAttempt(context.Background(), store.DeployAttempt{
		ID: "dep_webhook1", ServiceName: "web", Image: "web:sha1", CommitSHA: "sha1",
		Source: store.DeployAttemptSourceWebhook, Status: store.DeployAttemptStatusRunning, StartedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed deploy attempt: %v", err)
	}

	rec := postCancelDeploy(t, rt, cookie, "web", "dep_webhook1")
	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "webhook") {
		t.Errorf("body = %s, want the actual source named", rec.Body.String())
	}
}

// TestHandleCancelDeploy_PlainWriteToken_Forbidden proves AbilityDeploy is
// really enforced by a real request, matching
// TestHandleRestartApp_PlainWriteToken_Forbidden's own established
// pattern (apps_test.go) for the identical ability boundary.
func TestHandleCancelDeploy_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	seedWebApp(t, db)

	const plaintext = "write-scoped-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/deploys/dep_whatever/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not be able to cancel a deploy", rec.Code, http.StatusForbidden)
	}
}
