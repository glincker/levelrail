package models

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	gatewayCacheTTL = 3 * time.Second
)

// GatewayStore is the store surface the gateway reads.
type GatewayStore interface {
	ListModels(ctx context.Context) ([]store.Model, error)
	ListActiveModelKeys(ctx context.Context) ([]store.ModelKey, error)
}

// Gateway authenticates OpenAI-compatible requests addressed to a model's
// host with one of the model's virtual API keys and reverse proxies them to
// the engine, enforcing the key's limits and metering its usage.
// Only an allowlist of inference and listing routes is forwarded, so
// engine admin APIs (Ollama pull and delete, vLLM LoRA loading) are never
// exposed. Request size, generation length, concurrency and upstream
// timeouts are bounded by GatewayLimits.
type Gateway struct {
	store  GatewayStore
	hosts  *HostResolver
	logger *slog.Logger

	limits    GatewayLimits
	transport *http.Transport
	inflight  inflight
	keyLimits keyLimiter
	meter     *meter

	mu       sync.Mutex
	loadedAt time.Time
	byHost   map[string]store.Model
	keys     map[string][]store.ModelKey
}

// NewGateway builds a Gateway. hosts maps a model to the hostnames it is
// served on.
func NewGateway(st GatewayStore, hosts *HostResolver, logger *slog.Logger) *Gateway {
	if logger == nil {
		logger = slog.Default()
	}
	g := &Gateway{store: st, hosts: hosts, logger: logger, meter: newMeter(LoadMeterConfig(), logger)}
	g.SetLimits(LoadGatewayLimits())
	return g
}

// SetLimits replaces the request limits. Call it before serving.
func (g *Gateway) SetLimits(l GatewayLimits) {
	g.limits = l
	g.transport = newGatewayTransport(l)
}

// StartMetering flushes usage to st every flush interval until ctx ends.
func (g *Gateway) StartMetering(ctx context.Context, st UsageStore) {
	go g.meter.run(ctx, st)
}

// Invalidate makes the next request reload models and keys, so a revoked
// key stops working at once instead of after the cache TTL.
func (g *Gateway) Invalidate() {
	g.mu.Lock()
	g.loadedAt = time.Time{}
	g.mu.Unlock()
}

// InFlight returns the running request count per key id.
func (g *Gateway) InFlight() map[string]int { return g.keyLimits.inFlight() }

func (g *Gateway) lookup(ctx context.Context, host string) (store.Model, []store.ModelKey, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.byHost == nil || time.Since(g.loadedAt) > gatewayCacheTTL {
		list, err := g.store.ListModels(ctx)
		if err != nil {
			g.logger.Error("models: gateway list models failed", slog.String("error", err.Error()))
			return store.Model{}, nil, false
		}
		allKeys, err := g.store.ListActiveModelKeys(ctx)
		if err != nil {
			g.logger.Error("models: gateway list keys failed", slog.String("error", err.Error()))
			return store.Model{}, nil, false
		}
		g.keys = map[string][]store.ModelKey{}
		live := map[string]bool{}
		for _, k := range allKeys {
			g.keys[k.ModelName] = append(g.keys[k.ModelName], k)
			live[k.ID] = true
		}
		g.keyLimits.retain(live)
		g.byHost = map[string]store.Model{}
		for _, m := range list {
			if m.Deleting {
				continue
			}
			for _, h := range g.hosts.Hosts(m) {
				g.byHost[h] = m
			}
		}
		g.loadedAt = time.Now()
	}
	m, ok := g.byHost[host]
	return m, g.keys[m.Name], ok
}

func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return strings.ToLower(h)
	}
	return strings.ToLower(hostport)
}

// Handle serves r when its Host belongs to a model, returning true. It
// returns false without touching w for every other host.
func (g *Gateway) Handle(w http.ResponseWriter, r *http.Request) bool {
	m, keys, ok := g.lookup(r.Context(), hostOnly(r.Host))
	if !ok {
		return false
	}
	start := time.Now()
	sw := &statusWriter{ResponseWriter: w, obs: newResponseObserver(start, g.meter.cfg.ScanBytes)}
	defer func() {
		g.logger.Info("models: gateway request", slog.String("method", r.Method), slog.String("model", m.Name),
			slog.Int("status", sw.status), slog.Duration("duration", time.Since(start)), slog.Int64("bytes", sw.bytes))
		g.observe(m.Name, sw, start)
	}()
	g.serve(sw, r, m, keys)
	return true
}

