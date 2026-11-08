package azuredevops

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

const pushBody = `{"eventType":"git.push","resource":{
 "commits":[{"commitId":"bbb","comment":"fix login","author":{"name":"Jane","date":"2026-11-07T10:00:00Z"}}],
 "refUpdates":[{"name":"refs/heads/main","oldObjectId":"aaa","newObjectId":"bbb"}],
 "repository":{"name":"app","remoteUrl":"https://acme@dev.azure.com/acme/Shop/_git/app","webUrl":"https://dev.azure.com/acme/Shop/_git/app","project":{"name":"Shop"}}}}`

func TestParsePushEvent(t *testing.T) {
	if !IsPushPayload([]byte(pushBody)) || IsPushPayload([]byte(`{"ref":"refs/heads/x"}`)) {
		t.Fatal("IsPushPayload misclassified a body")
	}
	ev, err := ParsePushEvent([]byte(pushBody))
	if err != nil {
		t.Fatal(err)
	}
	if ev.Ref != "refs/heads/main" || ev.After != "bbb" || ev.Before != "aaa" || ev.HeadAuthor != "Jane" {
		t.Fatalf("event = %+v", ev)
	}
	if len(ev.RepoNames) != 1 || ev.RepoNames[0] != "Shop/app" || ev.RepoURLs[0] != "https://dev.azure.com/acme/Shop/_git/app" {
		t.Fatalf("repo identity = %v %v", ev.RepoNames, ev.RepoURLs)
	}
	tag := `{"eventType":"git.push","resource":{"refUpdates":[{"name":"refs/tags/v1","newObjectId":"ccc"}]}}`
	if _, err := ParsePushEvent([]byte(tag)); !errors.Is(err, webhook.ErrPushEventFieldsMissing) {
		t.Fatalf("tag push err = %v", err)
	}
}

func TestVerifyBasicAuth(t *testing.T) {
	good := http.Header{"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(WebhookUser+":s3"))}}
	bad := http.Header{"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(WebhookUser+":nope"))}}
	if !VerifyBasicAuth("s3", good) || VerifyBasicAuth("s3", bad) || VerifyBasicAuth("s3", http.Header{}) || VerifyBasicAuth("", good) {
		t.Fatal("basic auth verification wrong")
	}
}

func TestClientUserReposBranches(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != authHeader("pat") {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/acme/_apis/connectionData":
			_, _ = w.Write([]byte(`{"authenticatedUser":{"id":"u1","providerDisplayName":"Jane"}}`))
		case "/acme/_apis/git/repositories":
			_, _ = w.Write([]byte(`{"value":[{"id":"r1","name":"app","defaultBranch":"refs/heads/main","remoteUrl":"https://acme@dev.azure.com/acme/Shop/_git/app","project":{"id":"p1","name":"Shop","visibility":"private"}},{"id":"r2","name":"old","isDisabled":true,"project":{"name":"Shop"}}]}`))
		case "/acme/Shop/_apis/git/repositories/app/refs":
			_, _ = w.Write([]byte(`{"value":[{"name":"refs/heads/main","objectId":"bbb"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c, base := &Client{HTTP: srv.Client()}, srv.URL+"/acme"
	ctx := context.Background()

	acct, err := c.CurrentUser(ctx, base, "pat")
	if err != nil || acct.ID != "acme/u1" || acct.Login != "Jane" {
		t.Fatalf("CurrentUser = %+v, %v", acct, err)
	}
	repos, _, err := c.ListRepos(ctx, base, "pat", gitprovider.ListOptions{})
	if err != nil || len(repos) != 1 || repos[0].FullName != "Shop/app" || repos[0].DefaultBranch != "main" || repos[0].CloneURL != "https://dev.azure.com/acme/Shop/_git/app" {
		t.Fatalf("ListRepos = %+v, %v", repos, err)
	}
	bs, err := c.ListBranches(ctx, base, "pat", "Shop/app")
	if err != nil || len(bs) != 1 || bs[0].Name != "main" || bs[0].CommitSHA != "bbb" {
		t.Fatalf("ListBranches = %+v, %v", bs, err)
	}
	if _, err := c.CurrentUser(ctx, base, "wrong"); err == nil {
		t.Fatal("bad token accepted")
	}
}
