package telemetry

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestReferrerHost(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"not a url", ""},
		{"https://News.Example.com/a/b?token=secret#x", "news.example.com"},
		{"https://user:pw@host.example:8443/p", "host.example"},
		{"http://[::1]:8080/p", "[::1]"},
	}
	for _, tc := range tests {
		if got := ReferrerHost(tc.in); got != tc.want {
			t.Errorf("ReferrerHost(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestObserveAudienceCapsAndDirect(t *testing.T) {
	var w RequestWindow
	for i := 0; i < 6; i++ {
		w.ObserveAudience(fmt.Sprintf("h%d.example", i), "de", 3)
	}
	w.ObserveAudience("", "", 3)
	if w.Referrers[OverflowAudience] != 4 || len(w.Referrers) != 4 {
		t.Errorf("referrers = %v", w.Referrers)
	}
	if w.Countries["DE"] != 6 {
		t.Errorf("countries = %v", w.Countries)
	}
}

func TestAudienceRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)
	var w RequestWindow
	w.Requests = 3
	w.ObserveAudience("a.example", "US", 5)
	w.ObserveAudience("a.example", "US", 5)
	w.ObserveAudience("", "DE", 5)
	for i := 0; i < 2; i++ {
		if err := db.RecordRequests(ctx, map[string]RequestWindow{"web": w}, now); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.QueryAudience(ctx, "service:web", now.Add(-time.Hour), now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	for _, r := range rows {
		got[r.Kind+":"+r.Label] = r.Requests
	}
	if got["referrer:a.example"] != 4 || got["referrer:direct"] != 2 || got["country:US"] != 4 || got["country:DE"] != 2 {
		t.Errorf("rows = %v", got)
	}
}
