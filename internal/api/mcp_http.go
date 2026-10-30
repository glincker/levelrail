package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/mcptools"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	mcpHTTPPath = "/mcp"
	// AuditActionMCPToolCall is the audit action of one remote MCP tool call.
	AuditActionMCPToolCall = "mcp.tool_call"

	envMCPHTTPRPM          = "APP_MCP_HTTP_RPM"
	envMCPHTTPMaxBody      = "APP_MCP_HTTP_MAX_BODY_BYTES"
	envMCPHTTPMaxResponse  = "APP_MCP_HTTP_MAX_RESPONSE_BYTES"
	envMCPHTTPCallTimeout  = "APP_MCP_HTTP_CALL_TIMEOUT_SECONDS"
	envMCPHTTPOrigins      = "APP_MCP_HTTP_ALLOWED_ORIGINS"
	mcpHTTPDefaultRPM      = 60
	mcpHTTPDefaultBody     = 256 << 10
	mcpHTTPDefaultResponse = 4 << 20
	mcpHTTPDefaultTimeout  = 30
	mcpHTTPInternalHost    = "http://mcp.internal"
	mcpHTTPAgentClient     = "mcp-http"
	mcpHTTPMaxArgKeys      = 32
	mcpHTTPMaxKeyLen       = 64
)

// mcpHTTP serves the experimental read-only remote MCP endpoint. inner is the
// full router stack, set once by Handler, so tool calls take the REST path.
type mcpHTTP struct {
	rt           *Router
	inner        http.Handler
	limit        *apiRateLimiter
	maxBody      int64
	maxResponse  int64
	callTimeout  time.Duration
	extraOrigins []string
}

func splitOrigins(raw string) []string {
	var out []string
	for _, o := range strings.Split(raw, ",") {
		if n := normalizeOrigin(o); n != "" {
			out = append(out, n)
		}
	}
	return out
}

func normalizeOrigin(o string) string {
	u, err := url.Parse(strings.TrimSpace(o))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// originAllowed passes requests with no Origin (non-browser clients) and
// otherwise only the dashboard origin plus the env allowlist. Matching the
// Host header would not stop DNS rebinding, so it is never consulted.
func (m *mcpHTTP) originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	n := normalizeOrigin(origin)
	if n == "" {
		return false
	}
	if dash := normalizeOrigin(configuredDashboardURL()); dash != "" && dash == n {
		return true
	}
	return slices.Contains(m.extraOrigins, n)
}

func (m *mcpHTTP) serve(w http.ResponseWriter, r *http.Request) {
	if !experimental.Enabled(experimental.MCPHTTP) || m.inner == nil {
		writeError(w, http.StatusNotFound, experimental.DisabledMessage(experimental.MCPHTTP))
		return
	}
	if !m.originAllowed(r) {
		writeError(w, http.StatusForbidden, "origin not allowed")
		return
	}
	token, ok := bearerToken(r)
	if !ok {
		if allowed, retry := m.limit.allow("ip:" + clientIP(r)); !allowed {
			writeRateLimited(w, retry)
			return
		}
		w.Header().Set("WWW-Authenticate", `Bearer realm="levelrail-mcp"`)
		writeError(w, http.StatusUnauthorized, "bearer API token required")
		return
	}
	if allowed, retry := m.limit.allow("token:" + hashToken(token)); !allowed {
		writeRateLimited(w, retry)
		return
	}
	rec, status, msg := m.authenticate(r.Context(), token)
	if rec == nil {
		if status == http.StatusUnauthorized {
			w.Header().Set("WWW-Authenticate", `Bearer realm="levelrail-mcp"`)
		}
		writeError(w, status, msg)
		return
	}
	body, tooLarge := m.readBody(r)
	if tooLarge {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	// Ignore any session cookie: only the bearer token identifies the caller here.
	r.Header.Del("Cookie")

	start := time.Now()
	client := apiclient.NewClient(mcpHTTPInternalHost, token,
		apiclient.WithUserAgent("levelrail-mcp/"+version.Version),
		apiclient.WithTransport(&mcpInprocTransport{inner: m.inner, origin: r, maxResponse: m.maxResponse, timeout: m.callTimeout}))
	server, _ := mcptools.NewServerWithOptions(client, mcptools.Options{Mode: mcptools.ModeReadOnly})
	server.AddReceivingMiddleware(m.auditMiddleware(r, rec))
	m.rt.logger.Debug("api: mcp-http server built", slog.Duration("took", time.Since(start)))

	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	h.ServeHTTP(w, r)
}

// authenticate mirrors the token checks of requireAbilityDecided. Per-call
// ability, IAM, AI control and binding checks run again on every REST call.
func (m *mcpHTTP) authenticate(ctx context.Context, token string) (*store.APIToken, int, string) {
	rec, err := m.rt.lookupBearerToken(ctx, token)
	if errors.Is(err, store.ErrAPITokenNotFound) {
		return nil, http.StatusUnauthorized, "invalid token"
	}
	if err != nil {
		m.rt.logger.Error("api: mcp-http token lookup failed", slog.String("error", err.Error()))
		return nil, http.StatusInternalServerError, "internal error"
	}
	if rec.RevokedAt != nil {
		return nil, http.StatusUnauthorized, "token revoked"
	}
	if rec.ExpiresAt != nil && time.Now().After(*rec.ExpiresAt) {
		return nil, http.StatusUnauthorized, "token expired"
	}
	if gone, oerr := m.rt.tokenOwnerGone(ctx, rec); oerr != nil {
		m.rt.logger.Error("api: mcp-http token owner lookup failed", slog.String("error", oerr.Error()))
		return nil, http.StatusInternalServerError, "internal error"
	} else if gone {
		return nil, http.StatusUnauthorized, "invalid token"
	}
	if !hasAbility(rec.Abilities, AbilityRead) {
		return nil, http.StatusForbidden, "token lacks the required ability (needs " + AbilityRead + ")"
	}
	return rec, 0, ""
}

// readBody reads at most maxBody bytes and reports an oversize request.
func (m *mcpHTTP) readBody(r *http.Request) ([]byte, bool) {
	if r.ContentLength > m.maxBody {
		return nil, true
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, m.maxBody+1))
	if err != nil || int64(len(b)) > m.maxBody {
		return nil, int64(len(b)) > m.maxBody
	}
	return b, false
}

