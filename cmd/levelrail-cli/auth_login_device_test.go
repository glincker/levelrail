package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPollDeviceAuthUntilGranted_PendingThenApproved(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls < 3 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(deviceTokenResponse{ID: "tok_1", Name: "cli login", Token: "plaintext-value"})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "")
	var slept []time.Duration
	fakeSleep := func(d time.Duration) { slept = append(slept, d) }

	got, err := pollDeviceAuthUntilGranted(context.Background(), client, "device-abc", time.Millisecond, time.Hour, fakeSleep)
	if err != nil {
		t.Fatalf("pollDeviceAuthUntilGranted() error = %v", err)
	}
	if got.Token != "plaintext-value" || got.ID != "tok_1" {
		t.Errorf("got %+v", got)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (two pending, one granted)", calls)
	}
	if len(slept) != 3 {
		t.Errorf("slept %d times, want 3 (one per poll)", len(slept))
	}
}

func TestPollDeviceAuthUntilGranted_Denied(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"access_denied"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "")
	_, err := pollDeviceAuthUntilGranted(context.Background(), client, "device-abc", time.Millisecond, time.Hour, func(time.Duration) {})
	if err == nil || !strings.Contains(err.Error(), "denied") {
		t.Errorf("error = %v, want a denial message", err)
	}
}

func TestPollDeviceAuthUntilGranted_ExpiredFromServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"expired_token"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "")
	_, err := pollDeviceAuthUntilGranted(context.Background(), client, "device-abc", time.Millisecond, time.Hour, func(time.Duration) {})
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %v, want an expiry message", err)
	}
}

func TestPollDeviceAuthUntilGranted_LocalTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "")
	// timeout <= 0: the very first deadline check fails immediately, no
	// real request is ever made.
	_, err := pollDeviceAuthUntilGranted(context.Background(), client, "device-abc", time.Millisecond, 0, func(time.Duration) {})
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Errorf("error = %v, want a local expiry message", err)
	}
}

func TestRun_AuthLoginDevice_FullFlow(t *testing.T) {
	var startCalls, pollCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/device/start":
			startCalls++
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(deviceStartResponse{
				DeviceCode: "device-abc", UserCode: "AAAA-1111",
				VerificationURI: "http://example.invalid/settings/cli-access", ExpiresIn: 600, Interval: 1,
			})
		case "/api/v1/auth/device/token":
			// Granted on the very first poll (no pending bounce): this
			// test only needs to exercise one real 1-second sleep, since
			// runAuthLoginDevice uses the real time.Sleep, not the
			// injectable one pollDeviceAuthUntilGranted's own tests use.
			pollCalls++
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(deviceTokenResponse{ID: "tok_1", Name: "cli login", Token: "plaintext-value"})
		default:
			t.Fatalf("unexpected request to %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	t.Setenv("HOME", t.TempDir())

	var stdout, stderr strings.Builder
	got := runAuthLogin("levelrail-cli-test", []string{"--device", "--api-url", srv.URL}, &stdout, &stderr, envMap(), strings.NewReader(""))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if startCalls != 1 {
		t.Errorf("device/start called %d times, want 1", startCalls)
	}
	if !strings.Contains(stdout.String(), "plaintext-value") {
		t.Errorf("stdout = %q, want it to show the minted token once", stdout.String())
	}
	if !strings.Contains(stderr.String(), "AAAA-1111") {
		t.Errorf("stderr = %q, want it to show the user code to approve", stderr.String())
	}

	creds, err := readCredentialsFile("levelrail-cli-test", "default")
	if err != nil {
		t.Fatalf("readCredentialsFile() error = %v", err)
	}
	if creds.Token != "plaintext-value" {
		t.Errorf("saved token = %q, want plaintext-value", creds.Token)
	}
}
