package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleConnectGitHubAppManually_Success(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), nil)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"app_id": 12345,
		"client_id": "Iv1.abc123",
		"client_secret": "shhh-client-secret",
		"webhook_secret": "shhh-webhook-secret",
		"private_key": ` + jsonString(testRSAPrivateKeyPEM(t)) + `
	}`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/github-app/manual", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got gitHubAppStatusResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Connected || got.AppID != 12345 || got.ClientID != "Iv1.abc123" {
		t.Errorf("got = %+v, want connected app 12345/Iv1.abc123", got)
	}

	conn, err := db.GetGitHubAppConnection(context.Background())
	if err != nil {
		t.Fatalf("GetGitHubAppConnection() error = %v", err)
	}
	if conn.AppID != 12345 || conn.ClientID != "Iv1.abc123" {
		t.Errorf("stored connection = %+v, want app 12345/Iv1.abc123", conn)
	}
}

func TestHandleConnectGitHubAppManually_GHEInstanceURL(t *testing.T) {
	fakeClient := &fakeGitHubAppClient{}
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), fakeClient)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"app_id": 12345,
		"client_id": "Iv1.abc123",
		"client_secret": "shhh-client-secret",
		"webhook_secret": "shhh-webhook-secret",
		"instance_url": "https://ghe.example.com",
		"private_key": ` + jsonString(testRSAPrivateKeyPEM(t)) + `
	}`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/github-app/manual", body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if fakeClient.gotInstanceURL != "https://ghe.example.com" {
		t.Errorf("CheckInstanceReachable called with instanceURL = %q, want the GHE instance", fakeClient.gotInstanceURL)
	}

	conn, err := db.GetGitHubAppConnection(context.Background())
	if err != nil {
		t.Fatalf("GetGitHubAppConnection() error = %v", err)
	}
	if conn.InstanceURL != "https://ghe.example.com" {
		t.Errorf("stored connection InstanceURL = %q, want the GHE instance", conn.InstanceURL)
	}
}

func TestHandleConnectGitHubAppManually_GHEInstanceUnreachable(t *testing.T) {
	fakeClient := &fakeGitHubAppClient{reachableErr: errors.New("connection refused")}
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), fakeClient)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"app_id": 12345,
		"client_id": "Iv1.abc123",
		"client_secret": "shhh-client-secret",
		"webhook_secret": "shhh-webhook-secret",
		"instance_url": "https://ghe.example.com",
		"private_key": ` + jsonString(testRSAPrivateKeyPEM(t)) + `
	}`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/github-app/manual", body))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadGateway, rec.Body.String())
	}
	if _, err := db.GetGitHubAppConnection(context.Background()); !errors.Is(err, store.ErrGitHubAppConnectionNotFound) {
		t.Error("connection was saved despite an unreachable instance")
	}
}

func TestHandleConnectGitHubAppManually_InvalidInstanceURL(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), nil)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"app_id": 12345,
		"client_id": "Iv1.abc123",
		"client_secret": "shhh-client-secret",
		"webhook_secret": "shhh-webhook-secret",
		"instance_url": "not-a-url",
		"private_key": ` + jsonString(testRSAPrivateKeyPEM(t)) + `
	}`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/github-app/manual", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestHandleConnectGitHubAppManually_MissingMasterKey(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, nil, nil)
	rt.githubAppSecrets = nil
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/github-app/manual", `{"app_id":1}`))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleConnectGitHubAppManually_InvalidPrivateKey(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), nil)
	cookie := loginTestSession(t, rt, db)

	body := `{
		"app_id": 12345,
		"client_id": "Iv1.abc123",
		"client_secret": "shhh",
		"webhook_secret": "shhh",
		"private_key": "not a real pem"
	}`

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/github-app/manual", body))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "private_key") {
		t.Errorf("body = %s, want it to mention private_key", rec.Body.String())
	}
}

func TestHandleConnectGitHubAppManually_MissingFields(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), nil)
	cookie := loginTestSession(t, rt, db)

	cases := []struct {
		name string
		body string
	}{
		{"no app_id", `{"client_id":"x","client_secret":"x","webhook_secret":"x","private_key":"x"}`},
		{"no client_id", `{"app_id":1,"client_secret":"x","webhook_secret":"x","private_key":"x"}`},
		{"no client_secret", `{"app_id":1,"client_id":"x","webhook_secret":"x","private_key":"x"}`},
		{"no webhook_secret", `{"app_id":1,"client_id":"x","client_secret":"x","private_key":"x"}`},
		{"no private_key", `{"app_id":1,"client_id":"x","client_secret":"x","webhook_secret":"x"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/github-app/manual", tc.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
		})
	}
}

func TestGitHubAppManualRoute_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/github-app/manual", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetGitHubAppStatus_IncludesBaseURL(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), nil)
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got gitHubAppStatusResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := "https://" + testPrimaryDomain
	if got.BaseURL != want {
		t.Errorf("BaseURL = %q, want %q", got.BaseURL, want)
	}
}

func TestHandleGetGitHubAppStatus_EmptyBaseURL_WhenNoPrimaryDomain(t *testing.T) {
	rt, db := newTestRouterWithGitHubApp(t, newFakeGitHubAppSecrets(), nil)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/github-app", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got gitHubAppStatusResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty (no primary domain configured)", got.BaseURL)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
