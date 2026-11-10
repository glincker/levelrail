package api

import (
	"bufio"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const headerServerTiming = "Server-Timing"

// statementTimingConfig reads the live timing settings; a nil service keeps timing off.
func (rt *Router) statementTimingConfig(r *http.Request) store.StatementTimingConfig {
	rs := rt.runtimeSettings
	if rs == nil {
		return store.StatementTimingConfig{}
	}
	ctx := r.Context()
	return store.StatementTimingConfig{
		Enabled:  rs.Bool(ctx, runtimesettings.KeyPerfStatementTiming),
		SlowMS:   rs.Int(ctx, runtimesettings.KeyPerfSlowStatementMS),
		RingSize: rs.Int(ctx, runtimesettings.KeyPerfSlowStatements),
	}
}

// serverTimingMiddleware applies the timing settings per request and, when on, counts the request's
// statements. Only API responses to root callers carry the header, so static assets and other callers never see it.
func (rt *Router) serverTimingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		store.SetStatementTiming(rt.statementTimingConfig(r))
		if !store.StatementTimingEnabled() || !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		ctx, stats := store.WithStatementStats(r.Context())
		r = r.WithContext(ctx)
		next.ServeHTTP(&serverTimingWriter{ResponseWriter: w, stats: stats}, r)
	})
}

type serverTimingWriter struct {
	http.ResponseWriter
	stats   *store.StatementStats
	decided bool
}

func (w *serverTimingWriter) decide() {
	if w.decided {
		return
	}
	w.decided = true
	if !w.stats.Privileged() {
		return
	}
	queries, total := w.stats.Snapshot()
	w.Header().Set(headerServerTiming, formatServerTiming(queries, total))
}

// markTimingCaller flags the request for Server-Timing once the full auth gate has passed a root caller.
func markTimingCaller(r *http.Request, abilities []string) {
	if hasAbility(abilities, AbilityRoot) {
		store.StatementStatsFrom(r.Context()).MarkPrivileged()
	}
}

func formatServerTiming(queries int64, total time.Duration) string {
	ms := strconv.FormatFloat(float64(total)/float64(time.Millisecond), 'f', 2, 64)
	return "db;dur=" + ms + ", queries;desc=\"" + strconv.FormatInt(queries, 10) + "\""
}

func (w *serverTimingWriter) WriteHeader(code int) {
	w.decide()
	w.ResponseWriter.WriteHeader(code)
}

func (w *serverTimingWriter) Write(b []byte) (int, error) {
	w.decide()
	return w.ResponseWriter.Write(b)
}

func (w *serverTimingWriter) Flush() {
	w.decide()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach hijack and deadlines on the real writer.
func (w *serverTimingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Hijack keeps WebSocket upgrades working through this wrapper.
func (w *serverTimingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	w.decided = true
	return h.Hijack()
}
