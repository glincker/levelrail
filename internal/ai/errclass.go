package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ErrorClass names why a provider call failed, in terms an operator can act on.
type ErrorClass string

// Error classes a provider call can end in.
const (
	ClassInvalidKey      ErrorClass = "invalid_key"
	ClassQuotaExhausted  ErrorClass = "quota_exhausted"
	ClassRateLimited     ErrorClass = "rate_limited"
	ClassOverloaded      ErrorClass = "overloaded"
	ClassContextTooLong  ErrorClass = "context_too_long"
	ClassModelNotFound   ErrorClass = "model_not_found"
	ClassNetwork         ErrorClass = "network"
	ClassContentRefused  ErrorClass = "content_refused"
	ClassStreamCut       ErrorClass = "stream_interrupted"
	ClassBadRequest      ErrorClass = "bad_request"
	ClassBlockedAddress  ErrorClass = "blocked_address"
	ClassBudgetExhausted ErrorClass = "budget_exhausted"
)

// ProviderError is a classified provider failure. It never carries the API key.
type ProviderError struct {
	Provider   string
	Class      ErrorClass
	Status     int
	RetryAfter time.Duration
	Detail     string
}

func (e *ProviderError) Error() string {
	msg, _ := e.Explain()
	if e.Detail != "" {
		return fmt.Sprintf("ai: %s: %s (%s)", e.Provider, msg, e.Detail)
	}
	return fmt.Sprintf("ai: %s: %s", e.Provider, msg)
}

// Explain returns a plain-language cause and a suggested fix for the UI.
func (e *ProviderError) Explain() (message, fix string) {
	switch e.Class {
	case ClassInvalidKey:
		return "The provider rejected the API key.", "Open Settings, AI assistant and paste a valid key for this provider."
	case ClassQuotaExhausted:
		return "The provider account is out of credit or quota.", "Add credit or raise the spend limit at the provider, or switch to another provider in the fallback chain."
	case ClassRateLimited:
		return "The provider is rate limiting requests.", "Wait a moment and retry, or add a fallback provider."
	case ClassOverloaded:
		return "The provider is overloaded or temporarily down.", "Retry shortly, or add a fallback provider."
	case ClassContextTooLong:
		return "The conversation is longer than the model can read.", "Start a new chat or pick a model with a larger context window."
	case ClassModelNotFound:
		return "The provider does not have this model.", "Pick a model from the provider's list in Settings, AI assistant."
	case ClassNetwork:
		return "The provider could not be reached.", "Check the control plane's outbound network and the provider base URL."
	case ClassContentRefused:
		return "The model declined to answer this request.", "Rephrase the request."
	case ClassStreamCut:
		return "The provider connection dropped mid-answer.", "Send the message again."
	case ClassBlockedAddress:
		return "The provider address is not allowed.", "Use a public https endpoint, or set a private endpoint as a root user for an OpenAI compatible server."
	case ClassBudgetExhausted:
		return "The monthly AI token budget is used up.", "Raise APP_AI_MONTHLY_TOKEN_BUDGET or wait for the next month."
	default:
		return "The provider returned an error.", "Check the model id and provider settings."
	}
}

// Retryable reports whether repeating the same call may succeed.
func (e *ProviderError) Retryable() bool {
	switch e.Class {
	case ClassRateLimited, ClassOverloaded, ClassNetwork:
		return true
	default:
		return false
	}
}

// FallbackEligible reports whether another provider may serve the call:
// only failures that do not depend on this request's key or content.
func (e *ProviderError) FallbackEligible() bool {
	switch e.Class {
	case ClassRateLimited, ClassOverloaded, ClassNetwork, ClassQuotaExhausted:
		return true
	default:
		return false
	}
}

var keyLike = regexp.MustCompile(`(?i)\b(?:sk|gsk|xai|pk|key|AIza)[-_A-Za-z0-9]{8,}`)

// Scrub removes the key and key-shaped tokens from provider-supplied text.
func Scrub(s, key string) string {
	if key != "" {
		s = strings.ReplaceAll(s, key, "[redacted]")
	}
	return keyLike.ReplaceAllString(s, "[redacted]")
}

func parseRetryAfter(h http.Header, now time.Time) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if secs, err := strconv.ParseFloat(v, 64); err == nil && secs >= 0 {
		return time.Duration(secs * float64(time.Second))
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

func errorMessageFromBody(body []byte) string {
	var env struct {
		Error json.RawMessage `json:"error"`
		Msg   string          `json:"message"`
	}
	if json.Unmarshal(body, &env) != nil {
		return ""
	}
	if len(env.Error) > 0 {
		var obj struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		}
		if json.Unmarshal(env.Error, &obj) == nil && (obj.Message != "" || obj.Type != "") {
			return strings.TrimSpace(obj.Type + " " + obj.Message)
		}
		var s string
		if json.Unmarshal(env.Error, &s) == nil {
			return s
		}
	}
	return env.Msg
}

// ClassifyHTTP maps a non-2xx provider response to a ProviderError.
func ClassifyHTTP(provider string, status int, body []byte, h http.Header, key string, now time.Time) *ProviderError {
	msg := errorMessageFromBody(body)
	low := strings.ToLower(msg + " " + string(body))
	class := ClassBadRequest
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden || strings.Contains(low, "api key not valid") || strings.Contains(low, "incorrect api key") || strings.Contains(low, "invalid x-api-key") || strings.Contains(low, "invalid api key"):
		class = ClassInvalidKey
	case status == http.StatusPaymentRequired || containsAny(low, "insufficient_quota", "credit balance", "exceeded your current quota", "billing", "out of credits"):
		class = ClassQuotaExhausted
	case containsAny(low, "context length", "context_length", "maximum context", "prompt is too long", "too many tokens", "token limit", "context window"):
		class = ClassContextTooLong
	case (status == http.StatusNotFound || status == http.StatusBadRequest) && strings.Contains(low, "model") && containsAny(low, "not found", "does not exist", "not_found", "unknown model", "not supported"):
		class = ClassModelNotFound
	case status == http.StatusNotFound && !strings.Contains(low, "model"):
		class = ClassModelNotFound
	case status == http.StatusTooManyRequests:
		class = ClassRateLimited
	case status == 529 || strings.Contains(low, "overloaded") || status >= 500:
		class = ClassOverloaded
	}
	d := Scrub(msg, key)
	if len(d) > 300 {
		d = d[:300]
	}
	return &ProviderError{Provider: provider, Class: class, Status: status, RetryAfter: parseRetryAfter(h, now), Detail: d}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ClassifyTransport maps a transport failure (dial, TLS, timeout) to a ProviderError.
func ClassifyTransport(provider string, err error, key string) *ProviderError {
	pe := &ProviderError{Provider: provider, Class: ClassNetwork}
	if errors.Is(err, errBlockedAddress) {
		pe.Class = ClassBlockedAddress
	}
	var ne net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		pe.Detail = "timeout"
	case errors.As(err, &ne) && ne.Timeout():
		pe.Detail = "timeout"
	default:
		pe.Detail = Scrub(err.Error(), key)
		if len(pe.Detail) > 200 {
			pe.Detail = pe.Detail[:200]
		}
	}
	return pe
}

// AsProviderError unwraps a ProviderError from err.
func AsProviderError(err error) (*ProviderError, bool) {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe, true
	}
	return nil, false
}
