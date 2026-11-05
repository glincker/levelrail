package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseAuditRange(t *testing.T) {
	cases := []struct {
		in       string
		from, to int64
		bad      bool
	}{
		{"", 0, 0, false},
		{"100-200", 100, 200, false},
		{"100-", 100, 0, false},
		{"-200", 0, 200, false},
		{"abc", 0, 0, true},
		{"5-x", 0, 0, true},
		{"9-3", 0, 0, true},
	}
	for _, c := range cases {
		from, to, err := parseAuditRange(c.in)
		if (err != nil) != c.bad || from != c.from || to != c.to {
			t.Errorf("parseAuditRange(%q) = %d, %d, %v", c.in, from, to, err)
		}
	}
}

func TestResolveAuditTime(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	cases := map[string]string{
		"":                     "",
		"24h":                  "2026-01-09T12:00:00Z",
		"7d":                   "2026-01-03T12:00:00Z",
		"2026-02-01T00:00:00Z": "2026-02-01T00:00:00Z",
	}
	for in, want := range cases {
		got, err := resolveAuditTime(in, now)
		if err != nil || got != want {
			t.Errorf("resolveAuditTime(%q) = %q, %v, want %q", in, got, err, want)
		}
	}
	if _, err := resolveAuditTime("soon", now); err == nil {
		t.Error("want an error for an unparseable time")
	}
}

func TestRun_AuditVerifyRangeWarnsAboutBreaksElsewhere(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("from") != "100" {
			t.Errorf("query = %q, want from=100", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"chain_ok":false,"checked":50,"head":150,"head_hash":"abc","range_end":150,"range_end_hash":"abc","problems":[],"problems_outside_range":1}`))
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"audit-verify", "--api-url", srv.URL, "--range", "100-"}, &stdout, &stderr, envMap())
	if got != exitCheckFailed {
		t.Fatalf("exit = %d, want %d", got, exitCheckFailed)
	}
	out := stdout.String()
	if !strings.Contains(out, "range 100- intact") || strings.Contains(out, "audit log intact") || !strings.Contains(out, "warning") {
		t.Errorf("stdout = %q", out)
	}
}

func TestRun_AuditLogCSVPagesUntilExhausted(t *testing.T) {
	const total = 250
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		before := r.URL.Query().Get("before")
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"id", "a", "b", "c", "d", "e", "f", "g", "h", "created_at", "k"})
		for i := total; i >= 1; i-- {
			ts := fmt.Sprintf("2026-01-01T00:%02d:%02d.000000000Z", i/60, i%60)
			if before != "" && ts >= before {
				continue
			}
			_ = cw.Write([]string{fmt.Sprint("aud_", i), "", "", "", "", "", "", "", "", ts, ""})
			if i == total-199 {
				break
			}
		}
		cw.Flush()
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"audit-log", "--api-url", srv.URL, "--export", "csv"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, stderr=%q", got, stderr.String())
	}
	recs, err := csv.NewReader(&stdout).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != total+1 {
		t.Errorf("rows = %d, want %d plus header", len(recs)-1, total)
	}
}
