package metrics

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const fakeExporter = `# HELP http_requests_total Requests.
# TYPE http_requests_total counter
http_requests_total{method="get",code="200"} 1027 1395066363000
http_requests_total{method="post",code="500"} 3
process_open_fds 12
bad-name{a="b"} 1
too_many{a="1",b="2",c="3"} 1
not_a_number NaN
<script>alert(1)</script> 1
escaped{path="a\"b\\c"} 4.5
`

func testLimits(series int) Limits {
	return Limits{Timeout: time.Second, MaxBodyBytes: 1 << 16, MaxSeries: series, MaxLabels: 2, MaxLabelBytes: 4096}
}

func TestAppScrapeParseText(t *testing.T) {
	got, stats := ParseText(strings.NewReader(fakeExporter), testLimits(100))
	if len(got) != 4 {
		t.Fatalf("kept %d series, want 4: %+v", len(got), got)
	}
	if stats.Dropped != 4 || stats.Truncated {
		t.Fatalf("stats %+v, want 4 dropped and no truncation", stats)
	}
	if got[0].Name != "http_requests_total" || got[0].Labels != `{code="200",method="get"}` || got[0].Value != 1027 {
		t.Fatalf("first sample %+v", got[0])
	}
	if got[3].Labels != `{path="a\"b\\c"}` {
		t.Fatalf("escaped labels %q", got[3].Labels)
	}
}

func TestAppScrapeSeriesCap(t *testing.T) {
	got, stats := ParseText(strings.NewReader(fakeExporter), testLimits(2))
	if len(got) != 2 || !stats.Truncated {
		t.Fatalf("got %d series truncated=%v, want 2 and truncated", len(got), stats.Truncated)
	}
}

func TestAppScrapeTargetValidation(t *testing.T) {
	if _, err := TargetURL("8.8.8.8", 9100, "/metrics"); !errors.Is(err, ErrBadTarget) {
		t.Fatalf("public address accepted: %v", err)
	}
	for _, p := range []string{"", "metrics", "//evil.example/x", "/a/../b", "/m?x=1", "/m#f", "/@host"} {
		if ValidatePath(p) == nil {
			t.Fatalf("path %q accepted", p)
		}
	}
	u, err := TargetURL("172.18.0.5", 9100, "/metrics")
	if err != nil || u != "http://172.18.0.5:9100/metrics" {
		t.Fatalf("url %q err %v", u, err)
	}
}

func TestAppScrapeRefusesRedirectAndLargeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://example.invalid/", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(strings.Repeat("a 1\n", 20000)))
	}))
	defer srv.Close()
	client := NewClient(time.Second)
	if _, _, err := Scrape(context.Background(), client, srv.URL+"/redirect", testLimits(10)); !errors.Is(err, ErrRedirect) {
		t.Fatalf("redirect followed: %v", err)
	}
	if _, _, err := Scrape(context.Background(), client, srv.URL+"/big", testLimits(10)); !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("large body accepted: %v", err)
	}
}

func TestAppScrapeLabelLimits(t *testing.T) {
	text := "a{" + strings.Repeat("k", 65) + `="v"} 1` + "\nb{k=\"\xff\"} 1\nc{k=\"" + strings.Repeat("v", 40) + "\"} 1\nd{k=\"" + strings.Repeat("w", 40) + "\"} 1\n"
	lim := testLimits(100)
	lim.MaxLabelBytes = 60
	got, stats := ParseText(strings.NewReader(text), lim)
	if len(got) != 1 || got[0].Name != "c" || !stats.Truncated || stats.Dropped != 3 {
		t.Fatalf("got %+v stats %+v, want only c kept", got, stats)
	}
}

func TestAppScrapeErrorMessageHidesAddress(t *testing.T) {
	client := NewClient(time.Second)
	_, _, err := Scrape(context.Background(), client, "http://127.0.0.1:1/metrics", testLimits(10))
	if msg := ErrorMessage(err); msg != "connection refused" || strings.Contains(msg, "127.0.0.1") {
		t.Fatalf("message %q", msg)
	}
	if ErrorMessage(StatusError{Code: 503}) != "endpoint answered status 503" {
		t.Fatal("status message")
	}
}
