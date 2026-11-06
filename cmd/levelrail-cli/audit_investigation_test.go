package main

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func fakeAuditCSVServer(total int) *httptest.Server {
	return fakeAuditCSVServerWith(total, true, false)
}

// fakeAuditCSVServerWith can mimic a server that never sends X-Audit-Truncated, or one whose rows all share a timestamp.
func fakeAuditCSVServerWith(total int, sendTruncated, oneTimestamp bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		before, _ := time.Parse(time.RFC3339Nano, r.URL.Query().Get("before"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit == 0 || limit > 200 {
			limit = 200
		}
		base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		var rows [][]string
		more := false
		for i := total; i >= 1; i-- {
			// Pairs of rows share a timestamp, so one straddles every even page boundary.
			at := base.Add(time.Duration(i/2) * time.Second)
			if oneTimestamp {
				at = base
			}
			if !before.IsZero() && !at.Before(before) {
				continue
			}
			if len(rows) == limit {
				more = true
				break
			}
			rows = append(rows, []string{fmt.Sprint("aud_", i), "", "", "", "", "", "", "", "", at.Format("2006-01-02T15:04:05.000000000Z"), ""})
		}
		w.Header().Set("Content-Type", "text/csv")
		if more && sendTruncated {
			w.Header().Set("X-Audit-Truncated", "true")
		}
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"id", "a", "b", "c", "d", "e", "f", "g", "h", "created_at", "k"})
		_ = cw.WriteAll(rows)
	}))
}

func TestRun_AuditLogCSVPagesWithoutLosingTiedRows(t *testing.T) {
	const total = 450
	srv := fakeAuditCSVServer(total)
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
	seen := map[string]bool{}
	for _, r := range recs[1:] {
		if seen[r[0]] {
			t.Fatalf("duplicate row %s", r[0])
		}
		seen[r[0]] = true
	}
	if len(seen) != total {
		t.Errorf("rows = %d, want %d", len(seen), total)
	}
}

func TestRun_AuditLogCSVPagesAgainstServerWithoutTruncatedHeader(t *testing.T) {
	const total = 450
	srv := fakeAuditCSVServerWith(total, false, false)
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
	if len(recs)-1 != total {
		t.Errorf("rows = %d, want %d (an old server sends no truncation header)", len(recs)-1, total)
	}
}

func TestRun_AuditLogCSVFailsWhenTimestampTieCannotAdvance(t *testing.T) {
	srv := fakeAuditCSVServerWith(450, false, true)
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"audit-log", "--api-url", srv.URL, "--export", "csv"}, &stdout, &stderr, envMap())
	if got == exitOK {
		t.Fatalf("exit = 0, want non-zero when the export is incomplete (stderr=%q)", stderr.String())
	}
	if !strings.Contains(stderr.String(), "export incomplete") {
		t.Errorf("stderr = %q", stderr.String())
	}
}

func TestRun_AuditLogCSVLimitWarnsWhenCut(t *testing.T) {
	srv := fakeAuditCSVServer(450)
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"audit-log", "--api-url", srv.URL, "--export", "csv", "--limit", "300"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, stderr=%q", got, stderr.String())
	}
	if !strings.Contains(stderr.String(), "truncated") {
		t.Errorf("stderr = %q, want a truncation warning", stderr.String())
	}
	recs, _ := csv.NewReader(&stdout).ReadAll()
	if len(recs) != 201 {
		t.Errorf("rows = %d, want the server's 200 plus header", len(recs))
	}
}
