package dockerguard

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestUpstreamLogRateLimitsAndRecovers(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	now := time.Unix(1000, 0)
	u := &upstreamLog{now: func() time.Time { return now }}
	r := httptest.NewRequest("GET", "/_ping", nil)
	for range 5 {
		u.fail(logger, r, errors.New("dial"))
		now = now.Add(time.Second)
	}
	if n := strings.Count(buf.String(), "upstream request failed"); n != 1 {
		t.Fatalf("warnings in window = %d", n)
	}
	now = now.Add(upstreamWarnInterval)
	u.fail(logger, r, errors.New("dial"))
	if n := strings.Count(buf.String(), "upstream request failed"); n != 2 {
		t.Fatalf("warnings after interval = %d", n)
	}
	u.ok(logger)
	if !strings.Contains(buf.String(), "reachable again") {
		t.Fatal("no recovery line")
	}
}
