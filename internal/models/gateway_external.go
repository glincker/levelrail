package models

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

var (
	errLatencyBudget = errors.New("endpoint exceeded its latency budget")
	errUpstream5xx   = errors.New("endpoint answered with a server error")
)

// ExternalSource is the store surface the gateway needs for external endpoints.
type ExternalSource interface {
	ListInferenceEndpoints(ctx context.Context) ([]store.InferenceEndpoint, error)
	AddInferenceEndpointFallback(ctx context.Context, name string) error
}

// CredentialReader resolves an endpoint credential from the secrets manager.
type CredentialReader interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

type externalDeps struct {
	src       ExternalSource
	creds     CredentialReader
	transport http.RoundTripper
	fallbacks atomic.Int64
}

func (d *externalDeps) setTransport(t http.RoundTripper) { d.transport = t }

// SetCredentialReader lets the gateway attach endpoint credentials upstream.
func (g *Gateway) SetCredentialReader(r CredentialReader) { g.external.creds = r }

type externalTarget struct {
	ep   store.InferenceEndpoint
	cred string
}

func (g *Gateway) loadExternal(ctx context.Context) map[string]externalTarget {
	out := map[string]externalTarget{}
	src := g.external.src
	if src == nil {
		if es, ok := g.store.(ExternalSource); ok {
			g.external.src, src = es, es
		}
	}
	if src == nil {
		return out
	}
	list, err := src.ListInferenceEndpoints(ctx)
	if err != nil {
		g.logger.Error("models: gateway list endpoints failed", slog.String("error", err.Error()))
		return out
	}
	for _, e := range list {
		t := externalTarget{ep: e}
		if e.CredentialSet && g.external.creds != nil {
			c, err := g.external.creds.Resolve(ctx, store.ModelSecretsKey(e.Name), endpointCredentialKey)
			if err != nil {
				g.logger.Warn("models: endpoint credential unreadable", slog.String("model", e.Name), slog.String("error", err.Error()))
			}
			t.cred = c
		}
		out[e.Name] = t
	}
	return out
}

// upstream is one place the gateway can send a request.
type upstream struct {
	name     string
	target   *url.URL
	cred     string
	rt       http.RoundTripper
	modelRef string
	budget   time.Duration
}

func (g *Gateway) upstreamFor(m store.Model, ext map[string]externalTarget) *upstream {
	if m.Compute == store.ComputeExternal {
		t, ok := ext[m.Name]
		if !ok {
			return nil
		}
		u, err := url.Parse(t.ep.BaseURL)
		if err != nil {
			return nil
		}
		return &upstream{name: m.Name, target: u, cred: t.cred, rt: g.external.transport, budget: t.ep.LatencyBudget}
	}
	if m.EndpointDial == "" {
		return nil
	}
	return &upstream{name: m.Name, target: &url.URL{Scheme: "http", Host: m.EndpointDial}, rt: g.transport, modelRef: m.ModelRef}
}

func (g *Gateway) forwardExternal(w http.ResponseWriter, r *http.Request, m store.Model) {
	g.mu.Lock()
	ext, byName := g.ext, g.byName
	g.mu.Unlock()
	primary := g.upstreamFor(m, ext)
	if primary == nil {
		writeOpenAIError(w, http.StatusServiceUnavailable, "model_not_ready", "endpoint is not configured")
		return
	}
	ep := ext[m.Name].ep
	var fb *upstream
	if ep.FallbackModel != "" {
		if fm, ok := byName[ep.FallbackModel]; ok && ep.FallbackModel != m.Name {
			fb = g.upstreamFor(fm, ext)
			if fb != nil && ep.FallbackRef != "" {
				fb.modelRef = ep.FallbackRef
			}
			if fb != nil && fm.Compute == store.ComputeExternal {
				fb.budget = 0
			}
		}
	}
	var buf []byte
	if fb != nil && r.Body != nil && r.Body != http.NoBody {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "could not read the request body")
			return
		}
		buf = b
		r.Body = io.NopCloser(bytes.NewReader(buf))
	}
	if fb != nil && ep.HealthState == store.EndpointUnreachable {
		g.serveFallback(w, r, fb, buf, m.Name, "endpoint marked unreachable")
		return
	}
	var onFail func(error) bool
	if fb != nil {
		onFail = func(err error) bool {
			g.serveFallback(w, r, fb, buf, m.Name, err.Error())
			return true
		}
	}
	g.proxyUpstream(w, r, primary, fb != nil, onFail)
}

