package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// commentHarness drives one provider's pull request lifecycle against its
// fake client, exposing only what the provider-neutral upsert tests need.
type commentHarness struct {
	rt       *Router
	open     func() *httptest.ResponseRecorder
	push     func() *httptest.ResponseRecorder
	closePR  func() *httptest.ResponseRecorder
	forkOpen func() *httptest.ResponseRecorder
	comments *fakePRComments
	created  func() int
	failWith func(error)
}

func commentHarnesses(t *testing.T) map[string]*commentHarness {
	t.Helper()
	out := map[string]*commentHarness{}

	{
		rt, secret, _, f := setUpPreviewAppWithGitHubNotifications(t, true)
		out["github"] = &commentHarness{
			rt: rt,
			open: func() *httptest.ResponseRecorder {
				return sendPullRequestWebhook(rt, secret, githubPullRequestBody("opened", 42, "sha1", "main"))
			},
			push: func() *httptest.ResponseRecorder {
				return sendPullRequestWebhook(rt, secret, githubPullRequestBody("synchronize", 42, "sha2", "main"))
			},
			closePR: func() *httptest.ResponseRecorder {
				return sendPullRequestWebhook(rt, secret, githubPullRequestBody("closed", 42, "sha2", "main"))
			},
			forkOpen: func() *httptest.ResponseRecorder {
				return sendPullRequestWebhook(rt, secret, githubPullRequestBodyFrom("opened", 42, "sha1", "main", "mallory/web"))
			},
			comments: &f.prComments, created: func() int { return len(f.commentCalls) }, failWith: func(err error) { f.commentErr = err },
		}
	}
	{
		rt, secret, _, f := setUpPreviewAppWithGitLabNotifications(t, true)
		out["gitlab"] = &commentHarness{
			rt: rt,
			open: func() *httptest.ResponseRecorder {
				return sendGitLabPullRequestWebhook(rt, secret, gitlabPullRequestBody("open"))
			},
			push: func() *httptest.ResponseRecorder {
				return sendGitLabPullRequestWebhook(rt, secret, gitlabPullRequestBody("update"))
			},
			closePR: func() *httptest.ResponseRecorder {
				return sendGitLabPullRequestWebhook(rt, secret, gitlabPullRequestBody("close"))
			},
			forkOpen: func() *httptest.ResponseRecorder {
				return sendGitLabPullRequestWebhook(rt, secret, gitlabPullRequestBodyFrom("open", "mallory/web"))
			},
			comments: &f.prComments, created: func() int { return len(f.noteCalls) }, failWith: func(err error) { f.noteErr = err },
		}
	}
	{
		rt, secret, _, f := setUpPreviewAppWithBitbucketNotifications(t, true)
		out["bitbucket"] = &commentHarness{
			rt: rt,
			open: func() *httptest.ResponseRecorder {
				return sendBitbucketPullRequestWebhook(rt, secret, "pullrequest:created", bitbucketPullRequestBody())
			},
			push: func() *httptest.ResponseRecorder {
				return sendBitbucketPullRequestWebhook(rt, secret, "pullrequest:updated", bitbucketPullRequestBody())
			},
			closePR: func() *httptest.ResponseRecorder {
				return sendBitbucketPullRequestWebhook(rt, secret, "pullrequest:fulfilled", bitbucketPullRequestBody())
			},
			forkOpen: func() *httptest.ResponseRecorder {
				return sendBitbucketPullRequestWebhook(rt, secret, "pullrequest:created", bitbucketPullRequestBodyFrom("mallory/web"))
			},
			comments: &f.prComments, created: func() int { return len(f.commentCalls) }, failWith: func(err error) { f.commentErr = err },
		}
	}
	{
		rt, secret, _, f := setUpPreviewAppWithGiteaNotifications(t, true)
		out["gitea"] = &commentHarness{
			rt: rt,
			open: func() *httptest.ResponseRecorder {
				return sendGiteaPullRequestWebhook(rt, secret, giteaPullRequestBody("opened"))
			},
			push: func() *httptest.ResponseRecorder {
				return sendGiteaPullRequestWebhook(rt, secret, giteaPullRequestBody("synchronized"))
			},
			closePR: func() *httptest.ResponseRecorder {
				return sendGiteaPullRequestWebhook(rt, secret, giteaPullRequestBody("closed"))
			},
			forkOpen: func() *httptest.ResponseRecorder {
				return sendGiteaPullRequestWebhook(rt, secret, giteaPullRequestBodyFrom("opened", "mallory/web"))
			},
			comments: &f.prComments, created: func() int { return len(f.commentCalls) }, failWith: func(err error) { f.commentErr = err },
		}
	}
	return out
}

