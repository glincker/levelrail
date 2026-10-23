package trafficexport

import (
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

func TestVisitorID(t *testing.T) {
	secret := []byte("server-secret")
	day := time.Date(2026, 10, 22, 3, 0, 0, 0, time.UTC)
	base := VisitorID(secret, day, "203.0.113.7", "Mozilla/5.0")

	tests := []struct {
		name string
		got  string
		same bool
	}{
		{"same inputs", VisitorID(secret, day, "203.0.113.7", "Mozilla/5.0"), true},
		{"same day other hour", VisitorID(secret, day.Add(20*time.Hour), "203.0.113.7", "Mozilla/5.0"), true},
		{"same day other zone", VisitorID(secret, day.In(time.FixedZone("x", 5*3600)), "203.0.113.7", "Mozilla/5.0"), true},
		{"next day", VisitorID(secret, day.Add(24*time.Hour), "203.0.113.7", "Mozilla/5.0"), false},
		{"other ip", VisitorID(secret, day, "203.0.113.8", "Mozilla/5.0"), false},
		{"other ua", VisitorID(secret, day, "203.0.113.7", "curl/8"), false},
		{"other secret", VisitorID([]byte("other"), day, "203.0.113.7", "Mozilla/5.0"), false},
		{"field boundary", VisitorID(secret, day, "203.0.113.7M", "ozilla/5.0"), false},
	}
	for _, tc := range tests {
		if (tc.got == base) != tc.same {
			t.Errorf("%s: equal=%v want %v", tc.name, tc.got == base, tc.same)
		}
	}
	if len(base) != visitorIDHexLen || strings.Trim(base, "0123456789abcdef") != "" {
		t.Fatalf("not hex of expected length: %q", base)
	}
	if VisitorID(nil, day, "1.2.3.4", "ua") != "" {
		t.Fatal("empty secret must yield empty id")
	}
}

func TestNewEvent(t *testing.T) {
	at := time.Now()
	e := NewEvent("shop", ingress.AccessRecord{
		Time: at, Host: "h", Method: "POST", Path: "/p", Status: 502, Millis: 3, BytesIn: 1, BytesOut: 2,
		RequestID: "r", TraceID: "t", Upstream: true, Cold: true,
	})
	want := Event{
		Time: at, App: "shop", Host: "h", Method: "POST", Path: "/p", Status: 502, DurationMS: 3,
		BytesIn: 1, BytesOut: 2, RequestID: "r", TraceID: "t", UpstreamError: true, ColdStart: true,
	}
	if e != want {
		t.Fatalf("got %+v", e)
	}
}
