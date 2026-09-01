package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func startDeviceAuth(t *testing.T, rt *Router, clientName string) deviceStartResponse {
	t.Helper()
	body := `{"client_name":"` + clientName + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/start", strings.NewReader(body)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("device/start status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got deviceStartResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode device/start: %v", err)
	}
	return got
}

func pollDeviceToken(t *testing.T, rt *Router, deviceCode string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"device_code":"` + deviceCode + `"}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/token", strings.NewReader(body)))
	return rec
}

func TestHandleDeviceAuthStart(t *testing.T) {
	rt, _ := newTestRouter(t)
	got := startDeviceAuth(t, rt, "my-laptop")

	if got.DeviceCode == "" || got.UserCode == "" {
		t.Fatalf("got %+v, want non-empty codes", got)
	}
	if got.ExpiresIn <= 0 || got.Interval <= 0 {
		t.Errorf("got %+v, want positive expires_in/interval", got)
	}
	if !strings.Contains(got.VerificationURI, "/settings/cli-access") {
		t.Errorf("VerificationURI = %q, want it to point at /settings/cli-access", got.VerificationURI)
	}
	if !strings.Contains(got.VerificationURIComplete, got.UserCode) {
		t.Errorf("VerificationURIComplete = %q, want it to embed the user code", got.VerificationURIComplete)
	}
}

func TestHandleDeviceAuthToken_Pending(t *testing.T) {
	rt, _ := newTestRouter(t)
	started := startDeviceAuth(t, rt, "")

	rec := pollDeviceToken(t, rt, started.DeviceCode)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "authorization_pending") {
		t.Errorf("body = %s, want authorization_pending", rec.Body.String())
	}
}

func TestHandleDeviceAuthToken_UnknownCode(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := pollDeviceToken(t, rt, "not-a-real-device-code")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "expired_token") {
		t.Errorf("body = %s, want expired_token", rec.Body.String())
	}
}

func TestDeviceAuthFlow_ApproveThenPollGrantsToken(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	started := startDeviceAuth(t, rt, "test-device")

	listRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/auth/device/requests", ""))
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}
	var pending []deviceAuthRequestResource
	if err := json.Unmarshal(listRec.Body.Bytes(), &pending); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(pending) != 1 || pending[0].UserCode != started.UserCode || pending[0].ClientName != "test-device" {
		t.Fatalf("pending = %+v, want exactly the started request", pending)
	}

	approveRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(approveRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/device/"+started.UserCode+"/approve", ""))
	if approveRec.Code != http.StatusNoContent {
		t.Fatalf("approve status = %d, body = %s", approveRec.Code, approveRec.Body.String())
	}

	pollRec := pollDeviceToken(t, rt, started.DeviceCode)
	if pollRec.Code != http.StatusOK {
		t.Fatalf("poll after approve status = %d, body = %s", pollRec.Code, pollRec.Body.String())
	}
	var granted createTokenResponse
	if err := json.Unmarshal(pollRec.Body.Bytes(), &granted); err != nil {
		t.Fatalf("decode granted token: %v", err)
	}
	if granted.Token == "" || granted.ID == "" {
		t.Fatalf("got %+v, want a real minted token", granted)
	}
	if granted.Name != "cli login: test-device" {
		t.Errorf("Name = %q, want it to include the client name", granted.Name)
	}

	// Single-use: polling again with the same device_code must not mint
	// a second token.
	secondPollRec := pollDeviceToken(t, rt, started.DeviceCode)
	if secondPollRec.Code != http.StatusBadRequest {
		t.Errorf("second poll status = %d, want %d (device_code must be single-use)", secondPollRec.Code, http.StatusBadRequest)
	}

	// The pending queue no longer lists the now-redeemed request.
	listAfterRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(listAfterRec, authedRequest(t, cookie, http.MethodGet, "/api/v1/auth/device/requests", ""))
	var afterPending []deviceAuthRequestResource
	if err := json.Unmarshal(listAfterRec.Body.Bytes(), &afterPending); err != nil {
		t.Fatalf("decode list after approve: %v", err)
	}
	if len(afterPending) != 0 {
		t.Errorf("pending after approve = %+v, want empty", afterPending)
	}
}

func TestDeviceAuthFlow_DenyThenPollDenies(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	started := startDeviceAuth(t, rt, "")

	denyRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(denyRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/device/"+started.UserCode+"/deny", ""))
	if denyRec.Code != http.StatusNoContent {
		t.Fatalf("deny status = %d, body = %s", denyRec.Code, denyRec.Body.String())
	}

	pollRec := pollDeviceToken(t, rt, started.DeviceCode)
	if pollRec.Code != http.StatusBadRequest {
		t.Fatalf("poll after deny status = %d, body = %s", pollRec.Code, pollRec.Body.String())
	}
	if !strings.Contains(pollRec.Body.String(), "access_denied") {
		t.Errorf("body = %s, want access_denied", pollRec.Body.String())
	}
}

func TestDecideDeviceAuthRequest_UnknownCode(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/device/AAAA-0000/approve", ""))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDecideDeviceAuthRequest_AlreadyDecided(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	started := startDeviceAuth(t, rt, "")

	firstRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(firstRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/device/"+started.UserCode+"/approve", ""))
	if firstRec.Code != http.StatusNoContent {
		t.Fatalf("first approve status = %d", firstRec.Code)
	}

	secondRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(secondRec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/device/"+started.UserCode+"/deny", ""))
	if secondRec.Code != http.StatusNotFound {
		t.Errorf("second decide (deny after approve) status = %d, want %d", secondRec.Code, http.StatusNotFound)
	}
}

func TestDeviceAuthRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/device/requests", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeviceAuthStart_RateLimited(t *testing.T) {
	rt, _ := newTestRouter(t)
	var lastRec *httptest.ResponseRecorder
	for i := 0; i < loginGraceFailures+2; i++ {
		lastRec = httptest.NewRecorder()
		rt.Handler().ServeHTTP(lastRec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/start", strings.NewReader(`{}`)))
	}
	if lastRec.Code != http.StatusTooManyRequests {
		t.Errorf("status after %d rapid starts = %d, want %d", loginGraceFailures+2, lastRec.Code, http.StatusTooManyRequests)
	}
}
