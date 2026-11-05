package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

var forkCanaries = []string{
	"canary-prod-env-value", "CANARY_SECRET_NAME", "canary/vault/path", "canary-parent-volume",
	"/canary/host/path", "/canary/file", "canary-registry-cred", "CANARY_PROD_ENV",
}

func seedParentWithCanaries(t *testing.T, db *store.DB) []byte {
	t.Helper()
	ctx := context.Background()
	err := db.SaveDesiredService(ctx, store.DesiredService{
		Name: "web", Image: "img:v1", Port: 3000,
		Env:                  map[string]string{"CANARY_PROD_ENV": "canary-prod-env-value"},
		SecretEnv:            []store.SecretEnvRef{{Name: "CANARY_SECRET_NAME"}},
		VaultEnv:             map[string]store.VaultEnvRef{"V": {Path: "canary/vault/path", Key: "k"}},
		Volumes:              []store.ServiceVolume{{Name: "canary-parent-volume", ContainerPath: "/data"}},
		BindMounts:           []store.ServiceBindMount{{HostPath: "/canary/host/path", ContainerPath: "/h"}},
		Files:                []store.ServiceFile{{Path: "/canary/file", Content: "x"}},
		RegistryCredentialID: "canary-registry-cred",
	})
	if err != nil {
		t.Fatal(err)
	}
	return parentSnapshot(t, db)
}

func parentSnapshot(t *testing.T, db *store.DB) []byte {
	t.Helper()
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(svc)
	return b
}

func approveFork(t *testing.T, rt *Router, db *store.DB, pr int) {
	t.Helper()
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, fmt.Sprintf("/api/v1/apps/web/previews/%d/approve", pr), `{"confirm":true}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("approve: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rt.previewDeploys.Wait()
}

// Today an unapproved fork is held, and an approved one still gets no env, secrets, vault refs, volumes,
// mounts, files or registry credential from the parent, and the parent row is never rewritten.
func TestForkPreview_ApprovedForkGetsNothingFromTheParentApp(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)
	before := seedParentWithCanaries(t, db)

	sendPullRequestWebhook(rt, secret, githubPullRequestBodyFrom("opened", 42, "sha1", "main", "mallory/web"))
	if len(builder.calls) != 0 {
		t.Fatalf("builds before approval = %d, want 0", len(builder.calls))
	}
	approveFork(t, rt, db, 42)
	if len(builder.calls) != 1 {
		t.Fatalf("builds after approval = %d, want 1", len(builder.calls))
	}

	req := builder.calls[0]
	if !req.Preview || req.ServiceName != "web-pr-42" || req.ImageRepo != "web-pr-42" {
		t.Errorf("request = preview %v name %q repo %q, want a preview build under web-pr-42", req.Preview, req.ServiceName, req.ImageRepo)
	}
	dump := fmt.Sprintf("%+v", req)
	for _, c := range forkCanaries {
		if strings.Contains(dump, c) {
			t.Errorf("approved fork build request carries parent config %q", c)
		}
	}
	for k := range req.Service.Env {
		// PR_NUMBER is the platform alias of the webhook's PR number, checked by value below.
		if !strings.HasPrefix(k, "PREVIEW_") && k != "PR_NUMBER" {
			t.Errorf("approved fork inherited env %q", k)
		}
	}
	if got, ok := req.Service.Env["PR_NUMBER"]; ok && got.Value != "42" {
		t.Errorf("approved fork PR_NUMBER = %q, want the pull request number 42", got.Value)
	}
	if after := parentSnapshot(t, db); string(after) != string(before) {
		t.Errorf("parent app changed by a fork preview:\nbefore %s\nafter  %s", before, after)
	}
}

// Each payload shape is a fork by repository identity, whatever the provider's own fork flag says.
func TestForkPreview_PayloadShapesNeverBuildUnapproved(t *testing.T) {
	pr := func(head any, extra map[string]any) []byte {
		headObj := map[string]any{"ref": "feature-x", "sha": "sha1"}
		if head != nil {
			headObj["repo"] = head
		}
		for k, v := range extra {
			headObj[k] = v
		}
		b, _ := json.Marshal(map[string]any{
			"action": "opened", "number": 42,
			"pull_request": map[string]any{
				"head": headObj,
				"base": map[string]any{"ref": "main", "repo": map[string]any{"full_name": "org/web", "fork": false}},
			},
		})
		return b
	}
	cases := map[string][]byte{
		"fork flag true, other owner":         pr(map[string]any{"full_name": "mallory/web", "fork": true}, nil),
		"fork flag false lies, other owner":   pr(map[string]any{"full_name": "mallory/web", "fork": false}, nil),
		"other owner, same repo name casing":  pr(map[string]any{"full_name": "ORG-evil/WEB", "fork": true}, nil),
		"deleted head repository":             pr(nil, nil),
		"head repo without a name":            pr(map[string]any{"fork": true}, nil),
		"owner lookalike with trailing space": pr(map[string]any{"full_name": "org /web"}, nil),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rt, db, secret, builder := setUpPreviewApp(t)
			before := seedParentWithCanaries(t, db)
			rec := sendPullRequestWebhook(rt, secret, body)
			if rec.Code != http.StatusOK || len(builder.calls) != 0 {
				t.Fatalf("status = %d, builds = %d, want a held preview and no build", rec.Code, len(builder.calls))
			}
			row, err := db.GetPreviewEnvironmentByAppAndPR(context.Background(), "web", 42)
			if err != nil || row.Status != store.PreviewStatusAwaitingApproval {
				t.Fatalf("row = %+v, err = %v, want awaiting_approval", row, err)
			}
			if string(parentSnapshot(t, db)) != string(before) {
				t.Error("parent app changed by an unapproved fork event")
			}
		})
	}
}

// Control: the same harness deploys a same-repository pull request, so the fork cases above are not
// passing only because previews are broken.
func TestForkPreview_SameRepositoryControlDeploys(t *testing.T) {
	rt, _, secret, builder := setUpPreviewApp(t)
	sendPullRequestWebhook(rt, secret, githubPullRequestBodyFrom("opened", 42, "sha1", "main", "org/web"))
	if len(builder.calls) != 1 {
		t.Fatalf("same-repo builds = %d, want 1", len(builder.calls))
	}
}

// A fork event only ever creates or touches web-pr-N rows; it must not deploy another environment.
func TestForkPreview_ApprovedForkCreatesOnlyItsPreviewService(t *testing.T) {
	rt, db, secret, builder := setUpPreviewApp(t)
	sendPullRequestWebhook(rt, secret, githubPullRequestBodyFrom("opened", 42, "sha1", "main", "mallory/web"))
	approveFork(t, rt, db, 42)
	for _, c := range builder.calls {
		if c.ServiceName != "web-pr-42" {
			t.Errorf("fork preview built %q, want only web-pr-42", c.ServiceName)
		}
	}
	svc, err := db.GetDesiredService(context.Background(), "web")
	if err != nil || svc.Image != "img:v1" {
		t.Errorf("parent image = %q, err = %v, want it untouched", svc.Image, err)
	}
}
