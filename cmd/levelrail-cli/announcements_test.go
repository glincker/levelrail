package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func announcementsServer(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()
	future := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	list := `{"items":[` +
		`{"id":"crit-1","tier":"critical","title":"Upgrade now","body":"b","applies_to":"<1.5.14","links":[],"issued_at":"2026-11-01T00:00:00Z","expires_at":"2027-01-01T00:00:00Z","read":false,"dismissed":false,"dismissible":false},` +
		`{"id":"info-1","tier":"info","title":"Small news","body":"b","links":[],"issued_at":"2026-11-02T00:00:00Z","expires_at":"2027-01-01T00:00:00Z","read":true,"dismissed":false,"dismissible":true},` +
		`{"id":"old-1","tier":"feature","title":"Dismissed one","body":"b","links":[],"issued_at":"2026-11-03T00:00:00Z","expires_at":"2027-01-01T00:00:00Z","read":true,"dismissed":true,"dismissible":true},` +
		`{"id":"snz-1","tier":"advisory","title":"Snoozed one","body":"b","links":[],"issued_at":"2026-11-04T00:00:00Z","expires_at":"2027-01-01T00:00:00Z","read":false,"dismissed":false,"snoozed_until":"` + future + `","dismissible":true}` +
		`],"status":{"mode":"all","sequence":3}}`
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, key+" "+string(body))
		w.Header().Set("Content-Type", "application/json")
		switch key {
		case "GET /api/v1/announcements":
			_, _ = w.Write([]byte(list))
		case "POST /api/v1/announcements/info-1/read", "POST /api/v1/announcements/info-1/dismiss", "POST /api/v1/announcements/info-1/snooze":
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/v1/announcements/crit-1/dismiss":
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"a critical notice cannot be dismissed until the update is installed"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAnnouncementsCLI_LsHidesDismissedAndSnoozedUnlessAll(t *testing.T) {
	var calls []string
	srv := announcementsServer(t, &calls)
	defer srv.Close()
	var out, errb bytes.Buffer
	if code := run("cli", []string{"announcements", "ls", "--api-url", srv.URL}, &out, &errb, envMap()); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	got := out.String()
	for _, want := range []string{"critical", "Upgrade now", "id crit-1", "unread", "Small news", "read"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "Dismissed one") || strings.Contains(got, "Snoozed one") {
		t.Errorf("hidden items shown without --all: %q", got)
	}
	out.Reset()
	if code := run("cli", []string{"announcements", "ls", "--all", "--api-url", srv.URL}, &out, &errb, envMap()); code != exitOK {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	if !strings.Contains(out.String(), "Dismissed one") || !strings.Contains(out.String(), "snoozed until") {
		t.Errorf("--all must show every item: %q", out.String())
	}
	out.Reset()
	if code := run("cli", []string{"announcements", "ls", "--json", "--api-url", srv.URL}, &out, &errb, envMap()); code != exitOK || !strings.Contains(out.String(), `"id": "crit-1"`) && !strings.Contains(out.String(), `"id":"crit-1"`) {
		t.Errorf("json output: %d %q", code, out.String())
	}
}

func TestAnnouncementsCLI_MarksAndSnoozeBody(t *testing.T) {
	var calls []string
	srv := announcementsServer(t, &calls)
	defer srv.Close()
	for _, args := range [][]string{
		{"announcements", "read", "info-1"},
		{"announcements", "dismiss", "info-1"},
		{"announcements", "snooze", "info-1", "--for", "4h"},
	} {
		var out, errb bytes.Buffer
		if code := run("cli", append(args, "--api-url", srv.URL), &out, &errb, envMap()); code != exitOK {
			t.Fatalf("%v exit %d: %s", args, code, errb.String())
		}
	}
	joined := strings.Join(calls, "\n")
	for _, want := range []string{"POST /api/v1/announcements/info-1/read", "POST /api/v1/announcements/info-1/dismiss", `/snooze {"duration":"4h"}`} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing call %q in %q", want, joined)
		}
	}
	var out, errb bytes.Buffer
	if code := run("cli", []string{"announcements", "snooze", "info-1", "--api-url", srv.URL}, &out, &errb, envMap()); code == exitOK {
		t.Error("snooze without --for must fail")
	}
}

func TestAnnouncementsCLI_CriticalDismissExplainsUpgrade(t *testing.T) {
	var calls []string
	srv := announcementsServer(t, &calls)
	defer srv.Close()
	var out, errb bytes.Buffer
	if code := run("cli", []string{"announcements", "dismiss", "crit-1", "--api-url", srv.URL}, &out, &errb, envMap()); code == exitOK {
		t.Fatal("critical dismiss must fail")
	}
	msg := errb.String() + out.String()
	if !strings.Contains(msg, "cannot be dismissed until you upgrade") || !strings.Contains(msg, "<1.5.14") {
		t.Errorf("unclear message: %q", msg)
	}
}
