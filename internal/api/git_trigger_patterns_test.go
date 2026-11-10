package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestTriggerPatternsMatchPush(t *testing.T) {
	push := store.GitSource{Branch: "main", TriggerMode: spec.TriggerModePush}
	release := store.GitSource{Branch: "main", TriggerMode: spec.TriggerModeRelease}
	tests := []struct {
		name string
		gs   store.GitSource
		ref  string
		want bool
	}{
		{"empty push keeps branch", push, "refs/heads/main", true},
		{"empty push other branch", push, "refs/heads/dev", false},
		{"empty release keeps any tag", release, "refs/tags/anything", true},
		{"empty release ignores branch", release, "refs/heads/main", false},
		{"tag pattern match", withTag(release, "v*"), "refs/tags/v1.0.0", true},
		{"tag pattern miss", withTag(release, "v*"), "refs/tags/nightly", false},
		{"tag pattern negation", withTag(release, "v*,!v*-rc*"), "refs/tags/v2-rc1", false},
		{"tag pattern never admits a branch", withTag(release, "**"), "refs/heads/main", false},
		{"branch pattern train", withBranch(push, "main,release/*"), "refs/heads/release/1.2", true},
		{"branch pattern negation", withBranch(push, "release/*,!release/old"), "refs/heads/release/old", false},
		{"branch pattern never admits a tag", withBranch(push, "**"), "refs/tags/v1", false},
		{"branch pattern never admits a pull ref", withBranch(push, "**"), "refs/pull/7/head", false},
		{"hostile long ref", withBranch(push, "**"), "refs/heads/" + strings.Repeat("x", 100000), false},
		{"control characters", withBranch(push, "**"), "refs/heads/a\x00b", false},
	}
	for _, tt := range tests {
		got, msg := gitSourceTriggerMatchesPushMax(tt.gs, tt.ref, 1024)
		if got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
		if !got && (msg == "" || len(msg) > 400) {
			t.Errorf("%s: skip message %d bytes, want a bounded reason", tt.name, len(msg))
		}
	}
}

func withTag(gs store.GitSource, p string) store.GitSource    { gs.TagPattern = p; return gs }
func withBranch(gs store.GitSource, p string) store.GitSource { gs.BranchPattern = p; return gs }

func TestTriggerPatternsSaveAndSkippedPush(t *testing.T) {
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile"}`)

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/git-source/deploy-settings", body))
		return rec
	}
	for _, bad := range []string{`{"branch_pattern":"refs/heads/main"}`, `{"tag_pattern":"v[0-9]*"}`, `{"branch_pattern":"!"}`} {
		if rec := put(bad); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", bad, rec.Code)
		}
	}
	rec := put(`{"branch_pattern":"release/*, !release/old"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("set: %d %s", rec.Code, rec.Body.String())
	}
	var got gitDeploySettings
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.BranchPattern != "release/*\n!release/old" {
		t.Fatalf("saved = %+v, err %v", got, err)
	}
	gs, err := db.GetGitSource(t.Context(), "web")
	if err != nil || gs.BranchPattern != "release/*\n!release/old" || gs.TagPattern != "" {
		t.Fatalf("stored = %+v, err %v", gs, err)
	}

	fb := &fakeBuilder{tag: "web:sha1"}
	rt.builder = fb
	body, _ := json.Marshal(map[string]any{"ref": "refs/heads/main", "after": "sha1", "before": "sha0"})
	res := postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, nil)
	requireStatusOK(t, res)
	if !strings.Contains(res.Body.String(), "does not match the branch pattern") || fb.calls != 0 {
		t.Fatalf("body = %q, builder calls %d, want a skip without a deploy", res.Body.String(), fb.calls)
	}
}

func TestBranchPatternDeployIsNotUndoneByThePoller(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile"}`)
	if err := db.SetGitSourceTriggerPatterns(t.Context(), "web", "", "release/*"); err != nil {
		t.Fatal(err)
	}
	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:r1"}
	rt.builder = fb

	body, _ := json.Marshal(map[string]any{"ref": "refs/heads/release/1", "after": "r1sha", "before": "r0sha"})
	requireStatusOK(t, postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, nil))
	if fb.calls != 1 {
		t.Fatalf("pattern push: builder calls %d, want 1", fb.calls)
	}
	gs, err := db.GetGitSource(t.Context(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.handlePolledHead(t.Context(), "web", *gs, "mainsha"); err != nil {
		t.Fatal(err)
	}
	rel, err := db.GetGitSourceReliability(t.Context(), "web")
	if err != nil {
		t.Fatal(err)
	}
	if rel.LastSeenHead != "" || fb.calls != 1 {
		t.Fatalf("last seen head %q, builder calls %d: the poller must leave a pattern source alone", rel.LastSeenHead, fb.calls)
	}
}

func TestTagPatternFiltersGitHubReleaseEvent(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	created := connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main","trigger_mode":"release"}`)
	if err := db.SetGitSourceTriggerPatterns(t.Context(), "web", "v2.*", ""); err != nil {
		t.Fatal(err)
	}
	fb := &fakeBuilder{tag: "web:v1.2.3"}
	rt.builder = fb
	body := releaseBody("published")
	rec := postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body), body, map[string]string{"X-GitHub-Event": "release"})
	requireStatusOK(t, rec)
	if fb.calls != 0 || !strings.Contains(rec.Body.String(), "does not match the tag pattern") {
		t.Fatalf("body %q, builder calls %d, want v1.2.3 skipped", rec.Body.String(), fb.calls)
	}
}

func TestBranchPatternAppliesToGitLabAndGiteaPushes(t *testing.T) {
	for _, forge := range []string{"gitlab", "gitea"} {
		t.Run(forge, func(t *testing.T) {
			rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
			cookie := loginTestSession(t, rt, db)
			seedApp(t, db, "web")
			created := connectGitSource(t, rt, cookie, `{"repo_url":"https://`+forge+`.example.com/org/web.git","branch":"main","build_type":"dockerfile"}`)
			if err := db.SetGitSourceTriggerPatterns(t.Context(), "web", "", "release/*"); err != nil {
				t.Fatal(err)
			}
			fb := &fakeBuilder{tag: "web:x"}
			rt.builder = fb
			body := []byte(`{"ref":"refs/heads/main","after":"sha1"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
			if forge == "gitlab" {
				req.Header.Set("X-Gitlab-Event", "Push Hook")
				req.Header.Set("X-Gitlab-Token", created.WebhookSecret)
			} else {
				req.Header.Set("X-Gitea-Event-Type", "push")
				req.Header.Set("X-Hub-Signature-256", sign([]byte(created.WebhookSecret), body))
			}
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || fb.calls != 0 || !strings.Contains(rec.Body.String(), "branch pattern") {
				t.Fatalf("status %d body %q builder calls %d, want a 200 skip", rec.Code, rec.Body.String(), fb.calls)
			}
		})
	}
}

func TestTruncatedRefKeepsValidUTF8(t *testing.T) {
	if got := truncatedRef("refs/heads/" + strings.Repeat("é", 200)); !utf8.ValidString(got) {
		t.Fatal("truncatedRef produced invalid UTF-8")
	}
}
