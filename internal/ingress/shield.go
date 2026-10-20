package ingress

import (
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

func init() {
	caddy.RegisterModule(shieldModule{})
}

// ShieldHandler is the wire shape of the levelrail_shield handler. Key is
// the route's first host, the same key request stats use.
type ShieldHandler struct {
	Handler string               `json:"handler"`
	Key     string               `json:"key"`
	Rule    trafficpolicy.Shield `json:"rule"`
}

func newShieldHandler(key string, s trafficpolicy.Shield) ShieldHandler {
	return ShieldHandler{Handler: "levelrail_shield", Key: key, Rule: s}
}

// shieldLimiters keeps per-host limiter state across config reloads so a
// reload does not hand every client a fresh bucket.
var shieldLimiters = struct {
	sync.Mutex
	byKey map[string]*trafficpolicy.IPLimiter
}{byKey: make(map[string]*trafficpolicy.IPLimiter)}

func shieldLimiterFor(key string, capacity int) *trafficpolicy.IPLimiter {
	shieldLimiters.Lock()
	defer shieldLimiters.Unlock()
	l, ok := shieldLimiters.byKey[key]
	if !ok {
		l = trafficpolicy.NewIPLimiter(capacity, nil)
		shieldLimiters.byKey[key] = l
	}
	return l
}

var (
	tarpitMu    sync.Mutex
	tarpitSlots chan struct{}
)

func tarpitSemaphore(n int) chan struct{} {
	tarpitMu.Lock()
	defer tarpitMu.Unlock()
	if tarpitSlots == nil || cap(tarpitSlots) != n {
		tarpitSlots = make(chan struct{}, n)
	}
	return tarpitSlots
}

// shieldModule (http.handlers.levelrail_shield) applies one domain's abuse
// limits. Client addresses live only in the capped in-memory limiter.
type shieldModule struct {
	Key  string               `json:"key"`
	Rule trafficpolicy.Shield `json:"rule"`

	limits  trafficpolicy.Limits
	allow   []netip.Prefix
	bots    *trafficpolicy.BotMatcher
	limiter *trafficpolicy.IPLimiter
	tarpit  chan struct{}
}

func (shieldModule) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.levelrail_shield",
		New: func() caddy.Module { return new(shieldModule) },
	}
}

// Provision parses the allowlist and bot list once per config load.
func (m *shieldModule) Provision(caddy.Context) error {
	m.limits = trafficpolicy.LimitsFromEnv(os.Getenv)
	for _, e := range m.Rule.Allow {
		p, err := trafficpolicy.ParseIPOrPrefix(e)
		if err != nil {
			return fmt.Errorf("ingress: shield allow %q: %w", e, err)
		}
		m.allow = append(m.allow, p)
	}
	bots, err := trafficpolicy.NewBotMatcher(m.limits, m.Rule.BotAgents)
	if err != nil {
		return fmt.Errorf("ingress: shield bot list: %w", err)
	}
	m.bots = bots
	m.limiter = shieldLimiterFor(m.Key, m.limits.ShieldMaxTrackedIPs)
	m.tarpit = tarpitSemaphore(m.limits.ShieldTarpitSlots)
	return nil
}

func (m *shieldModule) allowed(a netip.Addr) bool {
	for _, p := range m.allow {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func (m *shieldModule) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	eff := m.Rule.Effective(time.Now(), m.limits)
	if !eff.Active {
		return next.ServeHTTP(w, r)
	}
	client := requestClientAddr(r)
	if client.IsValid() && m.allowed(client) {
		return next.ServeHTTP(w, r)
	}
	if eff.BlockBots && m.bots.Match(r.UserAgent()) {
		m.reject(w, r, eff, trafficpolicy.ShieldBadBot)
		return nil
	}
	if eff.MaxBodyBytes > 0 {
		if r.ContentLength > eff.MaxBodyBytes {
			m.reject(w, r, eff, trafficpolicy.ShieldBodyTooLarge)
			return nil
		}
		r.Body = http.MaxBytesReader(w, r.Body, eff.MaxBodyBytes)
	}
	if client.IsValid() && (eff.RatePerMinute > 0 || eff.MaxConcurrent > 0) {
		release, reason, ok := m.limiter.Acquire(client, eff.RatePerMinute, eff.Burst, eff.MaxConcurrent)
		if !ok {
			m.reject(w, r, eff, reason)
			return nil
		}
		defer release()
	}
	return next.ServeHTTP(w, r)
}

func (m *shieldModule) reject(w http.ResponseWriter, r *http.Request, eff trafficpolicy.ShieldEffective, reason trafficpolicy.ShieldReason) {
	DefaultRequestStats().ObserveShield(m.Key, reason)
	caddyhttp.SetVar(r.Context(), shieldOutcomeVar, int(reason)+1)
	if eff.Tarpit > 0 && reason != trafficpolicy.ShieldBodyTooLarge {
		select {
		case m.tarpit <- struct{}{}:
			t := time.NewTimer(eff.Tarpit)
			select {
			case <-t.C:
			case <-r.Context().Done():
			}
			t.Stop()
			<-m.tarpit
		default:
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	switch reason {
	case trafficpolicy.ShieldBadBot:
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("Automated access is not allowed.\n"))
	case trafficpolicy.ShieldBodyTooLarge:
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		_, _ = w.Write([]byte("Request body is too large.\n"))
	default:
		w.Header().Set("Retry-After", strconv.Itoa(eff.RetryAfter))
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("Too many requests. Slow down and retry later.\n"))
	}
}

var (
	_ caddyhttp.MiddlewareHandler = (*shieldModule)(nil)
	_ caddy.Provisioner           = (*shieldModule)(nil)
)
