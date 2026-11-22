package doauth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestChallengeMatchesVerifier(t *testing.T) {
	v, err := NewVerifier()
	if err != nil || len(v) < 43 {
		t.Fatalf("verifier = %q %v", v, err)
	}
	if Challenge(v) == "" || Challenge(v) == v {
		t.Fatal("challenge is not a stable transform")
	}
}

func TestAuthorizeURLCarriesPKCE(t *testing.T) {
	u, err := url.Parse(Client{Base: "https://example.test/"}.AuthorizeURL("cid", "https://cb.test/x", "st", "chal"))
	if err != nil || u.Host != "example.test" || u.Path != "/v1/oauth/authorize" {
		t.Fatalf("url = %v %v", u, err)
	}
	q := u.Query()
	if q.Get("state") != "st" || q.Get("code_challenge") != "chal" || q.Get("code_challenge_method") != "S256" || q.Get("response_type") != "code" {
		t.Fatalf("query = %v", q)
	}
}

func fakeServer(t *testing.T, status int, body any) (*httptest.Server, *url.Values) {
	t.Helper()
	got := &url.Values{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		*got = r.Form
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

func TestExchangeAndRefresh(t *testing.T) {
	srv, got := fakeServer(t, http.StatusOK, map[string]any{"access_token": "a", "refresh_token": "r", "expires_in": 60})
	c := Client{Base: srv.URL, Timeout: time.Second}
	tok, err := c.Exchange(context.Background(), "cid", "sec", "https://cb.test", "code", "ver")
	if err != nil || tok.Access != "a" || tok.Refresh != "r" || tok.ExpiresIn != time.Minute {
		t.Fatalf("exchange = %+v %v", tok, err)
	}
	if got.Get("grant_type") != "authorization_code" || got.Get("code_verifier") != "ver" {
		t.Fatalf("form = %v", *got)
	}
	if _, err := c.Refresh(context.Background(), "cid", "sec", "r"); err != nil || got.Get("grant_type") != "refresh_token" {
		t.Fatalf("refresh form = %v %v", *got, err)
	}
}

func TestRejectedAndUnusableResponses(t *testing.T) {
	srv, _ := fakeServer(t, http.StatusBadRequest, map[string]any{})
	if _, err := (Client{Base: srv.URL}).Refresh(context.Background(), "c", "s", "r"); !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v", err)
	}
	empty, _ := fakeServer(t, http.StatusOK, map[string]any{"refresh_token": "r"})
	if _, err := (Client{Base: empty.URL}).Refresh(context.Background(), "c", "s", "r"); err == nil || strings.Contains(err.Error(), "r\"") {
		t.Fatalf("err = %v", err)
	}
}

func TestRevokeStatus(t *testing.T) {
	srv, got := fakeServer(t, http.StatusOK, map[string]any{})
	if err := (Client{Base: srv.URL}).Revoke(context.Background(), "tok"); err != nil || got.Get("token") != "tok" {
		t.Fatalf("revoke = %v form %v", err, *got)
	}
	bad, _ := fakeServer(t, http.StatusInternalServerError, map[string]any{})
	if err := (Client{Base: bad.URL}).Revoke(context.Background(), "tok"); err == nil {
		t.Fatal("expected a status error")
	}
}
