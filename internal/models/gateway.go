package models

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	gatewayCacheTTL = 3 * time.Second
	openAIPrefix    = "/v1/"
)

// GatewayStore is the store surface the gateway reads.
type GatewayStore interface {
	ListModels(ctx context.Context) ([]store.Model, error)
}

// Gateway authenticates OpenAI-compatible requests addressed to a model's
// host with the model's API key and reverse proxies them to the engine.
// Only /v1/ paths are forwarded, so engine admin APIs (Ollama pull and
// delete) are never exposed.
type Gateway struct {
	store  GatewayStore
	hosts  *HostResolver
	logger *slog.Logger

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
	return &Gateway{store: st, hosts: hosts, logger: logger}
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
	if !strings.HasPrefix(r.URL.Path, openAIPrefix) || hasDotSegment(r.URL.Path) {
		writeOpenAIError(w, http.StatusNotFound, "not_found", "only OpenAI-compatible /v1/ routes are served")
		return true
	}
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found || !KeyMatches(strings.TrimSpace(token), m.APIKeyHash) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="model"`)
		writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "missing or invalid API key")
		return true
	}
	if m.EndpointDial == "" {
		writeOpenAIError(w, http.StatusServiceUnavailable, "model_not_ready", "model is not running yet")
		return true
	}
	target := &url.URL{Scheme: "http", Host: m.EndpointDial}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("X-Forwarded-For")
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			g.logger.Warn("models: gateway upstream failed", slog.String("model", m.Name), slog.String("error", err.Error()))
			writeOpenAIError(w, http.StatusBadGateway, "upstream_error", "model engine is unreachable")
		},
	}
	proxy.ServeHTTP(w, r)
	return true
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
