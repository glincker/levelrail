package ingress

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAccessLogRingKeepsNewestAndFilters(t *testing.T) {
	l := NewAccessLog(3)
	base := time.Now()
	for i := 0; i < 5; i++ {
		l.Record("a.test", AccessRecord{Time: base.Add(time.Duration(i) * time.Second), Path: "/p", Status: 200 + i, Millis: float64(i * 100)})
	}
	got := l.Query([]string{"a.test"}, AccessFilter{})
	if len(got) != 3 || got[0].Status != 204 || got[2].Status != 202 {
		t.Fatalf("want newest three descending, got %+v", got)
	}
	slow := l.Query([]string{"a.test"}, AccessFilter{MinMillis: 350})
	if len(slow) != 1 || slow[0].Status != 204 {
		t.Fatalf("min_ms filter: %+v", slow)
	}
}

func TestAccessLogDisabled(t *testing.T) {
	l := NewAccessLog(0)
	l.Record("a", AccessRecord{})
	if got := l.Query([]string{"a"}, AccessFilter{}); len(got) != 0 {
		t.Fatalf("disabled log returned %d", len(got))
	}
}

func TestEnsureCorrelationHeaders(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	id, tid := ensureCorrelationHeaders(r)
	if len(id) != 16 || len(tid) != 32 {
		t.Fatalf("generated ids: %q %q", id, tid)
	}
	if !strings.HasPrefix(r.Header.Get("Traceparent"), "00-"+tid+"-") {
		t.Fatalf("traceparent not set from trace id: %q", r.Header.Get("Traceparent"))
	}

	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Request-Id", "client-1")
	r.Header.Set("Traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	id, tid = ensureCorrelationHeaders(r)
	if id != "client-1" || tid != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("client ids not preserved: %q %q", id, tid)
	}

	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Traceparent", "garbage")
	if _, tid = ensureCorrelationHeaders(r); len(tid) != 32 {
		t.Fatalf("invalid traceparent should be replaced, got %q", tid)
	}
}