// observe records a finished authenticated request; requests without a
// known key are not metered.
func (g *Gateway) observe(model string, sw *statusWriter, start time.Time) {
	if sw.keyID == "" {
		return
	}
	o := observation{model: model, keyID: sw.keyID, at: start, status: sw.status, rateLimited: sw.rateLimited,
		duration: time.Since(start), ttft: sw.obs.ttft, bytes: sw.bytes}
	if u, ok := sw.obs.usage(); ok {
		o.usage = &u
		g.keyLimits.addTokens(sw.keyID, u.input+u.output, time.Now())
	}
	g.meter.record(o)
}

func (g *Gateway) serve(w *statusWriter, r *http.Request, m store.Model, keys []store.ModelKey) {
	route, match := matchRoute(m.Engine, r.URL.Path, r.Method)
	if hasEncodedSlash(r.URL.EscapedPath()) {
		match = routeNotFound
	}
	switch match {
	case routeNotFound:
		writeOpenAIError(w, http.StatusNotFound, "not_found", "route not served")
		return
	case routeWrongMethod:
		w.Header().Set("Allow", allowedMethods(m.Engine, r.URL.Path))
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed for this route")
		return
	case routeAllowed:
	}
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	key, authed := authenticate(keys, strings.TrimSpace(token), time.Now())
	if !found || !authed {
		w.Header().Set("WWW-Authenticate", `Bearer realm="model"`)
		writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "missing or invalid API key")
		return
	}
	w.keyID = key.ID
	if !pathAllowed(key, r.URL.Path) {
		writeOpenAIError(w, http.StatusForbidden, "permission_denied", "this API key may not use this route or model")
		return
	}
	if m.EndpointDial == "" {
		writeOpenAIError(w, http.StatusServiceUnavailable, "model_not_ready", "model is not running yet")
		return
	}
	releaseKey, wait, admitted := g.keyLimits.acquire(key, time.Now())
	if !admitted {
		w.rateLimited = true
		w.Header().Set("Retry-After", strconv.Itoa(max(int(math.Ceil(wait.Seconds())), 1)))
		writeOpenAIError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "rate limit exceeded for this API key")
		return
	}
	defer releaseKey()
	lim := g.limits
	release, ok := g.inflight.acquire(m.Name, lim.MaxInflight)
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(max(int(lim.RetryAfter/time.Second), 1)))
		writeOpenAIError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "too many concurrent requests for this model")
		return
	}
	defer release()
	if r.Method == http.MethodPost {
		info, rerr := lim.prepareBodyInfo(w, r, !route.multipart)
		if rerr != nil {
			writeOpenAIError(w, rerr.status, rerr.code, rerr.msg)
			return
		}
		if !modelAllowed(key, info.model, info.hasJSON) {
			writeOpenAIError(w, http.StatusForbidden, "permission_denied", "this API key may not use this route or model")
			return
		}
	}
	g.forward(w, r, m)
}

func (g *Gateway) forward(w http.ResponseWriter, r *http.Request, m store.Model) {
	clientCtx := r.Context()
	ctx, cancel := context.WithCancel(clientCtx)
	defer cancel()
	r = r.WithContext(ctx)
	target := &url.URL{Scheme: "http", Host: m.EndpointDial}
	proxy := &httputil.ReverseProxy{
		Transport: g.transport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("X-Forwarded-For")
		},
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			if g.limits.IdleTimeout > 0 {
				resp.Body = newIdleBody(resp.Body, g.limits.IdleTimeout, cancel)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			g.upstreamError(clientCtx, w, m.Name, err)
		},
	}
	proxy.ServeHTTP(w, r)
}

func (g *Gateway) upstreamError(clientCtx context.Context, w http.ResponseWriter, model string, err error) {
	var mbe *http.MaxBytesError
	var ne net.Error
	switch {
	case errors.As(err, &mbe):
		writeOpenAIError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds the size limit")
	case clientCtx.Err() != nil:
		g.logger.Debug("models: gateway client went away", slog.String("model", model))
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()):
		g.logger.Warn("models: gateway upstream timed out", slog.String("model", model), slog.String("error", err.Error()))
		writeOpenAIError(w, http.StatusGatewayTimeout, "gateway_timeout", "model engine did not respond in time")
	default:
		g.logger.Warn("models: gateway upstream failed", slog.String("model", model), slog.String("error", err.Error()))
		writeOpenAIError(w, http.StatusBadGateway, "upstream_error", "model engine is unreachable")
	}
}

// hasDotSegment reports a "." or ".." path segment, which the proxy would
// forward uncleaned and let a /v1/ request reach the engine's admin routes.
func hasDotSegment(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}

// Middleware wraps next so requests for model hosts are handled by the
// gateway.
func (g *Gateway) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.Handle(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeOpenAIError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": msg, "type": code, "code": code}})
}
