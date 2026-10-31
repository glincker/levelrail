package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/monitor"
)

// MonitorHeartbeats is the optional heartbeat half of the monitor engine.
type MonitorHeartbeats interface {
	Ping(ctx context.Context, token, signal string) error
	RotateToken(ctx context.Context, id string) (monitor.Monitor, error)
	Config() monitor.Config
}

func (rt *Router) heartbeats() (MonitorHeartbeats, bool) {
	if rt.monitors == nil {
		return nil, false
	}
	h, ok := rt.monitors.(MonitorHeartbeats)
	return h, ok
}

// handleHeartbeatPing is public: the token is the credential. Every miss answers the same bare 404
// after a minimum response time, so it never says whether a token exists.
func (rt *Router) handleHeartbeatPing(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	h, ok := rt.heartbeats()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	cfg := h.Config()
	rt.heartbeatMu.Lock()
	if rt.heartbeatLimiter == nil {
		rt.heartbeatLimiter = newAPIRateLimiter(cfg.HeartbeatPerMinute)
	}
	lim := rt.heartbeatLimiter
	rt.heartbeatMu.Unlock()
	if allowed, retry := lim.allow(clientIP(r)); !allowed {
		w.Header().Set("Retry-After", retryAfterSeconds(retry))
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	if _, err := io.Copy(io.Discard, http.MaxBytesReader(w, r.Body, int64(cfg.HeartbeatBodyMax))); err != nil {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}
	var signal string
	switch sig := r.PathValue("signal"); sig {
	case "":
		signal = monitor.PingOK
	case monitor.PingStart, monitor.PingFail:
		signal = sig
	default:
		heartbeatPad(start, cfg.HeartbeatMinResponse)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	err := h.Ping(r.Context(), r.PathValue("token"), signal)
	heartbeatPad(start, cfg.HeartbeatMinResponse)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, monitor.ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
	default:
		rt.internalError(w, "api: heartbeat ping failed", err)
	}
}

func heartbeatPad(start time.Time, floor time.Duration) {
	if rest := floor - time.Since(start); rest > 0 {
		time.Sleep(rest)
	}
}

func retryAfterSeconds(d time.Duration) string {
	secs := int(d/time.Second) + 1
	return strconv.Itoa(secs)
}

func (rt *Router) handleRotateHeartbeat(w http.ResponseWriter, r *http.Request) {
	if !rt.monitorsReady(w) {
		return
	}
	h, ok := rt.heartbeats()
	if !ok {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	m, err := h.RotateToken(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.monitorError(w, "rotate heartbeat token", err)
		return
	}
	writeJSON(w, http.StatusOK, toMonitorResource(m))
}
