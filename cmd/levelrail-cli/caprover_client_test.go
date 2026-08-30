package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeCaproverRootDomain is the rootDomain every fakeCaproverServer
// reports, matched by callers that assert on default-subdomain mapping.
const fakeCaproverRootDomain = "example.com"

// fakeCaproverServer answers the login exchange with wantToken, then
// GET /user/apps/appDefinitions with apps/fakeCaproverRootDomain,
// rejecting any request whose x-captain-auth header isn't wantToken once
// issued.
func fakeCaproverServer(t *testing.T, wantPassword, wantToken string, apps []caproverAppDefinition) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-namespace") != "captain" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v2/login":
			var body struct {
				Password string `json:"password"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Password != wantPassword {
				_ = json.NewEncoder(w).Encode(caproverEnvelope{Status: 1105, Description: "Wrong password"})
				return
			}
			data, _ := json.Marshal(struct {
				Token string `json:"token"`
			}{wantToken})
			_ = json.NewEncoder(w).Encode(caproverEnvelope{Status: caproverStatusOK, Data: data})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/user/apps/appDefinitions":
			if r.Header.Get("x-captain-auth") != wantToken {
				_ = json.NewEncoder(w).Encode(caproverEnvelope{Status: 1106, Description: "Auth token invalid"})
				return
			}
			data, _ := json.Marshal(caproverListAppsData{AppDefinitions: apps, RootDomain: fakeCaproverRootDomain})
			_ = json.NewEncoder(w).Encode(caproverEnvelope{Status: caproverStatusOK, Data: data})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestCaproverClient_LoginAndListApplications(t *testing.T) {
	apps := []caproverAppDefinition{{AppName: "web", ContainerHTTPPort: 3000}}
	srv := fakeCaproverServer(t, "s3cret", "session-token-abc", apps)
	defer srv.Close()

	c := NewCaproverClient(srv.URL, "s3cret")
	if err := c.Login(context.Background()); err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if c.token != "session-token-abc" {
		t.Errorf("token = %q, want session-token-abc", c.token)
	}

	got, rootDomain, err := c.ListApplications(context.Background())
	if err != nil {
		t.Fatalf("ListApplications() error = %v", err)
	}
	if len(got) != 1 || got[0].AppName != "web" {
		t.Errorf("apps = %+v, want one app named web", got)
	}
	if rootDomain != fakeCaproverRootDomain {
		t.Errorf("rootDomain = %q, want %q", rootDomain, fakeCaproverRootDomain)
	}
}

func TestCaproverClient_LoginWrongPassword(t *testing.T) {
	srv := fakeCaproverServer(t, "s3cret", "session-token-abc", nil)
	defer srv.Close()

	c := NewCaproverClient(srv.URL, "wrong")
	err := c.Login(context.Background())
	if err == nil {
		t.Fatalf("Login() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "Wrong password") {
		t.Errorf("error = %v, want it to contain the caprover description", err)
	}
	var apiErr *caproverAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want a *caproverAPIError", err)
	}
	if apiErr.Status != 1105 {
		t.Errorf("Status = %d, want 1105", apiErr.Status)
	}
}

func TestCaproverClient_ListApplicationsWithoutLoginFails(t *testing.T) {
	srv := fakeCaproverServer(t, "s3cret", "session-token-abc", nil)
	defer srv.Close()

	c := NewCaproverClient(srv.URL, "s3cret")
	_, _, err := c.ListApplications(context.Background())
	if err == nil {
		t.Fatalf("ListApplications() error = nil, want an error when called before Login")
	}
	var apiErr *caproverAPIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %v, want a *caproverAPIError", err)
	}
	if apiErr.Status != 1106 {
		t.Errorf("Status = %d, want 1106 (auth token invalid)", apiErr.Status)
	}
}
