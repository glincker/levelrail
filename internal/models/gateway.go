package models

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
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
}

// Gateway authenticates OpenAI-compatible requests addressed to a model's
// host with the model's API key and reverse proxies them to the engine.
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

	mu       sync.Mutex
	loadedAt time.Time
	byHost   map[string]store.Model
}

// NewGateway builds a Gateway. hosts maps a model to the hostnames it is
// served on.
func NewGateway(st GatewayStore, hosts *HostResolver, logger *slog.Logger) *Gateway {
	if logger == nil {
		logger = slog.Default()
	}
	g := &Gateway{store: st, hosts: hosts, logger: logger}
	g.SetLimits(LoadGatewayLimits())
	return g
}

// SetLimits replaces the request limits. Call it before serving.
func (g *Gateway) SetLimits(l GatewayLimits) {
	g.limits = l
	g.transport = newGatewayTransport(l)
}

func (g *Gateway) lookup(ctx context.Context, host string) (store.Model, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.byHost == nil || time.Since(g.loadedAt) > gatewayCacheTTL {
		list, err := g.store.ListModels(ctx)
		if err != nil {
			g.logger.Error("models: gateway list models failed", slog.String("error", err.Error()))
			return store.Model{}, false
		}
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
	return m, ok
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
	m, ok := g.lookup(r.Context(), hostOnly(r.Host))
	if !ok {
		return false
	}
	sw := &statusWriter{ResponseWriter: w}
	start := time.Now()
	defer func() {
		g.logger.Info("models: gateway request", slog.String("method", r.Method), slog.String("model", m.Name),
			slog.Int("status", sw.status), slog.Duration("duration", time.Since(start)), slog.Int64("bytes", sw.bytes))
	}()
	g.serve(sw, r, m)
	return true
}

func (g *Gateway) serve(w http.ResponseWriter, r *http.Request, m store.Model) {
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
	if !found || !KeyMatches(strings.TrimSpace(token), m.APIKeyHash) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="model"`)
		writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "missing or invalid API key")
		return
	}
	if m.EndpointDial == "" {
		writeOpenAIError(w, http.StatusServiceUnavailable, "model_not_ready", "model is not running yet")
		return
	}
	lim := g.limits
	release, ok := g.inflight.acquire(m.Name, lim.MaxInflight)
	if !ok {
		w.Header().Set("Retry-After", strconv.Itoa(max(int(lim.RetryAfter/time.Second), 1)))
		writeOpenAIError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "too many concurrent requests for this model")
		return
	}
	defer release()
	if r.Method == http.MethodPost {
		if rerr := lim.prepareBody(w, r, !route.multipart); rerr != nil {
			writeOpenAIError(w, rerr.status, rerr.code, rerr.msg)
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
