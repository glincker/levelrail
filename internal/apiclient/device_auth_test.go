package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClient_StartDeviceAuth(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody DeviceStartRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceStartResponse{
			DeviceCode:              "test-device-code",
			UserCode:                "123-456",
			VerificationURI:         "https://example.com/verify",
			VerificationURIComplete: "https://example.com/verify?code=123-456",
			ExpiresIn:               900,
			Interval:                5,
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "")
	got, err := client.StartDeviceAuth(context.Background(), DeviceStartRequest{ClientName: "test-client"})
	if err != nil {
		t.Fatalf("StartDeviceAuth() error = %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/api/v1/auth/device/start" {
		t.Errorf("request = %s %s, want POST /api/v1/auth/device/start", gotMethod, gotPath)
	}
	if gotBody.ClientName != "test-client" {
		t.Errorf("request body ClientName = %q, want %q", gotBody.ClientName, "test-client")
	}

	if got.DeviceCode != "test-device-code" || got.UserCode != "123-456" {
		t.Errorf("StartDeviceAuth() = %+v, want valid response", got)
	}
}

func TestClient_PollDeviceAuthToken_Success(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody DeviceTokenRequest
	now := time.Now().Truncate(time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{
			ID:        "token-id",
			Name:      "token-name",
			CreatedAt: now,
			Token:     "test-token",
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "")
	got, err := client.PollDeviceAuthToken(context.Background(), "test-device-code")
	if err != nil {
		t.Fatalf("PollDeviceAuthToken() error = %v", err)
	}

	if gotMethod != http.MethodPost || gotPath != "/api/v1/auth/device/token" {
		t.Errorf("request = %s %s, want POST /api/v1/auth/device/token", gotMethod, gotPath)
	}
	if gotBody.DeviceCode != "test-device-code" {
		t.Errorf("request body DeviceCode = %q, want %q", gotBody.DeviceCode, "test-device-code")
	}

	if got.Token != "test-token" || got.ID != "token-id" {
		t.Errorf("PollDeviceAuthToken() = %+v, want valid response", got)
	}
}

func TestClient_PollDeviceAuthToken_Pending(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "")
	_, err := client.PollDeviceAuthToken(context.Background(), "test-device-code")
	if err == nil {
		t.Fatal("PollDeviceAuthToken() error = nil, want an error for a pending response")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error = %T %v, want *APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("error status code = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
}
