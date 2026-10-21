package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/functions"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/kit/ids"
)

type functionCaptureStore interface {
	FunctionCaptureEnabled(ctx context.Context, name string) (bool, error)
	SetFunctionCapture(ctx context.Context, name string, enabled bool) error
}

type functionInputs struct {
	once  sync.Once
	store *functions.InputStore
	lim   functions.InvokeLimits
}

func (rt *Router) functionInputStore() (*functions.InputStore, functions.InvokeLimits) {
	rt.fnInputs.once.Do(func() {
		rt.fnInputs.lim = functions.InvokeLimitsFromEnv(nil)
		rt.fnInputs.store = functions.NewInputStore(rt.fnInputs.lim)
	})
	return rt.fnInputs.store, rt.fnInputs.lim
}

// FunctionInvokeRequest is one test call to a function through its public route.
type FunctionInvokeRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

// FunctionInvokeResult is what the function answered, plus how the call went.
type FunctionInvokeResult struct {
	RequestID     string              `json:"request_id"`
	Status        int                 `json:"status"`
	Headers       map[string][]string `json:"headers"`
	Body          string              `json:"body"`
	BodyTruncated bool                `json:"body_truncated,omitempty"`
	DurationMs    float64             `json:"duration_ms"`
	ColdStart     bool                `json:"cold_start"`
	Stored        bool                `json:"input_stored"`
}

var invokeSkippedHeaders = map[string]bool{
	"host": true, "content-length": true, "connection": true, "x-request-id": true, "transfer-encoding": true,
}

// functionBaseURL resolves a function's public URL, or "" when name is not a function.
func (rt *Router) functionBaseURL(ctx context.Context, name string) (string, error) {
	sleep, err := rt.appSleep.GetAppSleep(ctx, name)
	if err != nil {
		return "", fmt.Errorf("load sleep settings: %w", err)
	}
	if !sleep.HoldRequests || sleep.IdleMinutes <= 0 {
		return "", nil
	}
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return "", fmt.Errorf("load service: %w", err)
	}
	if len(svc.Domains) > 0 {
		return "https://" + svc.Domains[0], nil
	}
	return rt.fallbackURLFor(ctx, *svc), nil
}

func (rt *Router) captureEnabled(ctx context.Context, name string) bool {
	cs, ok := rt.apps.(functionCaptureStore)
	if !ok {
		return false
	}
	on, err := cs.FunctionCaptureEnabled(ctx, name)
	if err != nil {
		rt.logger.Warn("api: read function capture failed", slog.String("function", name), slog.String("error", err.Error()))
		return false
	}
	return on
}

