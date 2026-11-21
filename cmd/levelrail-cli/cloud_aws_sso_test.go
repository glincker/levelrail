package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCloudAWSSSOLoginLoopWaitsAndLinks(t *testing.T) {
	var polls atomic.Int32
	var waits []time.Duration
	old := ssoSleep
	ssoSleep = func(d time.Duration) { waits = append(waits, d) }
	t.Cleanup(func() { ssoSleep = old })
	var selected atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/cloud-accounts/aws/sso/start":
			_, _ = w.Write([]byte(`{"session_id":"s1","user_code":"ABCD-EFGH","verification_uri_complete":"https://v?c=1","interval":2}`))
		case "/api/v1/cloud-accounts/aws/sso/poll":
			switch polls.Add(1) {
			case 1:
				_, _ = w.Write([]byte(`{"status":"pending","interval":2}`))
			case 2:
				_, _ = w.Write([]byte(`{"status":"slow_down","interval":7}`))
			default:
				_, _ = w.Write([]byte(`{"status":"done","accounts":[{"account_id":"111122223333","account_name":"prod","roles":["Admin"]}]}`))
			}
		case "/api/v1/cloud-accounts/aws/sso/select":
			raw, _ := io.ReadAll(r.Body)
			selected.Store(string(raw))
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"aws-1"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	out, errOut := runCLIExpectOK(t, []string{"cloud", "aws", "sso", "login", "--start-url", "https://d-1.awsapps.com/start", "--sso-region", "eu-west-1", "--api-url", srv.URL})
	if !strings.Contains(errOut, "ABCD-EFGH") || !strings.Contains(out, "Linked aws-1") {
		t.Fatalf("out=%q err=%q", out, errOut)
	}
	want := []time.Duration{2 * time.Second, 2 * time.Second, 7 * time.Second}
	if len(waits) != 3 || waits[0] != want[0] || waits[1] != want[1] || waits[2] != want[2] {
		t.Fatalf("waits = %v, want %v (slow_down must widen the interval)", waits, want)
	}
	if s, _ := selected.Load().(string); !strings.Contains(s, "111122223333") || !strings.Contains(s, "Admin") {
		t.Fatalf("single account and role should be picked, got %q", s)
	}
}

func TestCloudAWSSSOLoginDeniedStops(t *testing.T) {
	old := ssoSleep
	ssoSleep = func(time.Duration) {}
	t.Cleanup(func() { ssoSleep = old })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/start") {
			_, _ = w.Write([]byte(`{"session_id":"s1","user_code":"X","verification_uri_complete":"https://v","interval":1}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"denied"}`))
	}))
	defer srv.Close()
	var outBuf, errBuf bytes.Buffer
	code := run("levelrail-cli-test", []string{"cloud", "aws", "sso", "login", "--start-url", "https://d-1.awsapps.com/start", "--sso-region", "eu-west-1", "--api-url", srv.URL}, &outBuf, &errBuf, envMap())
	if code == exitOK || !strings.Contains(errBuf.String(), "denied") {
		t.Fatalf("code=%d stderr=%q", code, errBuf.String())
	}
}