func (g *Gateway) serveFallback(w http.ResponseWriter, r *http.Request, fb *upstream, body []byte, from, reason string) {
	g.logger.Warn("models: gateway using fallback", slog.String("model", from), slog.String("fallback", fb.name), slog.String("reason", reason))
	if body != nil {
		body = rewriteModelField(body, fb.modelRef)
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
	}
	w.Header().Set("X-Fallback-From", from)
	g.countFallback(from)
	g.proxyUpstream(w, r, fb, false, nil)
}

func (g *Gateway) countFallback(name string) {
	g.external.fallbacks.Add(1)
	src := g.external.src
	if src == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := src.AddInferenceEndpointFallback(ctx, name); err != nil {
			g.logger.Warn("models: count fallback failed", slog.String("model", name), slog.String("error", err.Error()))
		}
	}()
}

func rewriteModelField(body []byte, ref string) []byte {
	if ref == "" {
		return body
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return body
	}
	if _, ok := fields["model"]; !ok {
		return body
	}
	raw, err := json.Marshal(ref)
	if err != nil {
		return body
	}
	fields["model"] = raw
	out, err := json.Marshal(fields)
	if err != nil {
		return body
	}
	return out
}

func (g *Gateway) proxyUpstream(w http.ResponseWriter, r *http.Request, up *upstream, failOn5xx bool, onFail func(error) bool) {
	clientCtx := r.Context()
	ctx, cancel := context.WithCancel(clientCtx)
	defer cancel()
	out := r.WithContext(ctx)
	rt := up.rt
	if up.budget > 0 {
		rt = budgetTransport{inner: up.rt, budget: up.budget}
	}
	proxy := &httputil.ReverseProxy{
		Transport: rt,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(up.target)
			pr.Out.Header.Del("Authorization")
			pr.Out.Header.Del("X-Forwarded-For")
			if up.cred != "" {
				pr.Out.Header.Set("Authorization", "Bearer "+up.cred)
			}
		},
		FlushInterval: -1,
		ModifyResponse: func(resp *http.Response) error {
			if failOn5xx && resp.StatusCode >= http.StatusInternalServerError {
				return errUpstream5xx
			}
			if g.limits.IdleTimeout > 0 {
				resp.Body = newIdleBody(resp.Body, g.limits.IdleTimeout, cancel)
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			if onFail != nil && clientCtx.Err() == nil && !isBodyTooLarge(err) && onFail(err) {
				return
			}
			g.upstreamError(clientCtx, w, up.name, err)
		},
	}
	proxy.ServeHTTP(w, out)
}

func isBodyTooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
}

// budgetTransport fails a request whose response headers have not arrived
// within budget, so the gateway can fall back instead of waiting on a slow endpoint.
type budgetTransport struct {
	inner  http.RoundTripper
	budget time.Duration
}

func (b budgetTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithCancel(req.Context())
	var expired atomic.Bool
	timer := time.AfterFunc(b.budget, func() {
		expired.Store(true)
		cancel()
	})
	resp, err := b.inner.RoundTrip(req.WithContext(ctx))
	if !timer.Stop() || err != nil {
		cancel()
		if resp != nil {
			_ = resp.Body.Close()
		}
		if expired.Load() {
			return nil, errLatencyBudget
		}
		return nil, err
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelBody) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}