// auditMiddleware writes one audit row per tools/call. It stores argument
// names only, never values, because tool inputs can carry secrets.
func (m *mcpHTTP) auditMiddleware(r *http.Request, rec *store.APIToken) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			start := time.Now()
			res, err := next(ctx, method, req)
			outcome := "ok"
			if cr, isCall := res.(*mcp.CallToolResult); err != nil || (isCall && cr.IsError) {
				outcome = "error"
			}
			tool, keys := mcpCallSummary(req)
			m.writeAudit(r, rec, tool, keys, outcome, time.Since(start))
			return res, err
		}
	}
}

func mcpCallSummary(req mcp.Request) (string, []string) {
	var name string
	var raw json.RawMessage
	switch p := req.GetParams().(type) {
	case *mcp.CallToolParamsRaw:
		name, raw = p.Name, p.Arguments
	case *mcp.CallToolParams:
		name = p.Name
		if p.Arguments != nil {
			raw, _ = json.Marshal(p.Arguments)
		}
	}
	var args map[string]json.RawMessage
	_ = json.Unmarshal(raw, &args)
	keys := make([]string, 0, len(args))
	for k := range args {
		if len(keys) < mcpHTTPMaxArgKeys {
			keys = append(keys, mcpTruncate(k, mcpHTTPMaxKeyLen))
		}
	}
	slices.Sort(keys)
	return mcpTruncate(name, mcpHTTPMaxKeyLen), keys
}

func mcpTruncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func (m *mcpHTTP) writeAudit(r *http.Request, rec *store.APIToken, tool string, argKeys []string, outcome string, took time.Duration) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		m.rt.logger.Warn("api: generate mcp audit entry id failed", slog.String("error", err.Error()))
		return
	}
	detail, _ := json.Marshal(map[string]any{
		"tool": tool, "token_id": rec.ID, "outcome": outcome,
		"duration_ms": took.Milliseconds(), "argument_names": argKeys,
	})
	status := http.StatusOK
	if outcome != "ok" {
		status = http.StatusBadRequest
	}
	entry := store.AuditEntry{
		ID: id, ActorType: "token", ActorID: rec.ID, ActorName: rec.Name,
		Ability: AbilityRead, Method: http.MethodPost, Path: mcpHTTPPath, StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: "mcp", AgentName: rec.AgentName, AgentClient: mcpHTTPAgentClient,
		Action: AuditActionMCPToolCall, Detail: string(detail),
	}
	if err := m.rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		m.rt.logger.Warn("api: save mcp audit entry failed", slog.String("error", err.Error()))
		return
	}
	m.rt.emitAuditEvent(entry)
}

// mcpInprocTransport runs a tool's REST call against the router in process,
// keeping the caller's address so access policies and audit IPs stay correct.
type mcpInprocTransport struct {
	inner       http.Handler
	origin      *http.Request
	maxResponse int64
	timeout     time.Duration
}

func (t *mcpInprocTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(req.Context(), t.timeout)
	defer cancel()
	in := req.Clone(ctx)
	in.URL = &url.URL{Path: req.URL.Path, RawPath: req.URL.RawPath, RawQuery: req.URL.RawQuery}
	in.RequestURI = in.URL.RequestURI()
	in.Host = t.origin.Host
	in.RemoteAddr = t.origin.RemoteAddr
	in.TLS = t.origin.TLS
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Real-Ip"} {
		in.Header.Del(h)
		for _, v := range t.origin.Header.Values(h) {
			in.Header.Add(h, v)
		}
	}
	in.Header.Del("Cookie")
	if in.Header.Get(AgentClientHeader) == "" {
		in.Header.Set(AgentClientHeader, mcpHTTPAgentClient)
	}
	rec := &mcpBufferedResponse{header: http.Header{}, status: http.StatusOK, limit: t.maxResponse}
	t.inner.ServeHTTP(rec, in)
	if rec.overflow {
		return nil, errors.New("tool response exceeded the remote MCP size limit")
	}
	return &http.Response{
		Status: http.StatusText(rec.status), StatusCode: rec.status, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: rec.header, Body: io.NopCloser(bytes.NewReader(rec.buf.Bytes())),
		ContentLength: int64(rec.buf.Len()), Request: req,
	}, nil
}

// mcpBufferedResponse buffers a handler's reply up to limit bytes.
type mcpBufferedResponse struct {
	header   http.Header
	status   int
	buf      bytes.Buffer
	limit    int64
	overflow bool
}

func (b *mcpBufferedResponse) Header() http.Header { return b.header }
func (b *mcpBufferedResponse) WriteHeader(s int)   { b.status = s }
func (b *mcpBufferedResponse) Flush()              {}
func (b *mcpBufferedResponse) Write(p []byte) (int, error) {
	if int64(b.buf.Len()+len(p)) > b.limit {
		b.overflow = true
		return 0, errors.New("response too large")
	}
	return b.buf.Write(p)
}
