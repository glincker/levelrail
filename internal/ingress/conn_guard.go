package ingress

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

const connGuardLimiterKey = "conn-guard"

func init() {
	caddy.RegisterModule(connGuardModule{})
}

// ConnGuardHandler is the wire shape of the levelrail_conn_guard handler.
type ConnGuardHandler struct {
	Handler         string `json:"handler"`
	MaxPerIP        int    `json:"max_per_ip,omitempty"`
	BodyReadTimeout string `json:"body_read_timeout,omitempty"`
}

// connGuardModule caps concurrent requests per client address and bounds the
// body read time. Upgrades and event streams are exempt so they stay open.
type connGuardModule struct {
	MaxPerIP        int    `json:"max_per_ip,omitempty"`
	BodyReadTimeout string `json:"body_read_timeout,omitempty"`

	timeout time.Duration
	limiter *trafficpolicy.IPLimiter
}

func (connGuardModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.levelrail_conn_guard",
		New: func() caddy.Module { return new(connGuardModule) },
	}
}

func (m *connGuardModule) Provision(caddy.Context) error {
	if d, err := time.ParseDuration(m.BodyReadTimeout); err == nil && d > 0 {
		m.timeout = d
	}
	m.limiter = shieldLimiterFor(connGuardLimiterKey, trafficpolicy.LimitsFromEnv(os.Getenv).ShieldMaxTrackedIPs)
	return nil
}

func isStreamingRequest(r *http.Request) bool {
	return r.Header.Get("Upgrade") != "" || strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

func (m *connGuardModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	if isStreamingRequest(r) {
		return next.ServeHTTP(w, r)
	}
	if client := requestClientAddr(r); m.MaxPerIP > 0 && client.IsValid() {
		release, _, ok := m.limiter.Acquire(client, 0, 0, m.MaxPerIP)
		if !ok {
			w.Header().Set("Retry-After", "1")
			return caddyhttp.Error(http.StatusTooManyRequests, http.ErrAbortHandler)
		}
		defer release()
	}
	if m.timeout > 0 && r.Body != nil && r.Body != http.NoBody {
		rc := http.NewResponseController(w)
		if rc.SetReadDeadline(time.Now().Add(m.timeout)) == nil {
			// The server only resets the deadline per request when it has its own read timeout.
			defer func() { _ = rc.SetReadDeadline(time.Time{}) }()
		}
	}
	return next.ServeHTTP(w, r)
}
