package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRun_ChannelsList(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]notificationChannelResource{
			{ID: "chn_1", Name: "Team Slack", Kind: "slack", Enabled: true, CreatedAt: "2026-08-15T00:00:00Z"},
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"channels", "list", "--api-url", srv.URL}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/notification-channels" {
		t.Errorf("path = %q, want /api/v1/notification-channels", gotPath)
	}
	if !strings.Contains(stdout.String(), "chn_1") {
		t.Errorf("stdout = %q, want the channel id listed", stdout.String())
	}
}

func TestRun_ChannelsCreate_NotifyURL(t *testing.T) {
	var gotPath string
	var gotBody createNotificationChannelRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(notificationChannelResource{
			ID: "chn_2", Name: gotBody.Name, Kind: gotBody.Kind, NotifyURL: gotBody.NotifyURL, Enabled: *gotBody.Enabled,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"channels", "create", "--name", "Team Slack", "--kind", "slack",
		"--notify-url", "https://hooks.slack.com/x", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotPath != "/api/v1/notification-channels" {
		t.Errorf("path = %q, want /api/v1/notification-channels", gotPath)
	}
	if gotBody.Kind != "slack" || gotBody.NotifyURL != "https://hooks.slack.com/x" {
		t.Errorf("request body = %+v, want kind slack and the given notify_url", gotBody)
	}
	if gotBody.Enabled == nil || !*gotBody.Enabled {
		t.Errorf("request Enabled = %v, want true (default)", gotBody.Enabled)
	}
}

// TestRun_ChannelsCreate_PushoverFlags proves the two convenience flags
// build the same query-string notify_url the backend's
// parsePushoverCreds expects, without the caller hand-constructing it.
func TestRun_ChannelsCreate_PushoverFlags(t *testing.T) {
	var gotBody createNotificationChannelRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(notificationChannelResource{
			ID: "chn_3", Name: gotBody.Name, Kind: gotBody.Kind, NotifyURL: gotBody.NotifyURL, Enabled: *gotBody.Enabled,
		})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"channels", "create", "--name", "My Phone", "--kind", "pushover",
		"--pushover-user-key", "user-key-456", "--pushover-api-token", "app-token-123",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}

	parsed, err := url.Parse(gotBody.NotifyURL)
	if err != nil {
		t.Fatalf("parse built notify_url %q: %v", gotBody.NotifyURL, err)
	}
	if parsed.Scheme+"://"+parsed.Host+parsed.Path != pushoverEndpoint {
		t.Errorf("notify_url endpoint = %q, want %q", parsed.Scheme+"://"+parsed.Host+parsed.Path, pushoverEndpoint)
	}
	if parsed.Query().Get("token") != "app-token-123" {
		t.Errorf("notify_url token param = %q, want app-token-123", parsed.Query().Get("token"))
	}
	if parsed.Query().Get("user") != "user-key-456" {
		t.Errorf("notify_url user param = %q, want user-key-456", parsed.Query().Get("user"))
	}
}

func TestRun_ChannelsCreate_MissingDestination(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"channels", "create", "--name", "x", "--kind", "pushover"}, &stdout, &stderr, envMap(nil))
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--notify-url is required") {
		t.Errorf("stderr = %q, want a missing destination error", stderr.String())
	}
}

func TestRun_ChannelsDelete(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"channels", "delete", "chn_1", "--api-url", srv.URL}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/v1/notification-channels/chn_1" {
		t.Errorf("request = %s %s, want DELETE /api/v1/notification-channels/chn_1", gotMethod, gotPath)
	}
}

func TestRun_ChannelsTest(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"channels", "test", "chn_1", "--api-url", srv.URL}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/notification-channels/chn_1/test" {
		t.Errorf("request = %s %s, want POST /api/v1/notification-channels/chn_1/test", gotMethod, gotPath)
	}
}

func TestRun_Channels_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"channels", "-h"}, &stdout, &stderr, envMap(nil))
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "channels list") || !strings.Contains(stdout.String(), "pushover") {
		t.Errorf("stdout = %q, want usage text mentioning pushover", stdout.String())
	}
}
