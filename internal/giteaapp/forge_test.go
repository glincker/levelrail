package giteaapp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/glincker/levelrail/internal/gitprovider"
)

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	mac := hmac.New(sha256.New, []byte("s3"))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	if !VerifySignature("s3", body, sig) || !VerifySignature("s3", body, "sha256="+sig) {
		t.Fatal("valid signature rejected")
	}
	if VerifySignature("other", body, sig) || VerifySignature("s3", body, "zz") || VerifySignature("", body, sig) {
		t.Fatal("bad signature accepted")
	}
}

func TestEnsureRepoWebhookUpdatesInPlace(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[{"id":7,"active":true,"events":["push"],"config":{"url":"https://x/hook"}}]`))
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"active":true,"events":["push","pull_request"],"config":{"url":"https://x/hook"}}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	h, err := c.EnsureRepoWebhook(context.Background(), srv.URL, "t", "a/b", "https://x/hook", "sec")
	if err != nil || h.ID != 7 {
		t.Fatalf("got %+v, %v", h, err)
	}
	if len(methods) != 2 || methods[1] != "PATCH /api/v1/repos/a/b/hooks/7" {
		t.Fatalf("calls = %v, want a PATCH of the existing hook", methods)
	}
}

func TestCreateOAuthApp(t *testing.T) {
	var got createOAuthAppRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user/applications/oauth2" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":3,"name":"n","client_id":"cid","client_secret":"sec"}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	app, err := c.CreateOAuthApp(context.Background(), srv.URL, "t", "n", "https://x/cb")
	if err != nil || app.ClientID != "cid" || app.Secret != "sec" || app.ID != 3 {
		t.Fatalf("got %+v, %v", app, err)
	}
	if !got.Confidential || len(got.RedirectURIs) != 1 {
		t.Fatalf("request = %+v", got)
	}
}

func TestListOwnerReposFallsBackToUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/orgs/bob/repos" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`[{"full_name":"bob/r","owner":{"login":"bob"},"permissions":{"admin":true,"push":true}}]`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	rs, _, err := c.ListOwnerRepos(context.Background(), srv.URL, "t", "bob", gitprovider.ListOptions{})
	if err != nil || len(rs) != 1 || !rs[0].Admin || rs[0].Owner != "bob" {
		t.Fatalf("got %+v, %v", rs, err)
	}
}
