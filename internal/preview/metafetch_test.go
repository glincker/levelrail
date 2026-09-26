package preview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/netguard"
)

func fetchTestConfig() Config {
	cfg := ConfigFromEnv(func(string) (string, bool) { return "", false }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	cfg.MetaTimeout = 2 * time.Second
	return cfg
}

func newAppServer(t *testing.T) (*httptest.Server, MetaTarget) {
	t.Helper()
	png := pngBytes(t, paintedImage(1200, 630, 255))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Cookie") != "" || r.Host != "web:3000" {
			http.Error(w, "unexpected request", http.StatusTeapot)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = fmt.Fprint(w, `<html><head><title>Home</title><meta property="og:image" content="/og.png"></head></html>`)
	})
	mux.HandleFunc("/og.png", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	mux.HandleFunc("/huge.png", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 3<<20))
	})
	mux.HandleFunc("/private", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusUnauthorized) })
	mux.HandleFunc("/forbidden", func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusForbidden) })
	mux.HandleFunc("/hop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/", http.StatusFound) })
	mux.HandleFunc("/away", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://elsewhere.example/", http.StatusFound)
	})
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	mux.HandleFunc("/json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{}`)
	})
	mux.HandleFunc("/long", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, "<head>"+strings.Repeat(" ", 600<<10)+`<meta property="og:image" content="/late.png"></head>`)
	})
	mux.HandleFunc("/slow", func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, MetaTarget{DeploymentID: "d1", Dial: srv.Listener.Addr().String(), Host: "web", Port: 3000, Domains: []string{"app.example.com"}}
}

func TestHTTPFetcher_FetchPage(t *testing.T) {
	_, target := newAppServer(t)
	f := newHTTPFetcher(fetchTestConfig())
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantErr    error
		wantTitle  string
		wantRef    string
	}{
		{name: "parses the page", path: "/", wantStatus: 200, wantTitle: "Home", wantRef: "/og.png"},
		{name: "same origin redirect is followed", path: "/hop", wantStatus: 200, wantTitle: "Home", wantRef: "/og.png"},
		{name: "401 is reported not fetched", path: "/private", wantStatus: 401},
		{name: "403 is reported not fetched", path: "/forbidden", wantStatus: 403},
		{name: "404", path: "/missing", wantStatus: 404},
		{name: "offsite redirect refused", path: "/away", wantErr: errOffsiteRedirect},
		{name: "redirect loop stops", path: "/loop", wantErr: errors.New("redirects")},
		{name: "non html", path: "/json", wantErr: ErrNotHTML},
		{name: "html read is capped", path: "/long", wantStatus: 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := f.FetchPage(context.Background(), target, tc.path)
			if tc.wantErr != nil {
				if err == nil || (!errors.Is(err, tc.wantErr) && !strings.Contains(err.Error(), tc.wantErr.Error())) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if res.HTTPStatus != tc.wantStatus || res.Meta.Title != tc.wantTitle || res.Meta.OGImage != tc.wantRef {
				t.Errorf("result = %d %+v, want status %d title %q ref %q", res.HTTPStatus, res.Meta, tc.wantStatus, tc.wantTitle, tc.wantRef)
			}
		})
	}
}

func TestHTTPFetcher_FetchPage_Timeout(t *testing.T) {
	_, target := newAppServer(t)
	cfg := fetchTestConfig()
	cfg.MetaTimeout = 100 * time.Millisecond
	start := time.Now()
	if _, err := newHTTPFetcher(cfg).FetchPage(context.Background(), target, "/slow"); err == nil {
		t.Fatal("a stalled page must time out")
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("timeout took %v", time.Since(start))
	}
}

func TestHTTPFetcher_FetchImage(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "false")
	srv, target := newAppServer(t)
	f := newHTTPFetcher(fetchTestConfig())
	base, _ := url.Parse("http://web:3000/")
	loopback := fmt.Sprintf("http://localhost:%s/og.png", strings.SplitN(srv.Listener.Addr().String(), ":", 2)[1])
	tests := []struct {
		name    string
		ref     string
		wantErr string
	}{
		{name: "relative on the app", ref: "/og.png"},
		{name: "public domain is read from the app itself", ref: "https://app.example.com/og.png"},
		{name: "over the size cap", ref: "/huge.png", wantErr: "size cap"},
		{name: "missing", ref: "/nope.png", wantErr: "404"},
		{name: "file scheme", ref: "file:///etc/passwd", wantErr: "unusable"},
		{name: "javascript scheme", ref: "javascript:alert(1)", wantErr: "unusable"},
		{name: "metadata service literal", ref: "http://169.254.169.254/latest", wantErr: "unusable"},
		{name: "private literal", ref: "http://10.1.2.3/x.png", wantErr: "unusable"},
		{name: "name resolving to loopback is refused at dial", ref: loopback, wantErr: "internal"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := f.FetchImage(context.Background(), target, base, tc.ref)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || len(data) == 0 {
				t.Fatalf("data %d bytes err %v", len(data), err)
			}
		})
	}
}
