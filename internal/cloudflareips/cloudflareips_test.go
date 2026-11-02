package cloudflareips

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

const goodBody = `{"success":true,"result":{"ipv4_cidrs":["198.51.100.0/24","203.0.113.0/24"],"ipv6_cidrs":["2001:db8::/32"]}}`

func lim() Limits { return Limits{Timeout: time.Second, MaxBytes: 4096, MinRanges: 3} }

func TestEmbeddedContains(t *testing.T) {
	l := New()
	if !l.Contains(netip.MustParseAddr("173.245.48.9")) || l.Contains(netip.MustParseAddr("8.8.8.8")) {
		t.Fatal("embedded snapshot membership wrong")
	}
}

func TestRefreshFailureKeepsLastGoodList(t *testing.T) {
	body, code := goodBody, http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	l := New()
	l.endpoint = srv.URL
	if changed, err := l.Refresh(context.Background(), lim(), time.Now()); err != nil || !changed {
		t.Fatalf("first refresh: %v %v", changed, err)
	}
	if !l.Contains(netip.MustParseAddr("198.51.100.7")) {
		t.Fatal("fetched list not active")
	}
	for _, bad := range []struct {
		b string
		c int
	}{{`{"success":true,"result":{"ipv4_cidrs":["198.51.100.0/24"]}}`, 200}, {"not json", 200}, {goodBody, 500}} {
		body, code = bad.b, bad.c
		if _, err := l.Refresh(context.Background(), lim(), time.Now()); err == nil {
			t.Fatalf("expected failure for %q", bad.b)
		}
	}
	st := l.Status()
	if st.Source != SourceFetched || st.Ranges != 3 || st.LastError == "" || !l.Contains(netip.MustParseAddr("203.0.113.1")) {
		t.Fatalf("last good list lost: %+v", st)
	}
}
