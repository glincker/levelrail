package giteaapp

import (
	"net/url"
	"testing"
)

func TestAuthorizeURL(t *testing.T) {
	got := AuthorizeURL("https://gitea.example.com/", "client-1", "https://cp.example.com/api/v1/gitea-app/callback", "state-1")

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", got, err)
	}
	if u.Scheme+"://"+u.Host+u.Path != "https://gitea.example.com/login/oauth/authorize" {
		t.Errorf("base = %q, want https://gitea.example.com/login/oauth/authorize (trailing slash trimmed)", u.Scheme+"://"+u.Host+u.Path)
	}
	q := u.Query()
	if q.Get("client_id") != "client-1" {
		t.Errorf("client_id = %q, want client-1", q.Get("client_id"))
	}
	if q.Get("redirect_uri") != "https://cp.example.com/api/v1/gitea-app/callback" {
		t.Errorf("redirect_uri = %q, unexpected", q.Get("redirect_uri"))
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q, want code", q.Get("response_type"))
	}
	if q.Get("state") != "state-1" {
		t.Errorf("state = %q, want state-1", q.Get("state"))
	}
	if q.Get("scope") != DefaultScope {
		t.Errorf("scope = %q, want %q", q.Get("scope"), DefaultScope)
	}
}
