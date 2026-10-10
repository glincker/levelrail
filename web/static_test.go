package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func staticTestFS() fstest.MapFS {
	return fstest.MapFS{
		"dist/index.html":                &fstest.MapFile{Data: []byte("<html><head></head></html>")},
		"dist/assets/app-AbCd1234.js":    &fstest.MapFile{Data: []byte("plain")},
		"dist/assets/app-AbCd1234.js.br": &fstest.MapFile{Data: []byte("brotli")},
		"dist/assets/app-AbCd1234.js.gz": &fstest.MapFile{Data: []byte("gzip")},
		"dist/assets/logo-AbCd1234.png":  &fstest.MapFile{Data: []byte("png")},
		"dist/theme-init.js":             &fstest.MapFile{Data: []byte("theme")},
	}
}

func TestStaticFiles_HeadersAndEncoding(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		acceptEnc    string
		wantStatus   int
		wantBody     string
		wantEncoding string
		wantCache    string
		wantVary     bool
	}{
		{"hashed js br", "/assets/app-AbCd1234.js", "gzip, br", 200, "brotli", "br", cacheImmutable, true},
		{"hashed js gzip only", "/assets/app-AbCd1234.js", "gzip", 200, "gzip", "gzip", cacheImmutable, true},
		{"hashed js br refused by q=0", "/assets/app-AbCd1234.js", "br;q=0, gzip", 200, "gzip", "gzip", cacheImmutable, true},
		{"hashed js identity", "/assets/app-AbCd1234.js", "", 200, "plain", "", cacheImmutable, true},
		{"png never compressed", "/assets/logo-AbCd1234.png", "br, gzip", 200, "png", "", cacheImmutable, false},
		{"unhashed revalidates", "/theme-init.js", "gzip", 200, "theme", "", cacheRevalidate, true},
		{"variant not routable", "/assets/app-AbCd1234.js.br", "br", 404, "", "", "no-store", false},
		{"missing asset is 404 not html", "/assets/gone-ZzZz9999.js", "", 404, "", "", "no-store", false},
	}
	h := handlerFromFS(staticTestFS(), nil)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.acceptEnc != "" {
				req.Header.Set("Accept-Encoding", tc.acceptEnc)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if got := rec.Header().Get("Cache-Control"); got != tc.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tc.wantCache)
			}
			if tc.wantStatus != 200 {
				return
			}
			if got := rec.Body.String(); got != tc.wantBody {
				t.Errorf("body = %q, want %q", got, tc.wantBody)
			}
			if got := rec.Header().Get("Content-Encoding"); got != tc.wantEncoding {
				t.Errorf("Content-Encoding = %q, want %q", got, tc.wantEncoding)
			}
			if got := rec.Header().Get("Vary") != ""; got != tc.wantVary {
				t.Errorf("Vary present = %v, want %v", got, tc.wantVary)
			}
			if rec.Header().Get("ETag") == "" {
				t.Error("missing ETag")
			}
		})
	}
}

func TestStaticFiles_ConditionalRequests(t *testing.T) {
	h := handlerFromFS(staticTestFS(), nil)
	for _, path := range []string{"/assets/app-AbCd1234.js", "/theme-init.js", "/", "/apps/foo"} {
		t.Run(path, func(t *testing.T) {
			first := httptest.NewRecorder()
			h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, path, nil))
			etag := first.Header().Get("ETag")
			if etag == "" {
				t.Fatal("first response has no ETag")
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("If-None-Match", etag)
			second := httptest.NewRecorder()
			h.ServeHTTP(second, req)
			if second.Code != http.StatusNotModified {
				t.Errorf("status = %d, want 304", second.Code)
			}
			if second.Body.Len() != 0 {
				t.Errorf("304 carried a body of %d bytes", second.Body.Len())
			}
		})
	}
}

func TestAcceptedEncodings(t *testing.T) {
	tests := []struct {
		header string
		want   map[string]bool
	}{
		{"", map[string]bool{}},
		{"gzip, deflate, br", map[string]bool{"gzip": true, "deflate": true, "br": true}},
		{"br;q=0, gzip;q=0.5", map[string]bool{"br": false, "gzip": true}},
		{"GZIP; q=1.0", map[string]bool{"gzip": true}},
	}
	for _, tc := range tests {
		got := acceptedEncodings(tc.header)
		if len(got) != len(tc.want) {
			t.Errorf("%q: got %v, want %v", tc.header, got, tc.want)
			continue
		}
		for k, v := range tc.want {
			if got[k] != v {
				t.Errorf("%q: %s = %v, want %v", tc.header, k, got[k], v)
			}
		}
	}
}
