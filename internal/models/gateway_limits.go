package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// Environment variables that tune the gateway. A zero or negative value
// disables the matching limit.
const (
	envGatewayMaxBody       = "APP_MODEL_GATEWAY_MAX_BODY_BYTES"
	envGatewayMaxN          = "APP_MODEL_GATEWAY_MAX_N"
	envGatewayMaxGenLen     = "APP_MODEL_GATEWAY_MAX_TOKENS"
	envGatewayMaxInflight   = "APP_MODEL_GATEWAY_MAX_INFLIGHT"
	envGatewayRetryAfter    = "APP_MODEL_GATEWAY_RETRY_AFTER"
	envGatewayDialTimeout   = "APP_MODEL_GATEWAY_DIAL_TIMEOUT"
	envGatewayHeaderTimeout = "APP_MODEL_GATEWAY_HEADER_TIMEOUT"
	envGatewayIdleTimeout   = "APP_MODEL_GATEWAY_IDLE_TIMEOUT"
)

// GatewayLimits bounds what one request may cost the engine. The limits are
// global: they apply to every model.
type GatewayLimits struct {
	MaxBodyBytes  int64
	MaxN          int
	MaxTokens     int
	MaxInflight   int
	RetryAfter    time.Duration
	DialTimeout   time.Duration
	HeaderTimeout time.Duration
	IdleTimeout   time.Duration
}

// DefaultGatewayLimits are the limits used when no environment override is set.
func DefaultGatewayLimits() GatewayLimits {
	return GatewayLimits{
		MaxBodyBytes:  32 << 20,
		MaxN:          16,
		MaxTokens:     32768,
		MaxInflight:   32,
		RetryAfter:    5 * time.Second,
		DialTimeout:   5 * time.Second,
		HeaderTimeout: 5 * time.Minute,
		IdleTimeout:   2 * time.Minute,
	}
}

// LoadGatewayLimits reads the limits from the environment, falling back to
// the defaults for unset or unparsable values.
func LoadGatewayLimits() GatewayLimits {
	l := DefaultGatewayLimits()
	l.MaxBodyBytes = envInt64(envGatewayMaxBody, l.MaxBodyBytes)
	l.MaxN = int(envInt64(envGatewayMaxN, int64(l.MaxN)))
	l.MaxTokens = int(envInt64(envGatewayMaxGenLen, int64(l.MaxTokens)))
	l.MaxInflight = int(envInt64(envGatewayMaxInflight, int64(l.MaxInflight)))
	l.RetryAfter = envDuration(envGatewayRetryAfter, l.RetryAfter)
	l.DialTimeout = envDuration(envGatewayDialTimeout, l.DialTimeout)
	l.HeaderTimeout = envDuration(envGatewayHeaderTimeout, l.HeaderTimeout)
	l.IdleTimeout = envDuration(envGatewayIdleTimeout, l.IdleTimeout)
	return l
}

// GatewayLimitsSummary is the read-only view of the limits shown on the
// model resource. A zero value means the limit is disabled.
type GatewayLimitsSummary struct {
	MaxBodyBytes           int64 `json:"max_body_bytes"`
	MaxN                   int   `json:"max_n"`
	MaxTokens              int   `json:"max_tokens"`
	MaxInflight            int   `json:"max_inflight_requests"`
	ResponseHeaderTimeoutS int   `json:"response_header_timeout_seconds"`
	StreamIdleTimeoutS     int   `json:"stream_idle_timeout_seconds"`
}

// Summary returns the limits as shown to operators.
func (l GatewayLimits) Summary() GatewayLimitsSummary {
	return GatewayLimitsSummary{
		MaxBodyBytes: max(l.MaxBodyBytes, 0), MaxN: max(l.MaxN, 0), MaxTokens: max(l.MaxTokens, 0),
		MaxInflight:            max(l.MaxInflight, 0),
		ResponseHeaderTimeoutS: max(int(l.HeaderTimeout/time.Second), 0),
		StreamIdleTimeoutS:     max(int(l.IdleTimeout/time.Second), 0),
	}
}

