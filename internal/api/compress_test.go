package api

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveCompress(h http.HandlerFunc, path string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	compressMiddleware(h).ServeHTTP(rec, req)
	return rec
}

func TestCompressMiddleware(t *testing.T) {
	t.Parallel()
	body := strings.Repeat(`{"name":"app"},`, 200)
	jsonHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
	gz := map[string]string{"Accept-Encoding": "gzip"}

	tests := []struct {
		name     string
		handler  http.HandlerFunc
		path     string
		hdr      map[string]string
		wantGzip bool
	}{
		{"json with gzip accepted", jsonHandler, "/api/v1/apps", gz, true},
		{"no accept-encoding", jsonHandler, "/api/v1/apps", nil, false},
		{"gzip refused with q=0", jsonHandler, "/api/v1/apps", map[string]string{"Accept-Encoding": "gzip;q=0"}, false},
		{"outside api prefix", jsonHandler, "/other", gz, false},
		{"auth prefix", jsonHandler, "/api/v1/auth/me", gz, false},
		{"set-cookie response", func(w http.ResponseWriter, r *http.Request) {
			http.SetCookie(w, &http.Cookie{Name: "s", Value: "v", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
			jsonHandler(w, r)
		}, "/api/v1/apps", gz, false},
		{"no-store secret response", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			jsonHandler(w, r)
		}, "/api/v1/apps/x/env/reveal", gz, false},
		{"sse stream", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: "+body+"\n\n")
			w.(http.Flusher).Flush()
		}, "/api/v1/apps/x/deploys/1/logs", gz, false},
		{"sse stream without accept header hint", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			_, _ = io.WriteString(w, "data: "+body+"\n\n")
		}, "/api/v1/apps/x/logs/stream", gz, false},
		{"streaming ndjson logs", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = io.WriteString(w, body)
			w.(http.Flusher).Flush()
		}, "/api/v1/apps/x/logs", gz, false},
		{"plain text logs", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, body)
		}, "/api/v1/apps/x/logs/download", gz, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveCompress(tc.handler, tc.path, tc.hdr)
			got := rec.Header().Get("Content-Encoding") == "gzip"
			if got != tc.wantGzip {
				t.Fatalf("gzip = %v, want %v (headers %v)", got, tc.wantGzip, rec.Header())
			}
			if tc.wantGzip {
				zr, err := gzip.NewReader(rec.Body)
				if err != nil {
					t.Fatalf("gzip reader: %v", err)
				}
				out, _ := io.ReadAll(zr)
				if string(out) != body {
					t.Fatalf("decoded body mismatch")
				}
				if rec.Body.Len() >= len(body) {
					t.Fatalf("compressed %d >= raw %d", rec.Body.Len(), len(body))
				}
			} else if tc.hdr != nil && !strings.Contains(rec.Body.String(), body) {
				t.Fatalf("body altered: %q", rec.Body.String()[:20])
			}
		})
	}
}

func TestCompressMiddleware_LiveJSONRouteThroughHandler(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	seedSearchApps(t, db, "alpha", "beta")
	tok := seedMatrixToken(t, db, "root", []string{AbilityRoot})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	bearer(tok)(req)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("status %d encoding %q", rec.Code, rec.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(zr)
	if !strings.Contains(string(out), `"alpha"`) {
		t.Fatalf("decoded list missing app: %s", out)
	}
}
