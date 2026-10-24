package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestWebhookBodySeenClaim(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	window := 10 * time.Minute
	body := pushBody("refs/heads/main")
	other := pushBody("refs/heads/dev")

	cases := []struct {
		name  string
		first []byte
		again []byte
		gap   time.Duration
		want  bool
	}{
		{"identical body inside window is a replay", body, body, time.Minute, false},
		{"distinct body is accepted", body, other, time.Second, true},
		{"identical body after window is accepted", body, body, window + time.Second, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s webhookBodySeen
			if !s.claim(webhookBodyKey("github", "web", tc.first), t0, window) {
				t.Fatal("first claim rejected")
			}
			if got := s.claim(webhookBodyKey("github", "web", tc.again), t0.Add(tc.gap), window); got != tc.want {
				t.Errorf("second claim = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWebhookBodyKeyScopes(t *testing.T) {
	body := pushBody("refs/heads/main")
	if webhookBodyKey("github", "web", body) == webhookBodyKey("gitlab", "web", body) {
		t.Error("key ignores provider")
	}
	if webhookBodyKey("github", "web", body) == webhookBodyKey("github", "api", body) {
		t.Error("key ignores app")
	}
}

func TestWebhookReplayWindowEnv(t *testing.T) {
	t.Setenv(envWebhookReplayWindow, "")
	if got := webhookReplayWindow(); got != defaultWebhookReplayWindow {
		t.Errorf("default = %v, want %v", got, defaultWebhookReplayWindow)
	}
	t.Setenv(envWebhookReplayWindow, "45s")
	if got := webhookReplayWindow(); got != 45*time.Second {
		t.Errorf("env = %v, want 45s", got)
	}
}

func TestHandleGitPushWebhook_ReplayedBodyIgnored(t *testing.T) {
	cases := []struct {
		name       string
		replayHdrs map[string]string
	}{
		{"changed delivery header", map[string]string{"X-GitHub-Delivery": "fresh-id"}},
		{"missing delivery header", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, created := newConnectedGitSourceRouter(t, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile"}`)
			var fetchCalls []fakeGitSourceFetchCall
			rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
			fb := &fakeBuilder{tag: "web:sha1"}
			rt.builder = fb

			body := pushBody("refs/heads/main")
			sig := sign([]byte(created.WebhookSecret), body)
			first := postGitWebhook(t, rt, "X-Hub-Signature-256", sig, body, map[string]string{"X-GitHub-Delivery": "orig-id"})
			requireStatusOK(t, first)

			replay := postGitWebhook(t, rt, "X-Hub-Signature-256", sig, body, tc.replayHdrs)
			requireStatusOK(t, replay)
			if !strings.Contains(replay.Body.String(), webhookNoteReplayedBody) {
				t.Errorf("replay body = %q, want note %q", replay.Body.String(), webhookNoteReplayedBody)
			}
			if fb.calls != 1 {
				t.Errorf("builder called %d times, want 1", fb.calls)
			}
		})
	}
}

func TestHandleGitPushWebhook_DistinctBodiesBothDeploy(t *testing.T) {
	rt, created := newConnectedGitSourceRouter(t, `{"repo_url":"https://github.com/org/web.git","branch":"main","build_type":"dockerfile"}`)
	var fetchCalls []fakeGitSourceFetchCall
	rt.gitSourceFetch = newFakeGitSourceFetch(&fetchCalls, t.TempDir(), new(bool), nil)
	fb := &fakeBuilder{tag: "web:sha"}
	rt.builder = fb

	for _, sha := range []string{"sha1", "sha2"} {
		b, _ := json.Marshal(map[string]string{"ref": "refs/heads/main", "after": sha})
		rec := postGitWebhook(t, rt, "X-Hub-Signature-256", sign([]byte(created.WebhookSecret), b), b, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body = %s", sha, rec.Code, rec.Body.String())
		}
	}
	if fb.calls != 2 {
		t.Errorf("builder called %d times, want 2", fb.calls)
	}
}
