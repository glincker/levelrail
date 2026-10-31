package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

func heldAttempts(t *testing.T, db *store.DB) []store.DeployAttempt {
	t.Helper()
	all, err := db.ListDeployAttempts(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	return all
}

func TestProtectedEnvironmentAutomaticPathsAwaitApproval(t *testing.T) {
	sha := strings.Repeat("a", 40)
	cases := []struct {
		name string
		run  func(rt *Router) (int, string)
	}{
		{"push event", func(rt *Router) (int, string) {
			s, m, _ := rt.admitGitPush(context.Background(), "web", webhook.PushEvent{After: sha, Ref: "refs/heads/main"})
			return s, m
		}},
		{"release tag", func(rt *Router) (int, string) {
			gs := store.GitSource{RepoURL: "https://example.com/r.git", Branch: "main", TriggerMode: "release"}
			return rt.processGitHubReleaseWebhookEvent(context.Background(), "web", gs, []byte(`{"action":"published","release":{"tag_name":"v1"}}`))
		}},
		{"build hook", func(rt *Router) (int, string) {
			res := rt.executeBuildHook(context.Background(), store.BuildHook{ID: "bh_1", ServiceName: "web", Name: "cms", Branch: "main"}, buildHookPayload{})
			if !strings.HasPrefix(res, "held: ") {
				return http.StatusInternalServerError, res
			}
			return http.StatusAccepted, res
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt, db, _ := newHoldRouter(t)
			if err := db.SaveGitSource(context.Background(), store.GitSource{ServiceName: "web", RepoURL: "https://example.com/r.git", Branch: "main"}); err != nil {
				t.Fatal(err)
			}
			rt.gitSourceSecrets = newFakeGitSourceSecrets()
			rt.resolveBranchSHA = func(context.Context, string, string, string) (string, error) { return sha, nil }
			prev := resolveTagSHA
			resolveTagSHA = func(context.Context, string, string, string) (string, error) { return sha, nil }
			t.Cleanup(func() { resolveTagSHA = prev })
			status, msg := tc.run(rt)
			if status != http.StatusAccepted || !strings.Contains(msg, "awaits approval") {
				t.Fatalf("result = %d %q, want accepted and awaiting approval", status, msg)
			}
			attempts := heldAttempts(t, db)
			if len(attempts) != 1 || attempts[0].Status != store.DeployAttemptStatusAwaitingApproval {
				t.Fatalf("attempts = %+v, want exactly one awaiting_approval", attempts)
			}
		})
	}
}

func TestUnprotectedEnvironmentIsNotHeld(t *testing.T) {
	rt, db, _ := newHoldRouter(t)
	if err := db.SetEnvironmentProtected(context.Background(), "env_prod", false); err != nil {
		t.Fatal(err)
	}
	status, msg, handled := rt.admitGitPush(context.Background(), "web", webhook.PushEvent{After: strings.Repeat("b", 40), Ref: "refs/heads/main"})
	if handled || status != 0 || msg != "" {
		t.Fatalf("admitGitPush = %d %q handled %v, want untouched fallthrough", status, msg, handled)
	}
	if n := len(heldAttempts(t, db)); n != 0 {
		t.Fatalf("attempts = %d, want none", n)
	}
}

func TestNewerHoldSupersedesOlderOne(t *testing.T) {
	rt, db, _ := newHoldRouter(t)
	for _, c := range []string{"c", "d"} {
		if s, _, _ := rt.admitGitPush(context.Background(), "web", webhook.PushEvent{After: strings.Repeat(c, 40), Ref: "refs/heads/main"}); s != http.StatusAccepted {
			t.Fatalf("hold %s status = %d", c, s)
		}
	}
	pending, err := db.ListDeployApprovals(context.Background(), store.DeployApprovalStatusPending, "web")
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %+v, %v; want one", pending, err)
	}
	awaiting, failed := 0, 0
	for _, a := range heldAttempts(t, db) {
		switch {
		case a.Status == store.DeployAttemptStatusAwaitingApproval:
			awaiting++
		case a.Status == store.DeployAttemptStatusFailed && a.Reason == "Superseded":
			failed++
		}
	}
	if awaiting != 1 || failed != 1 {
		t.Fatalf("awaiting %d superseded %d, want 1 and 1", awaiting, failed)
	}
}
