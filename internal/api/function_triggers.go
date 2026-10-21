package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/functions"
	"github.com/GLINCKER/levelrail/internal/store"
)

// FunctionTriggerStore persists webhook triggers and the async invocation queue.
type FunctionTriggerStore interface {
	SetFunctionWebhook(ctx context.Context, fn, path string, rotated bool, now time.Time) error
	GetFunctionWebhook(ctx context.Context, fn string) (store.FunctionWebhook, error)
	DeleteFunctionWebhook(ctx context.Context, fn string) error
	EnqueueFunctionInvocation(ctx context.Context, inv store.FunctionInvocation, maxQueued int) error
	GetFunctionInvocation(ctx context.Context, id string) (store.FunctionInvocation, error)
	ListFunctionInvocations(ctx context.Context, fn, status string, limit int) ([]store.FunctionInvocation, error)
	CountFunctionInvocations(ctx context.Context, fn string) (map[string]int, error)
	RetryFunctionInvocation(ctx context.Context, id string, now time.Time) error
	DeleteFunctionInvocation(ctx context.Context, id string) error
}

const (
	functionWebhookURLPrefix  = "/api/v1/webhooks/functions/"
	functionWebhookSecretNS   = "function-webhook/"
	functionWebhookSecretKey  = "secret"
	functionInvocationListMax = 200
	maxFunctionPathLen        = 2048
)

type functionWebhookView struct {
	URL                 string     `json:"url"`
	Path                string     `json:"path"`
	ReplayWindowSeconds int        `json:"replay_window_seconds"`
	CreatedAt           time.Time  `json:"created_at"`
	RotatedAt           *time.Time `json:"rotated_at,omitempty"`
	Secret              string     `json:"secret,omitempty"`
}

type functionScheduleView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Timezone string `json:"timezone,omitempty"`
	Enabled  bool   `json:"enabled"`
}

type functionTriggersResponse struct {
	Function  string                 `json:"function"`
	Webhook   *functionWebhookView   `json:"webhook"`
	Schedules []functionScheduleView `json:"schedules"`
	Queue     map[string]int         `json:"queue"`
}

