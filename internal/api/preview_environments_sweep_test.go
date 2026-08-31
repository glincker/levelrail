package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestSweepStalePreviewEnvironments_TearsDownStaleOnly is the TTL
// fallback's core behavior: a preview whose UpdatedAt is older than the
// configured TTL gets torn down through the same teardownPreviewRecord
// path the pull-request-closed webhook uses, while one updated recently
// (a real, still-open pull request) is left alone.
func TestSweepStalePreviewEnvironments_TearsDownStaleOnly(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)
	ctx := context.Background()
	rt.previewTTL = time.Hour

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	if rec := sendPullRequestWebhook(rt, secret, body); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	preview, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	preview.UpdatedAt = time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	if err := db.UpdatePreviewEnvironment(ctx, *preview); err != nil {
		t.Fatalf("UpdatePreviewEnvironment() backdate error = %v", err)
	}

	swept, err := rt.SweepStalePreviewEnvironments(ctx)
	if err != nil {
		t.Fatalf("SweepStalePreviewEnvironments() error = %v", err)
	}
	if swept != 1 {
		t.Fatalf("swept = %d, want 1", swept)
	}

	if _, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42); !errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
		t.Fatalf("preview still present after sweep, error = %v", err)
	}
}

// TestSweepStalePreviewEnvironments_FreshLeftAlone guards against a TTL
// sweep tearing down an actively-updated preview: a fresh UpdatedAt must
// never be swept regardless of how short a caller sets previewTTL to.
func TestSweepStalePreviewEnvironments_FreshLeftAlone(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)
	ctx := context.Background()
	rt.previewTTL = 24 * time.Hour

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	if rec := sendPullRequestWebhook(rt, secret, body); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	swept, err := rt.SweepStalePreviewEnvironments(ctx)
	if err != nil {
		t.Fatalf("SweepStalePreviewEnvironments() error = %v", err)
	}
	if swept != 0 {
		t.Fatalf("swept = %d, want 0", swept)
	}

	if _, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42); err != nil {
		t.Fatalf("fresh preview was swept: %v", err)
	}
}

func TestHandleSweepPreviewEnvironments(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)
	ctx := context.Background()
	rt.previewTTL = time.Hour

	body := githubPullRequestBody("opened", 42, "sha1", "main")
	if rec := sendPullRequestWebhook(rt, secret, body); rec.Code != http.StatusOK {
		t.Fatalf("opened: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	preview, err := db.GetPreviewEnvironmentByAppAndPR(ctx, "web", 42)
	if err != nil {
		t.Fatalf("GetPreviewEnvironmentByAppAndPR() error = %v", err)
	}
	preview.UpdatedAt = time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	if err := db.UpdatePreviewEnvironment(ctx, *preview); err != nil {
		t.Fatalf("UpdatePreviewEnvironment() backdate error = %v", err)
	}

	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/previews/sweep", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var got sweepPreviewEnvironmentsResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Swept != 1 {
		t.Errorf("Swept = %d, want 1", got.Swept)
	}
}
