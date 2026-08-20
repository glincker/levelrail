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

// newChannelCreateEchoServer starts a test server that decodes a create-channel
// request into gotBody and echoes it back as the created channel resource.
// gotPath may be nil when the caller doesn't need the request path.
func newChannelCreateEchoServer(t *testing.T, id string, gotBody *createNotificationChannelRequest, gotPath *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotPath != nil {
			*gotPath = r.URL.Path
		}
		if err := json.NewDecoder(r.Body).Decode(gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(notificationChannelResource{
			ID: id, Name: gotBody.Name, Kind: gotBody.Kind, NotifyURL: gotBody.NotifyURL, Enabled: *gotBody.Enabled,
		})
	}))
}

func TestRun_ChannelsList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []notificationChannelResource{
		{ID: "chn_1", Name: "Team Slack", Kind: "slack", Enabled: true, CreatedAt: "2026-08-15T00:00:00Z"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"channels", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/notification-channels" {
		t.Errorf("path = %q, want /api/v1/notification-channels", gotPath)
	}
	if !strings.Contains(stdout, "chn_1") {
		t.Errorf("stdout = %q, want the channel id listed", stdout)
	}
}

func TestRun_ChannelsCreate_NotifyURL(t *testing.T) {
	var gotPath string
	var gotBody createNotificationChannelRequest
	srv := newChannelCreateEchoServer(t, "chn_2", &gotBody, &gotPath)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"channels", "create", "--name", "Team Slack", "--kind", "slack",
		"--notify-url", "https://hooks.slack.com/x", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
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
	srv := newChannelCreateEchoServer(t, "chn_3", &gotBody, nil)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"channels", "create", "--name", "My Phone", "--kind", "pushover",
		"--pushover-user-key", "user-key-456", "--pushover-api-token", "app-token-123",
		"--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
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

// assertChannelsCreate runs "channels create" with flags against a fresh
// echo server and asserts the request body's kind and notify_url.
func assertChannelsCreate(t *testing.T, id string, flags []string, wantKind, wantNotifyURL string) {
	t.Helper()
	var gotBody createNotificationChannelRequest
	srv := newChannelCreateEchoServer(t, id, &gotBody, nil)
	defer srv.Close()

	args := append(append([]string{"channels", "create"}, flags...), "--api-url", srv.URL)
	runCLIExpectOK(t, args)
	if gotBody.Kind != wantKind || gotBody.NotifyURL != wantNotifyURL {
		t.Errorf("request body = %+v, want kind %s and notify_url %s", gotBody, wantKind, wantNotifyURL)
	}
}

// TestRun_ChannelsCreate_PagerDutyRoutingKeyFlag proves
// --pagerduty-routing-key is sent through as notify_url unchanged, the
// backend's own convention for a pagerduty channel.
func TestRun_ChannelsCreate_PagerDutyRoutingKeyFlag(t *testing.T) {
	assertChannelsCreate(t, "chn_4",
		[]string{"--name", "Oncall", "--kind", "pagerduty", "--pagerduty-routing-key", "routing-key-789"},
		"pagerduty", "routing-key-789")
}

func TestRun_ChannelsCreate_TeamsWebhookURL(t *testing.T) {
	assertChannelsCreate(t, "chn_5",
		[]string{"--name", "Ops Teams", "--kind", "teams", "--notify-url", "https://example.webhook.office.com/x"},
		"teams", "https://example.webhook.office.com/x")
}

func TestRun_ChannelsCreate_MissingDestination(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"channels", "create", "--name", "x", "--kind", "pushover"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitValidation, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--notify-url is required") {
		t.Errorf("stderr = %q, want a missing destination error", stderr.String())
	}
}

// assertChannelsPathMethod runs a "channels" subcommand against a
// no-content echo server and asserts the request method and path.
func assertChannelsPathMethod(t *testing.T, args []string, wantMethod, wantPath string) {
	t.Helper()
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	fullArgs := append(append([]string{}, args...), "--api-url", srv.URL)
	runCLIExpectOK(t, fullArgs)
	if *gotMethod != wantMethod || *gotPath != wantPath {
		t.Errorf("request = %s %s, want %s %s", *gotMethod, *gotPath, wantMethod, wantPath)
	}
}

func TestRun_ChannelsDelete(t *testing.T) {
	assertChannelsPathMethod(t, []string{"channels", "delete", "chn_1"},
		http.MethodDelete, "/api/v1/notification-channels/chn_1")
}

func TestRun_ChannelsTest(t *testing.T) {
	assertChannelsPathMethod(t, []string{"channels", "test", "chn_1"},
		http.MethodPost, "/api/v1/notification-channels/chn_1/test")
}

func TestRun_Channels_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"channels", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stdout.String(), "channels list") || !strings.Contains(stdout.String(), "pushover") {
		t.Errorf("stdout = %q, want usage text mentioning pushover", stdout.String())
	}
}
