package selfupgrade

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const atomBody = `<?xml version="1.0"?><feed xmlns="http://www.w3.org/2005/Atom">
<entry><updated>2026-11-18T10:00:00Z</updated><link rel="alternate" href="https://github.com/o/r/releases/tag/v1.5.8"/></entry>
<entry><updated>2026-11-19T10:00:00Z</updated><link rel="alternate" href="https://github.com/o/r/releases/tag/kit/v9.0.0"/></entry>
<entry><updated>2026-11-19T11:00:00Z</updated><link rel="alternate" href="https://github.com/o/r/releases/tag/v1.6.0-beta.1"/></entry>
<entry><updated>2026-11-10T10:00:00Z</updated><link rel="alternate" href="https://github.com/o/r/releases/tag/v1.5.7"/></entry>
</feed>`

func webServer(t *testing.T, atom func(http.ResponseWriter), latest func(http.ResponseWriter, *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/o/r/releases.atom":
			atom(w)
		case "/o/r/releases/latest":
			latest(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDiscovererLearnsTheNewestStableTag(t *testing.T) {
	status := func(code int) func(http.ResponseWriter) { return func(w http.ResponseWriter) { w.WriteHeader(code) } }
	redirectTo := func(loc string) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, loc, http.StatusFound) }
	}
	tests := []struct {
		name       string
		atom       func(http.ResponseWriter)
		latest     func(http.ResponseWriter, *http.Request)
		maxBytes   int64
		wantTag    string
		wantSource string
		wantErr    bool
	}{
		{"feed names the highest stable product tag", func(w http.ResponseWriter) { _, _ = w.Write([]byte(atomBody)) }, nil, 0, "v1.5.8", "atom", false},
		{"feed down, redirect answers", status(502), redirectTo("/o/r/releases/tag/v1.5.9"), 0, "v1.5.9", "redirect", false},
		{"oversize feed is refused, redirect used", func(w http.ResponseWriter) { _, _ = w.Write([]byte(atomBody)) }, redirectTo("/o/r/releases/tag/v1.5.9"), 64, "v1.5.9", "redirect", false},
		{"oversize feed and no redirect fails", func(w http.ResponseWriter) { _, _ = w.Write([]byte(atomBody)) }, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(404) }, 64, "", "", true},
		{"nested module tag from the redirect is refused", status(502), redirectTo("/o/r/releases/tag/kit/v0.2.0"), 0, "", "", true},
		{"redirect to another host is refused", status(502), redirectTo("https://evil.example/o/r/releases/tag/v9.9.9"), 0, "", "", true},
		{"garbage feed and no redirect fails", func(w http.ResponseWriter) { _, _ = w.Write([]byte("<html>")) }, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) }, 0, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			latest := tt.latest
			if latest == nil {
				latest = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }
			}
			srv := webServer(t, tt.atom, latest)
			got, err := Discoverer{Repo: "o/r", WebBase: srv.URL, AllowHTTP: true, MaxBytes: tt.maxBytes}.Latest(t.Context())
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got.Tag != tt.wantTag || got.Source != tt.wantSource {
				t.Fatalf("got %+v, want tag %q via %q", got, tt.wantTag, tt.wantSource)
			}
			if tt.wantSource == "atom" && got.PublishedAt != "2026-11-18T10:00:00Z" {
				t.Fatalf("published = %q", got.PublishedAt)
			}
		})
	}
}

func TestDiscovererRefusesPlainHTTPAndBadRepo(t *testing.T) {
	srv := webServer(t, func(w http.ResponseWriter) { _, _ = w.Write([]byte(atomBody)) }, nil)
	tests := []struct {
		name string
		d    Discoverer
	}{
		{"http base without opt-in", Discoverer{Repo: "o/r", WebBase: srv.URL}},
		{"repo with a query", Discoverer{Repo: "o/r?x=1", WebBase: "https://github.com"}},
		{"repo with extra path", Discoverer{Repo: "o/r/x", WebBase: "https://github.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.d.Latest(t.Context()); err == nil {
				t.Fatal("expected a refusal")
			}
		})
	}
}

// A tag found through the release pages still has to pass the same download
// checks: a release without a checksums file is refused by the fetcher.
func TestDiscoveredTagIsStillVerifiedByTheFetcher(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/"+checksumsAsset) {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("data"))
	}))
	t.Cleanup(srv.Close)
	_, err := Fetcher{BaseURL: srv.URL, AllowHTTP: true}.Fetch(t.Context(), "v1.5.8", "levelrail-linux-amd64", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "checksums") {
		t.Fatalf("fetch of a release with no checksums = %v, want a checksums error", err)
	}
}
