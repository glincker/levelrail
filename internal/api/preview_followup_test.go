package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type appDeleteFails struct{ AppGroupLister }

func (appDeleteFails) DeleteApp(context.Context, string) error { return errors.New("delete refused") }

func TestPreviewTeardown_AppRowDeleteFailureIsReported(t *testing.T) {
	rt, _, secret, _ := setUpPreviewApp(t)
	if rec := openPR(t, rt, secret, 1); rec.Code != http.StatusOK {
		t.Fatalf("open: status = %d", rec.Code)
	}
	rt.appGroups = appDeleteFails{rt.appGroups}

	rec := sendPullRequestWebhook(rt, secret, githubPullRequestBody("closed", 1, "sha1", "main"))
	if rec.Code != http.StatusMultiStatus {
		t.Fatalf("close: status = %d, body = %q, want 207", rec.Code, rec.Body.String())
	}
	if got := previewPRs(t, rt)[1]; got != store.PreviewStatusFailed {
		t.Errorf("preview status = %q, want failed", got)
	}
}

func TestPreviewLimit_EvictionFailureRefusesTheNewPreview(t *testing.T) {
	rt, _, secret, builder := setUpPreviewApp(t)
	rt.previewLimits = PreviewLimits{MaxPerApp: 1}
	if rec := openPR(t, rt, secret, 1); rec.Code != http.StatusOK {
		t.Fatalf("open #1: status = %d", rec.Code)
	}
	rt.appGroups = appDeleteFails{rt.appGroups}

	openPR(t, rt, secret, 2)
	if len(builder.calls) != 1 {
		t.Errorf("builder ran %d times, want 1 (second preview must not deploy)", len(builder.calls))
	}
	if got := previewPRs(t, rt)[2]; got != store.PreviewStatusLimitReached {
		t.Errorf("#2 status = %q, want limit_reached", got)
	}
}

func TestForkPreview_HoldWarnsWhenEarlierDeploymentCannotBeRemoved(t *testing.T) {
	rt, db, secret, _ := setUpPreviewApp(t)
	cookie := loginTestSession(t, rt, db)
	sendPullRequestWebhook(rt, secret, githubPullRequestBodyFrom("opened", 42, "sha1", "main", "mallory/web"))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/previews/42/approve", `{"confirm":true}`))
	rt.previewDeploys.Wait()
	rt.appGroups = appDeleteFails{rt.appGroups}

	sendPullRequestWebhook(rt, secret, githubPullRequestBodyFrom("synchronize", 42, "sha2", "main", "mallory/web"))
	row, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
	if err != nil || !strings.Contains(row.StatusReason, "use Tear down") {
		t.Fatalf("row = %+v, err = %v, want a Tear down warning", row, err)
	}
}

func TestPreviewComment_UpdateAndCreateBothFailingIsSwallowed(t *testing.T) {
	for name, h := range commentHarnesses(t) {
		t.Run(name, func(t *testing.T) {
			h.open()
			db := previewDB(t, h.rt)
			p, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
			if err != nil {
				t.Fatalf("load preview: %v", err)
			}
			_ = db.SetPreviewEnvironmentCommentID(context.Background(), p.ID, 0)
			h.comments.updateErr = errors.New("403 not your comment")
			h.failWith(errors.New("provider is down"))

			if rec := h.push(); rec.Code != http.StatusOK {
				t.Fatalf("push: status = %d, want 200 despite comment failures", rec.Code)
			}
		})
	}
}
