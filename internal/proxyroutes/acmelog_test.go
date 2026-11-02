package proxyroutes

import (
	"testing"
	"time"
)

func TestACMEFailureSinceIgnoresOldLines(t *testing.T) {
	logs := `time="2026-11-02T10:00:00Z" level=error msg="Unable to obtain ACME certificate for domains" domains=[app.example.com]
time="2026-11-02T10:05:00Z" level=info msg="Server configuration reloaded" domain=app.example.com`
	since := time.Date(2026, 11, 2, 10, 2, 0, 0, time.UTC)
	if got := ACMEFailureSince(logs, "app.example.com", since); got != "" {
		t.Fatalf("old failure shown after retry: %q", got)
	}
	if got := ACMEFailureSince(logs, "app.example.com", time.Time{}); got == "" {
		t.Fatal("failure should show without a cutoff")
	}
	newer := logs + "\n" + `{"level":"error","msg":"acme: error: 429 rateLimited","time":"2026-11-02T10:06:00Z","domain":"app.example.com"}`
	if got := ACMEFailureSince(newer, "app.example.com", since); got == "" {
		t.Fatal("newer failure must be reported")
	}
}

func TestACMEFailureSinceKeepsUnstampedLines(t *testing.T) {
	logs := "acme: error for app.example.com"
	if got := ACMEFailureSince(logs, "app.example.com", time.Now()); got == "" {
		t.Fatal("unstamped line has unknown age and should be kept")
	}
}