// runFunctionInvoke sends the request to the function's real route, following the wake redirect.
func (rt *Router) runFunctionInvoke(ctx context.Context, name, base string, in FunctionInvokeRequest) (FunctionInvokeResult, error) {
	inputs, lim := rt.functionInputStore()
	method := strings.ToUpper(strings.TrimSpace(in.Method))
	if method == "" {
		method = http.MethodGet
	}
	path := in.Path
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(in.Body) > lim.RequestBytes {
		return FunctionInvokeResult{}, fmt.Errorf("body is larger than %d bytes", lim.RequestBytes)
	}
	target := base + path
	if _, err := url.ParseRequestURI(target); err != nil {
		return FunctionInvokeResult{}, fmt.Errorf("invalid path: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader([]byte(in.Body))) //nolint:gosec // G704: the operator calls their own function route on purpose
	if err != nil {
		return FunctionInvokeResult{}, fmt.Errorf("build request: %w", err)
	}
	sent := http.Header{}
	for k, v := range in.Headers {
		if invokeSkippedHeaders[strings.ToLower(k)] {
			continue
		}
		req.Header.Set(k, v)
		sent.Set(k, v)
	}
	id := ids.MustHex(8)
	req.Header.Set("X-Request-Id", id)

	cold := false
	client := &http.Client{
		Timeout: lim.Timeout,
		// The platform's own route may still carry the default self-signed certificate.
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // calls the platform's own route
		CheckRedirect: func(next *http.Request, via []*http.Request) error {
			if len(via) > 1 || next.URL.String() != target {
				return http.ErrUseLastResponse
			}
			cold = true
			return nil
		},
	}
	start := time.Now()
	resp, err := client.Do(req) //nolint:gosec // G704: the operator calls their own function route on purpose
	if err != nil {
		return FunctionInvokeResult{}, fmt.Errorf("call function: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(lim.ResponseBytes)+1))
	if err != nil {
		return FunctionInvokeResult{}, fmt.Errorf("read response: %w", err)
	}
	elapsed := time.Since(start)
	res := FunctionInvokeResult{
		RequestID: id, Status: resp.StatusCode, Headers: resp.Header, DurationMs: float64(elapsed) / float64(time.Millisecond), ColdStart: cold,
	}
	if len(raw) > lim.ResponseBytes {
		raw, res.BodyTruncated = raw[:lim.ResponseBytes], true
	}
	res.Body = strings.ToValidUTF8(string(raw), "�")
	if rt.captureEnabled(ctx, name) {
		inputs.Put(id, functions.Input{Function: name, Method: method, Path: path, Headers: sent, Body: in.Body, At: start})
		res.Stored = true
	}
	return res, nil
}

func (rt *Router) invokeTarget(w http.ResponseWriter, r *http.Request) (name, base string, ok bool) {
	name = r.PathValue("name")
	base, err := rt.functionBaseURL(r.Context(), name)
	switch {
	case err != nil:
		rt.internalError(w, "api: invoke function: resolve url failed", err, slog.String("function", name))
	case base == "":
		writeError(w, http.StatusNotFound, "function not found or it has no URL yet")
	default:
		return name, base, true
	}
	return "", "", false
}

// handleInvokeFunction handles POST /api/v1/functions/{name}/invoke: a test call through the real route, wake included.
func (rt *Router) handleInvokeFunction(w http.ResponseWriter, r *http.Request) {
	name, base, ok := rt.invokeTarget(w, r)
	if !ok {
		return
	}
	var in FunctionInvokeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "body must be a JSON invoke request")
		return
	}
	rt.respondInvoke(w, r, name, base, in)
}

// handleReplayFunctionInvocation handles POST /api/v1/functions/{name}/invocations/{id}/replay.
func (rt *Router) handleReplayFunctionInvocation(w http.ResponseWriter, r *http.Request) {
	name, base, ok := rt.invokeTarget(w, r)
	if !ok {
		return
	}
	inputs, _ := rt.functionInputStore()
	stored, found := inputs.Get(r.PathValue("id"))
	if !found || stored.Function != name {
		writeError(w, http.StatusNotFound, "no stored input for this invocation")
		return
	}
	headers := map[string]string{}
	for k, v := range stored.Headers {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	rt.respondInvoke(w, r, name, base, FunctionInvokeRequest{Method: stored.Method, Path: stored.Path, Headers: headers, Body: stored.Body})
}

func (rt *Router) respondInvoke(w http.ResponseWriter, r *http.Request, name, base string, in FunctionInvokeRequest) {
	res, err := rt.runFunctionInvoke(r.Context(), name, base, in)
	if err != nil {
		rt.logger.Warn("api: invoke function failed", slog.String("function", name), slog.String("error", err.Error()))
		var ue *url.Error
		if errors.As(err, &ue) {
			writeError(w, http.StatusBadGateway, "the function did not answer: "+ue.Err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, res)
}

type functionCaptureBody struct {
	Enabled bool `json:"enabled"`
}

// handleGetFunctionCapture handles GET /api/v1/functions/{name}/capture.
func (rt *Router) handleGetFunctionCapture(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, functionCaptureBody{Enabled: rt.captureEnabled(r.Context(), r.PathValue("name"))})
}

// handleSetFunctionCapture handles PUT /api/v1/functions/{name}/capture: opt a function in or out of replay storage.
func (rt *Router) handleSetFunctionCapture(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	cs, ok := rt.apps.(functionCaptureStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "input capture is not available")
		return
	}
	var body functionCaptureBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "body must be {\"enabled\": true|false}")
		return
	}
	if err := cs.SetFunctionCapture(r.Context(), name, body.Enabled); err != nil {
		rt.internalError(w, "api: set function capture failed", err, slog.String("function", name))
		return
	}
	if !body.Enabled {
		inputs, _ := rt.functionInputStore()
		inputs.Forget(name)
	}
	writeJSON(w, http.StatusOK, body)
}

// FunctionInvocation is one recorded call at the edge.
type FunctionInvocation struct {
	ID         string    `json:"id"`
	Time       time.Time `json:"time"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Status     int       `json:"status"`
	DurationMs float64   `json:"duration_ms"`
	ColdStart  bool      `json:"cold_start"`
	BytesIn    int64     `json:"bytes_in"`
	BytesOut   int64     `json:"bytes_out"`
	TraceID    string    `json:"trace_id,omitempty"`
	Replayable bool      `json:"replayable"`
	LogsFrom   time.Time `json:"logs_from"`
	LogsTo     time.Time `json:"logs_to"`
}

// mergeColdRecords folds the wake hook's held 307 into the replayed call that shares its request id.
func mergeColdRecords(recs []ingress.AccessRecord) []ingress.AccessRecord {
	cold := map[string]ingress.AccessRecord{}
	for _, rec := range recs {
		if rec.Cold && rec.RequestID != "" {
			cold[rec.RequestID] = rec
		}
	}
	out := make([]ingress.AccessRecord, 0, len(recs))
	for _, rec := range recs {
		if c, ok := cold[rec.RequestID]; ok {
			if rec.Cold {
				continue
			}
			rec.Cold, rec.Time, rec.Millis = true, c.Time, rec.Millis+c.Millis
		}
		out = append(out, rec)
	}
	return out
}

// handleListFunctionInvocations handles GET /api/v1/functions/{name}/invocations: recent edge calls, newest first.
func (rt *Router) handleListFunctionInvocations(w http.ResponseWriter, r *http.Request) {
	if rt.access == nil {
		writeError(w, http.StatusNotImplemented, "edge access log is not configured on this control plane")
		return
	}
	name := r.PathValue("name")
	var f ingress.AccessFilter
	var err error
	if f.Since, err = parseOptionalTime(r.URL.Query().Get("since")); err != nil {
		writeError(w, http.StatusBadRequest, "since must be RFC3339")
		return
	}
	f.Limit = invocationLimit(r.URL.Query().Get("limit"))
	inputs, lim := rt.functionInputStore()
	recs := mergeColdRecords(rt.access.AppAccess(name, f))
	out := make([]FunctionInvocation, 0, len(recs))
	for _, rec := range recs {
		_, stored := inputs.Get(rec.RequestID)
		dur := time.Duration(rec.Millis * float64(time.Millisecond))
		out = append(out, FunctionInvocation{
			ID: rec.RequestID, Time: rec.Time, Method: rec.Method, Path: rec.Path, Status: rec.Status, DurationMs: rec.Millis,
			ColdStart: rec.Cold, BytesIn: rec.BytesIn, BytesOut: rec.BytesOut, TraceID: rec.TraceID, Replayable: stored,
			LogsFrom: rec.Time.Add(-lim.LogPad), LogsTo: rec.Time.Add(dur + lim.LogPad),
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"invocations": out, "capture": rt.captureEnabled(r.Context(), name)})
}

func invocationLimit(v string) int {
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil || n < 1 {
		return 0
	}
	return n
}
