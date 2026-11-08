package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// envConditionalGetMaxBytes caps the body buffered to compute an ETag; bigger
// responses stream through without one.
const envConditionalGetMaxBytes = "APP_API_CONDITIONAL_GET_MAX_BYTES"

const defaultConditionalGetMaxBytes = 4 << 20

func conditionalGetMaxBytes() int {
	if n, err := strconv.Atoi(os.Getenv(envConditionalGetMaxBytes)); err == nil && n > 0 {
		return n
	}
	return defaultConditionalGetMaxBytes
}

// conditionalGetMiddleware adds a strong ETag to plain JSON GET responses and
// answers a matching If-None-Match with 304. Responses marked no-store (the
// secret-bearing handlers), cookie setters, auth routes, sensitive reads and
// streams never get a validator. It sits inside compress so the hash covers
// the uncompressed body.
func conditionalGetMiddleware(enabled func(*http.Request) bool, next http.Handler) http.Handler {
	limit := conditionalGetMaxBytes()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !conditionalEligibleRequest(r) || (enabled != nil && !enabled(r)) {
			next.ServeHTTP(w, r)
			return
		}
		cw := &conditionalWriter{ResponseWriter: w, req: r, limit: limit}
		next.ServeHTTP(cw, r)
		cw.finish()
	})
}

func conditionalEligibleRequest(r *http.Request) bool {
	if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/api/") ||
		strings.HasPrefix(r.URL.Path, "/api/v1/auth/") || sensitiveReadRoute(r) {
		return false
	}
	return r.Header.Get("Range") == "" && r.Header.Get("Upgrade") == "" &&
		!strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

type conditionalWriter struct {
	http.ResponseWriter
	req         *http.Request
	limit       int
	status      int
	buf         bytes.Buffer
	decided     bool
	passthrough bool
}

func (c *conditionalWriter) WriteHeader(status int) {
	if c.decided {
		return
	}
	c.decided = true
	h := c.Header()
	if status != http.StatusOK || h.Get("Etag") != "" || !compressibleResponse(h, status) {
		c.passthrough = true
		c.ResponseWriter.WriteHeader(status)
		return
	}
	c.status = status
}

func (c *conditionalWriter) Write(b []byte) (int, error) {
	if !c.decided {
		c.WriteHeader(http.StatusOK)
	}
	if c.passthrough {
		return c.ResponseWriter.Write(b)
	}
	if c.buf.Len()+len(b) > c.limit {
		c.spill()
		return c.ResponseWriter.Write(b)
	}
	return c.buf.Write(b)
}

// spill gives up on an ETag for an oversized body and streams what was held.
func (c *conditionalWriter) spill() {
	c.passthrough = true
	c.ResponseWriter.WriteHeader(c.status)
	_, _ = c.ResponseWriter.Write(c.buf.Bytes()) //nolint:gosec // replays the handler body unchanged
	c.buf.Reset()
}

func (c *conditionalWriter) Flush() {
	if !c.passthrough && c.decided {
		c.spill()
	}
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (c *conditionalWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

func (c *conditionalWriter) finish() {
	if c.passthrough {
		return
	}
	if !c.decided {
		return
	}
	sum := sha256.Sum256(c.buf.Bytes())
	etag := `"` + hex.EncodeToString(sum[:16]) + `"`
	h := c.Header()
	h.Set("Etag", etag)
	if h.Get("Cache-Control") == "" {
		h.Set("Cache-Control", "private, no-cache")
	}
	if ifNoneMatchHit(c.req.Header.Get("If-None-Match"), etag) {
		h.Del("Content-Type")
		h.Del("Content-Length")
		c.ResponseWriter.WriteHeader(http.StatusNotModified)
		return
	}
	c.ResponseWriter.WriteHeader(c.status)
	_, _ = c.ResponseWriter.Write(c.buf.Bytes())
}

// ifNoneMatchHit uses the weak comparison RFC 9110 requires for If-None-Match.
func ifNoneMatchHit(header, etag string) bool {
	if header == "" {
		return false
	}
	for _, part := range strings.Split(header, ",") {
		p := strings.TrimPrefix(strings.TrimSpace(part), "W/")
		if p == "*" || p == etag {
			return true
		}
	}
	return false
}

// conditionalGetEnabled reads api.conditional_get live; unset settings mean on.
func (rt *Router) conditionalGetEnabled(r *http.Request) bool {
	return rt.runtimeSettings == nil || rt.runtimeSettings.Bool(r.Context(), runtimesettings.KeyAPIConditionalGet)
}