func envInt64(name string, def int64) int64 {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func envDuration(name string, def time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

// requestError is a client error with the HTTP status and OpenAI error code
// to report.
type requestError struct {
	status int
	code   string
	msg    string
}

func (e *requestError) Error() string { return e.msg }

// tokenFields are the request fields that bound generated length.
var tokenFields = []string{"max_tokens", "max_completion_tokens", "max_output_tokens"}

// countFields are the request fields that multiply generation work.
var countFields = []string{"n", "best_of"}

// prepareBody enforces the body size cap on r and, for JSON routes, the
// n and token caps. The body is read up to the cap and re-attached so the
// proxy can forward it.
func (l GatewayLimits) prepareBody(w http.ResponseWriter, r *http.Request, jsonBody bool) *requestError {
	_, rerr := l.prepareBodyInfo(w, r, jsonBody)
	return rerr
}

// bodyInfo is what prepareBodyInfo learned from a request body.
type bodyInfo struct {
	model   string
	hasJSON bool
}

// prepareBodyInfo is prepareBody that also reports the request's "model"
// field. hasJSON is false when the request has no body.
func (l GatewayLimits) prepareBodyInfo(w http.ResponseWriter, r *http.Request, jsonBody bool) (bodyInfo, *requestError) {
	var info bodyInfo
	if r.Body == nil || r.Body == http.NoBody {
		return info, nil
	}
	if l.MaxBodyBytes > 0 && r.ContentLength > l.MaxBodyBytes {
		return info, tooLarge(l.MaxBodyBytes)
	}
	if l.MaxBodyBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, l.MaxBodyBytes)
	}
	if !jsonBody {
		info.hasJSON = true
		return info, nil
	}
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return info, tooLarge(l.MaxBodyBytes)
		}
		return info, &requestError{http.StatusBadRequest, "invalid_request_error", "could not read the request body"}
	}
	r.Body = io.NopCloser(bytes.NewReader(buf))
	r.ContentLength = int64(len(buf))
	if len(bytes.TrimSpace(buf)) == 0 {
		return info, nil
	}
	info.hasJSON = true
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(buf, &fields); err != nil {
		return info, &requestError{http.StatusBadRequest, "invalid_request_error", "request body must be a JSON object"}
	}
	if raw, ok := fields["model"]; ok {
		_ = json.Unmarshal(raw, &info.model)
	}
	return info, l.checkFields(fields)
}

func tooLarge(limit int64) *requestError {
	return &requestError{http.StatusRequestEntityTooLarge, "request_too_large", fmt.Sprintf("request body exceeds the %d byte limit", limit)}
}

func (l GatewayLimits) checkFields(fields map[string]json.RawMessage) *requestError {
	if l.MaxN > 0 {
		for _, f := range countFields {
			if v, ok := numberField(fields[f]); ok && v > float64(l.MaxN) {
				return &requestError{http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("%s must be at most %d", f, l.MaxN)}
			}
		}
	}
	if l.MaxTokens > 0 {
		for _, f := range tokenFields {
			v, ok := numberField(fields[f])
			if !ok {
				continue
			}
			if v > float64(l.MaxTokens) {
				return &requestError{http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("%s must be at most %d", f, l.MaxTokens)}
			}
			if v < 0 {
				// llama.cpp treats a negative limit as unlimited.
				return &requestError{http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("%s must not be negative", f)}
			}
		}
	}
	return nil
}

// numberField decodes raw as a JSON number. Values that are absent, null or
// not numbers are left for the engine to reject.
func numberField(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var v float64
	err := json.Unmarshal(raw, &v)
	if err == nil {
		return v, true
	}
	trimmed := bytes.TrimSpace(raw)
	if trimmed[0] != '-' && (trimmed[0] < '0' || trimmed[0] > '9') {
		return 0, false
	}
	// A number outside float64 range is as large as a number can be.
	if trimmed[0] == '-' {
		return -1, true
	}
	return 1e308, true
}

// inflight caps concurrent requests per model with a counting semaphore.
type inflight struct {
	mu   sync.Mutex
	sems map[string]chan struct{}
}

func (f *inflight) acquire(model string, limit int) (release func(), ok bool) {
	if limit <= 0 {
		return func() {}, true
	}
	f.mu.Lock()
	if f.sems == nil {
		f.sems = map[string]chan struct{}{}
	}
	sem, exists := f.sems[model]
	if !exists || cap(sem) != limit {
		sem = make(chan struct{}, limit)
		f.sems[model] = sem
	}
	f.mu.Unlock()
	select {
	case sem <- struct{}{}:
		return func() { <-sem }, true
	default:
		return nil, false
	}
}
