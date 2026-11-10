package ingress

import (
	"net/http/httptest"
	"strconv"
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

func TestAccessLogSubscribe(t *testing.T) {
	for _, capacity := range []int{3, 0} {
		l := NewAccessLog(capacity)
		var got []string
		unsub := l.Subscribe(func(key string, rec AccessRecord) { got = append(got, key+rec.Path) })
		var second int
		unsub2 := l.Subscribe(func(string, AccessRecord) { second++ })

		l.Record("a", AccessRecord{Path: "/1"})
		unsub()
		unsub()
		l.Record("a", AccessRecord{Path: "/2"})
		unsub2()
		l.Record("a", AccessRecord{Path: "/3"})

		if len(got) != 1 || got[0] != "a/1" {
			t.Fatalf("cap %d: first subscriber saw %v", capacity, got)
		}
		if second != 2 {
			t.Fatalf("cap %d: second subscriber saw %d, want 2", capacity, second)
		}
	}
}

func TestAccessLogSubscribeNilSafe(*testing.T) {
	var l *AccessLog
	l.Subscribe(func(string, AccessRecord) {})()
	NewAccessLog(1).Subscribe(nil)()
}

func BenchmarkAccessLogRecordNoSubscribers(b *testing.B) {
	l := NewAccessLog(10)
	for i := 0; i < b.N; i++ {
		l.Record("a", AccessRecord{})
	}
}

func TestAccessLogRetentionSourceApplied(t *testing.T) {
	l := NewAccessLog(10)
	l.Record("k", AccessRecord{Time: time.Now().Add(-2 * time.Hour), Status: 200})
	window := 3 * time.Hour
	l.SetRetentionSource(func() time.Duration { return window })
	if got := len(l.Query([]string{"k"}, AccessFilter{})); got != 1 {
		t.Fatalf("record within the window hidden: %d", got)
	}
	window = time.Hour
	if got := len(l.Query([]string{"k"}, AccessFilter{})); got != 0 {
		t.Fatalf("a shorter stored window did not apply to the next read: %d", got)
	}
}

func TestAccessLogCapacityResizesLive(t *testing.T) {
	base := time.Now()
	l := NewAccessLog(5)
	size := 5
	l.SetCapacitySource(func() int { return size })
	for i := range 5 {
		l.Record("a", AccessRecord{Path: strconv.Itoa(i), Time: base.Add(time.Duration(i) * time.Second)})
	}
	size = 2
	l.capChecked.Store(0)
	l.Record("a", AccessRecord{Path: "5", Time: base.Add(5 * time.Second)})
	got := l.Query([]string{"a"}, AccessFilter{})
	if len(got) != 2 || got[0].Path != "5" || got[1].Path != "4" {
		t.Fatalf("after shrink got %+v", got)
	}
	size = 0
	l.capChecked.Store(0)
	l.Record("a", AccessRecord{Path: "6", Time: base.Add(6 * time.Second)})
	if len(l.rings) != 0 || len(l.Query([]string{"a"}, AccessFilter{})) != 0 {
		t.Fatal("size 0 should free the rings")
	}
	size = 3
	l.capChecked.Store(0)
	l.Record("a", AccessRecord{Path: "7", Time: base.Add(7 * time.Second)})
	if got := l.Query([]string{"a"}, AccessFilter{}); len(got) != 1 {
		t.Fatalf("after regrow got %+v", got)
	}
}
