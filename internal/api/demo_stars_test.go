package api

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestStars(url string) *demoStars {
	return &demoStars{url: url, client: &http.Client{Timeout: time.Second}, now: time.Now}
}

func waitStars(t *testing.T, s *demoStars, want int, wantOK bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		n, ok := s.Get()
		if ok == wantOK && n == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	n, ok := s.Get()
	t.Fatalf("stars = %d, %v; want %d, %v", n, ok, want, wantOK)
}

func TestDemoStarsFetchSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"stargazers_count":1234}`))
	}))
	defer srv.Close()
	s := newTestStars(srv.URL)
	if _, ok := s.Get(); ok {
		t.Fatal("first Get must not block on the fetch")
	}
	waitStars(t, s, 1234, true)
}

func TestDemoStarsFailureKeepsLastValue(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			http.Error(w, "rate limited", http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(`{"stargazers_count":42}`))
	}))
	defer srv.Close()
	s := newTestStars(srv.URL)
	s.Get()
	waitStars(t, s, 42, true)

	fail.Store(true)
	s.mu.Lock()
	s.nextAt = time.Time{}
	s.mu.Unlock()
	s.Get()
	time.Sleep(100 * time.Millisecond)
	waitStars(t, s, 42, true)
}

func TestDemoStarsFailureOmitsWhenNeverFetched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	s := newTestStars(srv.URL)
	s.Get()
	time.Sleep(100 * time.Millisecond)
	if _, ok := s.Get(); ok {
		t.Fatal("expected no count after a failed fetch")
	}
}

func TestDemoStarsDisabled(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	if demoStarsFromEnv(func(k string) string {
		if k == "APP_DEMO_STARS_DISABLE" {
			return "true"
		}
		return ""
	}, none) != nil {
		t.Fatal("APP_DEMO_STARS_DISABLE must disable the fetch")
	}
	empty := func(k string) (string, bool) { return "", k == "APP_DEMO_STARS_REPO" }
	if demoStarsFromEnv(func(string) string { return "" }, empty) != nil {
		t.Fatal("an empty APP_DEMO_STARS_REPO must disable the fetch")
	}
	if s := demoStarsFromEnv(func(string) string { return "" }, none); s == nil || s.url != "https://api.github.com/repos/glincker/levelrail" {
		t.Fatalf("default repo wrong: %+v", s)
	}
	bad := func(k string) (string, bool) { return "a b/c", k == "APP_DEMO_STARS_REPO" }
	if demoStarsFromEnv(func(string) string { return "" }, bad) != nil {
		t.Fatal("a malformed repo must disable the fetch")
	}
	var nilStars *demoStars
	if _, ok := nilStars.Get(); ok {
		t.Fatal("nil stars must report no count")
	}
}
