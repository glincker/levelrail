package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"

	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
)

const gitValidateToken = "ghp_supersecrettokenvalue123456"

func validateGit(t *testing.T, rt *Router, cookie *http.Cookie, target string) (int, gitAccessResult, string) {
	t.Helper()
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, target, ""))
	var out gitAccessResult
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out, rec.Body.String()
}

func TestGitSourceValidate_TypedFixes(t *testing.T) {
	tests := []struct {
		name     string
		repoURL  string
		withTok  bool
		resolve  func(context.Context, string, string, string) (string, error)
		wantOK   bool
		wantFix  string
		wantCred string
	}{
		{"public repo reads", "https://github.com/o/r.git", false, func(context.Context, string, string, string) (string, error) { return "abc123", nil }, true, "", gitAuthNone},
		{"private repo no credential", "https://github.com/o/r.git", false, func(context.Context, string, string, string) (string, error) {
			return "", transport.ErrAuthenticationRequired
		}, false, gitFixNoAuth, gitAuthNone},
		{"expired token", "https://github.com/o/r.git", true, func(context.Context, string, string, string) (string, error) {
			return "", transport.ErrAuthorizationFailed
		}, false, gitFixAuthDenied, gitAuthDeployToken},
		{"token cannot see repo", "https://github.com/o/r.git", true, func(context.Context, string, string, string) (string, error) {
			return "", transport.ErrRepositoryNotFound
		}, false, gitFixRepoNotFound, gitAuthDeployToken},
		{"branch missing", "https://github.com/o/r.git", true, func(context.Context, string, string, string) (string, error) {
			return "", fmt.Errorf("x: %w", errRemoteBranchNotFound)
		}, false, gitFixBranchMissing, gitAuthDeployToken},
		{"host down", "https://github.com/o/r.git", false, func(context.Context, string, string, string) (string, error) {
			return "", errors.New("dial tcp: connection refused")
		}, false, gitFixUnreachable, gitAuthNone},
		{"bad url", "ftp://example.com/r.git", false, nil, false, gitFixBadRepoURL, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			secrets := newFakeGitSourceSecrets()
			rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
			cookie := loginTestSession(t, rt, db)
			seedApp(t, db, "web")
			ctx := context.Background()
			if err := db.SaveGitSource(ctx, store.GitSource{ServiceName: "web", RepoURL: tc.repoURL, Branch: "main"}); err != nil {
				t.Fatal(err)
			}
			if tc.withTok {
				_ = secrets.SetValue(ctx, store.GitSourceSecretsKey("web"), gitSourceTokenKey, gitValidateToken)
			}
			if tc.resolve != nil {
				rt.resolveBranchSHA = tc.resolve
			}
			code, res, body := validateGit(t, rt, cookie, "/api/v1/apps/web/git-source/validate")
			if code != http.StatusOK {
				t.Fatalf("status = %d: %s", code, body)
			}
			if res.OK != tc.wantOK {
				t.Fatalf("ok = %v, want %v: %s", res.OK, tc.wantOK, body)
			}
			if tc.wantFix != "" && (res.Fix == nil || res.Fix.Code != tc.wantFix) {
				t.Fatalf("fix = %+v, want code %s", res.Fix, tc.wantFix)
			}
			if tc.wantFix == "" && res.Fix != nil {
				t.Fatalf("unexpected fix %+v", res.Fix)
			}
			if tc.wantCred != "" && res.Credential != tc.wantCred {
				t.Fatalf("credential = %q, want %q", res.Credential, tc.wantCred)
			}
			if strings.Contains(body, gitValidateToken) {
				t.Fatalf("response leaked the token: %s", body)
			}
		})
	}
}

func TestGitSourceValidate_NeverLogsCredential(t *testing.T) {
	var logs bytes.Buffer
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	rt.logger = slog.New(secretmask.Handler(slog.NewTextHandler(&logs, nil)))
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	ctx := context.Background()
	_ = db.SaveGitSource(ctx, store.GitSource{ServiceName: "web", RepoURL: "https://user:" + gitValidateToken + "@github.com/o/r.git", Branch: "main"})
	_ = secrets.SetValue(ctx, store.GitSourceSecretsKey("web"), gitSourceTokenKey, gitValidateToken)
	secretmask.Add(gitValidateToken)
	rt.resolveBranchSHA = func(_ context.Context, repoURL, _, token string) (string, error) {
		return "", fmt.Errorf("remote said no to %s with %s", repoURL, token)
	}
	_, _, body := validateGit(t, rt, cookie, "/api/v1/apps/web/git-source/validate")
	if strings.Contains(body, gitValidateToken) || strings.Contains(logs.String(), gitValidateToken) {
		t.Fatalf("credential leaked.\nbody: %s\nlogs: %s", body, logs.String())
	}
}

func TestGitSourceValidate_NoSourceAndAuth(t *testing.T) {
	rt, db := newTestRouterWithGitSourceSecrets(t, newFakeGitSourceSecrets())
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	if code, _, _ := validateGit(t, rt, cookie, "/api/v1/apps/web/git-source/validate"); code != http.StatusNotFound {
		t.Fatalf("no source = %d, want 404", code)
	}
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodPost, "/api/v1/apps/web/git-source/validate"},
		{http.MethodPost, "/api/v1/git-providers/github/validate"},
	})
}

func TestGitHubProviderValidate_MasksMintedToken(t *testing.T) {
	const minted = "ghs_mintedinstallationtoken9999"
	fakeSecrets := newFakeGitHubAppSecrets()
	fakeClient := &fakeGitHubAppClient{mintToken: githubapp.InstallationToken{Token: minted}}
	rt, db := newTestRouterWithGitHubApp(t, fakeSecrets, fakeClient)
	cookie := loginTestSession(t, rt, db)
	seedInstalledGitHubApp(t, db, fakeSecrets)

	code, res, body := validateGit(t, rt, cookie, "/api/v1/git-providers/github/validate")
	if code != http.StatusOK || !res.OK {
		t.Fatalf("validate = %d ok=%v body=%s", code, res.OK, body)
	}
	if strings.Contains(body, minted) {
		t.Fatalf("response leaked the installation token: %s", body)
	}
	if got := secretmask.Mask("clone failed with " + minted); strings.Contains(got, minted) {
		t.Fatalf("minted installation token is not registered with secretmask: %q", got)
	}

	fakeClient.mintErr = errors.New("401 bad credentials")
	_, res, _ = validateGit(t, rt, cookie, "/api/v1/git-providers/github/validate")
	if res.OK || res.Fix == nil || res.Fix.Code != gitFixKeyRejected {
		t.Fatalf("rejected key = ok=%v fix=%+v, want %s", res.OK, res.Fix, gitFixKeyRejected)
	}
}

func TestGitProviderValidate(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, p := range []string{"github", "gitlab", "gitea", "bitbucket"} {
		code, res, body := validateGit(t, rt, cookie, "/api/v1/git-providers/"+p+"/validate")
		if code != http.StatusOK || res.OK || res.Fix == nil || res.Fix.Code != gitFixNotConnected {
			t.Errorf("%s unconnected = %d ok=%v fix=%+v body=%s", p, code, res.OK, res.Fix, body)
		}
	}
	if code, _, _ := validateGit(t, rt, cookie, "/api/v1/git-providers/svn/validate"); code != http.StatusNotFound {
		t.Fatalf("unknown provider = %d, want 404", code)
	}
}