type functionInvocationView struct {
	ID             string    `json:"id"`
	Function       string    `json:"function"`
	Source         string    `json:"source"`
	Method         string    `json:"method"`
	Path           string    `json:"path"`
	BodyBytes      int       `json:"body_bytes"`
	Status         string    `json:"status"`
	Attempts       int       `json:"attempts"`
	MaxAttempts    int       `json:"max_attempts"`
	NextAttemptAt  time.Time `json:"next_attempt_at"`
	LastStatusCode int       `json:"last_status_code,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func invocationView(inv store.FunctionInvocation) functionInvocationView {
	return functionInvocationView{
		ID: inv.ID, Function: inv.Function, Source: inv.Source, Method: inv.Method, Path: inv.Path,
		BodyBytes: len(inv.Body), Status: inv.Status, Attempts: inv.Attempts, MaxAttempts: inv.MaxAttempts,
		NextAttemptAt: inv.NextAttemptAt, LastStatusCode: inv.LastStatusCode, LastError: inv.LastError,
		CreatedAt: inv.CreatedAt, UpdatedAt: inv.UpdatedAt,
	}
}

func (rt *Router) webhookView(w store.FunctionWebhook) *functionWebhookView {
	return &functionWebhookView{
		URL: functionWebhookURLPrefix + w.Function, Path: w.Path, ReplayWindowSeconds: int(rt.functionCfg.ReplayWindow.Seconds()),
		CreatedAt: w.CreatedAt, RotatedAt: w.RotatedAt,
	}
}

func validFunctionPath(p string) bool {
	return strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") && len(p) <= maxFunctionPathLen && !strings.ContainsAny(p, " \r\n")
}

func (rt *Router) requireFunction(w http.ResponseWriter, r *http.Request) (string, bool) {
	name := r.PathValue("name")
	if rt.functionTriggers == nil {
		writeError(w, http.StatusNotImplemented, "function triggers are not available on this control plane")
		return "", false
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		writeError(w, http.StatusNotFound, "function not found")
		return "", false
	}
	return name, true
}

// handleGetFunctionTriggers handles GET /api/v1/functions/{name}/triggers.
func (rt *Router) handleGetFunctionTriggers(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.requireFunction(w, r)
	if !ok {
		return
	}
	out := functionTriggersResponse{Function: name, Schedules: []functionScheduleView{}}
	wh, err := rt.functionTriggers.GetFunctionWebhook(r.Context(), name)
	switch {
	case err == nil:
		out.Webhook = rt.webhookView(wh)
	case !errors.Is(err, store.ErrFunctionWebhookNotFound):
		rt.internalError(w, "api: get function triggers: load webhook failed", err, slog.String("function", name))
		return
	}
	tasks, err := rt.scheduledTasks.ListScheduledTasksForService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get function triggers: list schedules failed", err, slog.String("function", name))
		return
	}
	for _, t := range tasks {
		if t.Mode == store.ScheduledTaskModeHTTP {
			out.Schedules = append(out.Schedules, functionScheduleView{ID: t.ID, Name: t.Name, Schedule: t.Schedule, Timezone: t.Timezone, Enabled: t.Enabled})
		}
	}
	if out.Queue, err = rt.functionTriggers.CountFunctionInvocations(r.Context(), name); err != nil {
		rt.internalError(w, "api: get function triggers: count queue failed", err, slog.String("function", name))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type functionWebhookRequest struct {
	Path   string `json:"path"`
	Rotate bool   `json:"rotate"`
}

// handlePutFunctionWebhook handles PUT /api/v1/functions/{name}/triggers/webhook: enable, retarget or rotate.
// The secret is returned only when one was generated.
func (rt *Router) handlePutFunctionWebhook(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.requireFunction(w, r)
	if !ok {
		return
	}
	if rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "webhook triggers need a master key on this control plane")
		return
	}
	var req functionWebhookRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPolicyBody)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	existing, err := rt.functionTriggers.GetFunctionWebhook(r.Context(), name)
	found := err == nil
	if err != nil && !errors.Is(err, store.ErrFunctionWebhookNotFound) {
		rt.internalError(w, "api: put function webhook: load failed", err, slog.String("function", name))
		return
	}
	path := req.Path
	if path == "" {
		path = existing.Path
	}
	if path == "" {
		path = "/"
	}
	if !validFunctionPath(path) {
		writeError(w, http.StatusBadRequest, "path must start with / and name a path on the function")
		return
	}
	secretKey := functionWebhookSecretNS + name
	hasSecret, err := rt.gitSourceSecrets.Exists(r.Context(), secretKey, functionWebhookSecretKey)
	if err != nil {
		rt.internalError(w, "api: put function webhook: check secret failed", err, slog.String("function", name))
		return
	}
	secret := ""
	if req.Rotate || !found || !hasSecret {
		if secret, err = functions.NewSecret(); err != nil {
			rt.internalError(w, "api: put function webhook: generate secret failed", err, slog.String("function", name))
			return
		}
		if err = rt.gitSourceSecrets.SetValue(r.Context(), secretKey, functionWebhookSecretKey, secret); err != nil {
			rt.internalError(w, "api: put function webhook: store secret failed", err, slog.String("function", name))
			return
		}
	}
	if err = rt.functionTriggers.SetFunctionWebhook(r.Context(), name, path, secret != "" && found, time.Now()); err != nil {
		rt.internalError(w, "api: put function webhook: save failed", err, slog.String("function", name))
		return
	}
	saved, err := rt.functionTriggers.GetFunctionWebhook(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: put function webhook: reload failed", err, slog.String("function", name))
		return
	}
	view := rt.webhookView(saved)
	view.Secret = secret
	writeJSON(w, http.StatusOK, view)
}

// handleDeleteFunctionWebhook handles DELETE /api/v1/functions/{name}/triggers/webhook.
func (rt *Router) handleDeleteFunctionWebhook(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.requireFunction(w, r)
	if !ok {
		return
	}
	if err := rt.functionTriggers.DeleteFunctionWebhook(r.Context(), name); err != nil {
		rt.internalError(w, "api: delete function webhook failed", err, slog.String("function", name))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type enqueueFunctionRequest struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	Body        string `json:"body"`
	ContentType string `json:"content_type"`
}

func (rt *Router) enqueueFunction(ctx context.Context, name, source, method, path, contentType string, body []byte, dedupe string) (store.FunctionInvocation, error) {
	id, err := functions.NewInvocationID()
	if err != nil {
		return store.FunctionInvocation{}, err
	}
	now := time.Now()
	inv := store.FunctionInvocation{
		ID: id, Function: name, Source: source, Method: method, Path: path, ContentType: contentType, Body: body,
		DedupeKey: dedupe, MaxAttempts: rt.functionCfg.MaxAttempts, NextAttemptAt: now, CreatedAt: now,
	}
	if err := rt.functionTriggers.EnqueueFunctionInvocation(ctx, inv, rt.functionCfg.MaxQueued); err != nil {
		return store.FunctionInvocation{}, err
	}
	if rt.functionKick != nil {
		rt.functionKick()
	}
	return inv, nil
}

// handleEnqueueFunctionInvocation handles POST /api/v1/functions/{name}/invocations: return 202 now, deliver with retries.
func (rt *Router) handleEnqueueFunctionInvocation(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.requireFunction(w, r)
	if !ok {
		return
	}
	var req enqueueFunctionRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, rt.functionCfg.MaxBodyBytes+maxPolicyBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	method := strings.ToUpper(req.Method)
	if method == "" {
		method = http.MethodPost
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		writeError(w, http.StatusBadRequest, "method must be GET, POST, PUT, PATCH or DELETE")
		return
	}
	path := req.Path
	if path == "" {
		path = "/"
	}
	if !validFunctionPath(path) {
		writeError(w, http.StatusBadRequest, "path must start with / and name a path on the function")
		return
	}
	if int64(len(req.Body)) > rt.functionCfg.MaxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "body too large")
		return
	}
	inv, err := rt.enqueueFunction(r.Context(), name, "api", method, path, req.ContentType, []byte(req.Body), "")
	if rt.writeEnqueueError(w, name, err) {
		return
	}
	writeJSON(w, http.StatusAccepted, invocationView(inv))
}

func (rt *Router) writeEnqueueError(w http.ResponseWriter, name string, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, store.ErrFunctionQueueFull):
		w.Header().Set("Retry-After", strconv.Itoa(int(rt.functionCfg.PollInterval.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "function queue is full, try again later")
	default:
		rt.internalError(w, "api: enqueue function invocation failed", err, slog.String("function", name))
	}
	return true
}

// handleFunctionWebhook handles POST /api/v1/webhooks/functions/{name}. Unauthenticated by design: the HMAC signature
// over timestamp and body, checked inside the replay window, stands in for a session.
func (rt *Router) handleFunctionWebhook(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if rt.functionTriggers == nil || rt.gitSourceSecrets == nil {
		writeError(w, http.StatusNotImplemented, "function webhooks are not configured on this control plane")
		return
	}
	if rt.functionWebhookLimit != nil {
		if allowed, retryAfter := rt.functionWebhookLimit.allow(clientIP(r) + "|" + name); !allowed {
			writeRateLimited(w, retryAfter)
			return
		}
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, rt.functionCfg.MaxBodyBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "cannot read request body")
		return
	}
	if int64(len(body)) > rt.functionCfg.MaxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	wh, err := rt.functionTriggers.GetFunctionWebhook(r.Context(), name)
	if errors.Is(err, store.ErrFunctionWebhookNotFound) {
		writeError(w, http.StatusNotFound, "no webhook for this function")
		return
	}
	if err != nil {
		rt.internalError(w, "api: function webhook: load failed", err, slog.String("function", name))
		return
	}
	secret, err := rt.gitSourceSecrets.Resolve(r.Context(), functionWebhookSecretNS+name, functionWebhookSecretKey)
	if err != nil {
		rt.internalError(w, "api: function webhook: resolve secret failed", err, slog.String("function", name))
		return
	}
	sig, err := functions.Verify(secret, r.Header.Get(functions.HeaderTimestamp), r.Header.Get(functions.HeaderSignature), body, rt.functionCfg.ReplayWindow, time.Now())
	if err != nil {
		rt.logger.Warn("api: function webhook: rejected", slog.String("function", name), slog.String("reason", err.Error()), slog.String("remote_addr", r.RemoteAddr))
		writeError(w, http.StatusUnauthorized, "invalid or expired signature")
		return
	}
	inv, err := rt.enqueueFunction(r.Context(), name, "webhook", http.MethodPost, wh.Path, r.Header.Get("Content-Type"), body, sig)
	if errors.Is(err, store.ErrFunctionInvocationDuplicate) {
		writeJSON(w, http.StatusOK, map[string]any{"duplicate": true})
		return
	}
	if rt.writeEnqueueError(w, name, err) {
		return
	}
	writeJSON(w, http.StatusAccepted, invocationView(inv))
}

// handleListQueuedFunctionCalls handles GET /api/v1/function-invocations?function=&status=&limit=.
func (rt *Router) handleListQueuedFunctionCalls(w http.ResponseWriter, r *http.Request) {
	if rt.functionTriggers == nil {
		writeError(w, http.StatusNotImplemented, "function triggers are not available on this control plane")
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > functionInvocationListMax {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("limit must be 1 to %d", functionInvocationListMax))
			return
		}
		limit = n
	}
	status := r.URL.Query().Get("status")
	switch status {
	case "", store.FunctionInvocationQueued, store.FunctionInvocationRunning, store.FunctionInvocationDone, store.FunctionInvocationDead:
	default:
		writeError(w, http.StatusBadRequest, "status must be queued, running, done or dead")
		return
	}
	canRead, filtered, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: list function invocations: resolve caller failed", err)
		return
	}
	rows, err := rt.functionTriggers.ListFunctionInvocations(r.Context(), r.URL.Query().Get("function"), status, limit)
	if err != nil {
		rt.internalError(w, "api: list function invocations failed", err)
		return
	}
	out := []functionInvocationView{}
	for _, inv := range rows {
		if !filtered || canRead(inv.Function) {
			out = append(out, invocationView(inv))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"invocations": out})
}

func (rt *Router) invocationForCaller(w http.ResponseWriter, r *http.Request) (store.FunctionInvocation, bool) {
	if rt.functionTriggers == nil {
		writeError(w, http.StatusNotImplemented, "function triggers are not available on this control plane")
		return store.FunctionInvocation{}, false
	}
	id := r.PathValue("id")
	inv, err := rt.functionTriggers.GetFunctionInvocation(r.Context(), id)
	if errors.Is(err, store.ErrFunctionInvocationNotFound) {
		writeError(w, http.StatusNotFound, "invocation not found")
		return inv, false
	}
	if err != nil {
		rt.internalError(w, "api: load function invocation failed", err, slog.String("invocation_id", id))
		return inv, false
	}
	canRead, filtered, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: function invocation: resolve caller failed", err, slog.String("invocation_id", id))
		return inv, false
	}
	if filtered && !canRead(inv.Function) {
		writeError(w, http.StatusNotFound, "invocation not found")
		return inv, false
	}
	return inv, true
}

// handleRetryFunctionInvocation handles POST /api/v1/function-invocations/{id}/retry for a dead-lettered call.
func (rt *Router) handleRetryFunctionInvocation(w http.ResponseWriter, r *http.Request) {
	inv, ok := rt.invocationForCaller(w, r)
	if !ok {
		return
	}
	if err := rt.functionTriggers.RetryFunctionInvocation(r.Context(), inv.ID, time.Now()); err != nil {
		if errors.Is(err, store.ErrFunctionInvocationNotDead) {
			writeError(w, http.StatusConflict, "only a dead-lettered invocation can be retried")
			return
		}
		rt.internalError(w, "api: retry function invocation failed", err, slog.String("invocation_id", inv.ID), slog.String("function", inv.Function))
		return
	}
	if rt.functionKick != nil {
		rt.functionKick()
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": inv.ID, "status": store.FunctionInvocationQueued})
}

// handleDeleteFunctionInvocation handles DELETE /api/v1/function-invocations/{id}: discard a dead letter.
func (rt *Router) handleDeleteFunctionInvocation(w http.ResponseWriter, r *http.Request) {
	inv, ok := rt.invocationForCaller(w, r)
	if !ok {
		return
	}
	if err := rt.functionTriggers.DeleteFunctionInvocation(r.Context(), inv.ID); err != nil {
		rt.internalError(w, "api: delete function invocation failed", err, slog.String("invocation_id", inv.ID), slog.String("function", inv.Function))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
