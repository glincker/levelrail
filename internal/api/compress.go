package api

import (
	"compress/gzip"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

// envAPICompression set to "off" disables response compression.
const envAPICompression = "APP_API_COMPRESSION"

var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

// compressMiddleware gzips GET JSON responses under /api/ for clients that
// accept it. Responses that set a cookie, carry Cache-Control: no-store (the
// marker every secret-bearing handler already sets), live under /api/v1/auth/,
// or are not plain JSON (SSE, NDJSON log streams, archives) pass through
// untouched, which keeps secrets out of compressed bodies that an attacker
// could probe with reflected input (BREACH).
func compressMiddleware(next http.Handler) http.Handler {
	if strings.EqualFold(os.Getenv(envAPICompression), "off") {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !compressEligibleRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		cw := &compressWriter{ResponseWriter: w}
		defer cw.close()
		next.ServeHTTP(cw, r)
	})
}

func compressEligibleRequest(r *http.Request) bool {
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/") ||
		strings.HasPrefix(r.URL.Path, "/api/v1/auth/") {
		return false
	}
	if r.Header.Get("Range") != "" || r.Header.Get("Upgrade") != "" ||
		strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		return false
	}
	return acceptsGzip(r.Header.Get("Accept-Encoding"))
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
			continue
		}
		if q, ok := strings.CutPrefix(strings.ReplaceAll(strings.TrimSpace(params), " ", ""), "q="); ok {
			if v, err := strconv.ParseFloat(q, 64); err == nil && v <= 0 {
				return false
			}
		}
		return true
	}
	return false
}

func compressibleResponse(h http.Header, status int) bool {
	if status < 200 || status == http.StatusNoContent || status == http.StatusNotModified {
		return false
	}
	if h.Get("Content-Encoding") != "" || h.Get("Content-Range") != "" || len(h.Values("Set-Cookie")) > 0 ||
		strings.Contains(strings.ToLower(h.Get("Cache-Control")), "no-store") {
		return false
	}
	mt, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	return err == nil && (mt == "application/json" || strings.HasSuffix(mt, "+json"))
}

type compressWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
}

func (c *compressWriter) WriteHeader(status int) {
	if c.wroteHeader {
		return
	}
	c.wroteHeader = true
	h := c.Header()
	if compressibleResponse(h, status) {
		h.Set("Content-Encoding", "gzip")
		h.Del("Content-Length")
		if etag := h.Get("Etag"); etag != "" && !strings.HasPrefix(etag, "W/") {
			h.Set("Etag", "W/"+etag)
		}
		c.gz = gzipPool.Get().(*gzip.Writer)
		c.gz.Reset(c.ResponseWriter)
	}
	c.ResponseWriter.WriteHeader(status)
}

func (c *compressWriter) Write(b []byte) (int, error) {
	if !c.wroteHeader {
		c.WriteHeader(http.StatusOK)
	}
	if c.gz == nil {
		return c.ResponseWriter.Write(b)
	}
	return c.gz.Write(b)
}

func (c *compressWriter) Flush() {
	if c.gz != nil {
		_ = c.gz.Flush()
	}
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *compressWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

func (c *compressWriter) close() {
	if c.gz == nil {
		return
	}
	_ = c.gz.Close()
	gzipPool.Put(c.gz)
	c.gz = nil
}