func TestPreviewComment_OneCommentEditedInPlaceAcrossLifecycle(t *testing.T) {
	for name, h := range commentHarnesses(t) {
		t.Run(name, func(t *testing.T) {
			if rec := h.open(); rec.Code != http.StatusOK {
				t.Fatalf("open: status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if h.created() != 1 || !strings.Contains(h.comments.comments[0].Body, "Preview environment: ready") {
				t.Fatalf("after open: created = %d, comments = %+v, want one ready comment", h.created(), h.comments.comments)
			}
			if !strings.Contains(h.comments.comments[0].Body, "<!-- preview-env:web-pr-42 -->") {
				t.Errorf("comment lacks the hidden marker: %q", h.comments.comments[0].Body)
			}

			h.push()
			h.push()
			if h.created() != 1 || len(h.comments.comments) != 1 {
				t.Fatalf("after pushes: created = %d, thread = %d comments, want still 1 (edited in place)", h.created(), len(h.comments.comments))
			}
			if !strings.Contains(h.comments.comments[0].Body, "`sha") {
				t.Errorf("comment lacks the commit sha: %q", h.comments.comments[0].Body)
			}

			h.closePR()
			if h.created() != 1 || !strings.Contains(h.comments.comments[0].Body, "Preview environment: removed") {
				t.Fatalf("after close: created = %d, body = %q, want the same comment marked removed", h.created(), h.comments.comments[0].Body)
			}
			if !strings.Contains(h.comments.comments[0].Body, "closed or merged") {
				t.Errorf("removed comment lacks the reason: %q", h.comments.comments[0].Body)
			}
		})
	}
}

func TestPreviewComment_FindsExistingByMarkerWhenIDIsLost(t *testing.T) {
	for name, h := range commentHarnesses(t) {
		t.Run(name, func(t *testing.T) {
			h.open()
			db := previewDB(t, h.rt)
			p, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
			if err != nil || p.CommentID == 0 {
				t.Fatalf("preview = %+v, err = %v, want a remembered comment id", p, err)
			}
			if err := db.SetPreviewEnvironmentCommentID(context.Background(), p.ID, 0); err != nil {
				t.Fatalf("forget comment id: %v", err)
			}

			h.push()
			if h.created() != 1 || len(h.comments.comments) != 1 {
				t.Fatalf("created = %d, thread = %d, want the marker lookup to reuse the comment", h.created(), len(h.comments.comments))
			}
			p, _ = db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
			if p.CommentID == 0 {
				t.Error("comment id was not remembered again")
			}
		})
	}
}

func TestPreviewComment_RecreatesAfterOperatorDeletesIt(t *testing.T) {
	for name, h := range commentHarnesses(t) {
		t.Run(name, func(t *testing.T) {
			h.open()
			h.comments.comments = nil

			h.push()
			if h.created() != 2 || len(h.comments.comments) != 1 {
				t.Fatalf("created = %d, thread = %d, want a fresh comment after the old one vanished", h.created(), len(h.comments.comments))
			}
		})
	}
}

func TestPreviewComment_FailureNeverFailsTheDeploy(t *testing.T) {
	for name, h := range commentHarnesses(t) {
		t.Run(name, func(t *testing.T) {
			h.failWith(errors.New("provider is down"))
			h.comments.listErr = errors.New("provider is down")

			rec := h.open()
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "preview deployed") {
				t.Fatalf("status = %d, body = %q, want the deploy to succeed despite comment failures", rec.Code, rec.Body.String())
			}
			p, err := previewDB(t, h.rt).GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
			if err != nil || p.Status != store.PreviewStatusActive {
				t.Fatalf("preview = %+v, err = %v, want active", p, err)
			}
		})
	}
}

func TestPreviewComment_ForkSkipExplainsWhyOnceAndIsEditedNotRepeated(t *testing.T) {
	for name, h := range commentHarnesses(t) {
		t.Run(name, func(t *testing.T) {
			h.forkOpen()
			h.forkOpen()
			if h.created() != 1 || len(h.comments.comments) != 1 {
				t.Fatalf("created = %d, thread = %d, want exactly one explanatory comment", h.created(), len(h.comments.comments))
			}
			body := h.comments.comments[0].Body
			if !strings.Contains(body, "awaiting approval") || !strings.Contains(body, "fork") || !strings.Contains(body, "approve") {
				t.Errorf("fork comment = %q, want why it was skipped and how to approve", body)
			}
		})
	}
}

func TestRenderPreviewComment(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 30, 0, 0, time.UTC)
	p := store.PreviewEnvironment{PreviewAppID: "web-pr-7", HeadSHA: "0123456789abcdef", Domain: "pr-7.web.example.com"}

	ready := renderPreviewComment(p, previewCommentReady, "", now)
	for _, want := range []string{"<!-- preview-env:web-pr-7 -->", "Preview environment: ready", "https://pr-7.web.example.com", "`0123456`", "2026-09-26 12:30:00 UTC"} {
		if !strings.Contains(ready, want) {
			t.Errorf("ready comment missing %q:\n%s", want, ready)
		}
	}

	failed := renderPreviewComment(p, previewCommentFailed, "build failed\n@everyone `rm -rf` "+strings.Repeat("x", 400), now)
	if strings.Contains(failed, "https://") {
		t.Errorf("failed comment should not advertise the URL:\n%s", failed)
	}
	if strings.Contains(failed, "@everyone") || strings.Contains(failed, "`rm") {
		t.Errorf("failed comment leaks a live mention or backticks:\n%s", failed)
	}
	if len(failed) > 700 {
		t.Errorf("failed comment is %d bytes, want the reason bounded", len(failed))
	}
}
