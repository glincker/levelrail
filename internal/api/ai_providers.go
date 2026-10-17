package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AIVault is the internal/ai.Vault surface the provider and settings routes need.
// nil is valid: those routes return 501.
type AIVault interface {
	List(ctx context.Context) ([]ai.ProviderView, error)
	Set(ctx context.Context, id string, in ai.SetInput, privateAllowed bool) (ai.ProviderView, ai.SetResult, error)
	Test(ctx context.Context, id string) (ai.TestResult, error)
	Remove(ctx context.Context, id string) error
	Models(ctx context.Context, id string) ([]ai.ModelInfo, error)
	Get(ctx context.Context, id string) (ai.ProviderView, error)
	Settings(ctx context.Context) (ai.AssistantSettings, error)
	SetChoice(ctx context.Context, active *store.AIModelRef, fallback *[]store.AIModelRef) error
}

// AIUsageStore is the metering surface the usage endpoint and budget display need.
type AIUsageStore interface {
	AIUsageByDay(ctx context.Context, since time.Time) ([]store.AIUsageDay, error)
	SumAITokensSince(ctx context.Context, since time.Time) (int64, error)
}

const (
	aiAuditPrivateBaseURL = "ai_assistant.private_base_url"
	aiProviderBodyLimit   = 64 << 10
	aiUsageMaxDays        = 365
	aiUsageDefaultDays    = 30
)

type aiErrorBody struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
	Fix   string `json:"fix,omitempty"`
}

func writeAIError(w http.ResponseWriter, status int, msg, code, fix string) {
	writeJSON(w, status, aiErrorBody{Error: msg, Code: code, Fix: fix})
}

// writeAIVaultError maps a Vault error to a typed response. Provider errors
// carry only classified, key-free text.
func (rt *Router) writeAIVaultError(w http.ResponseWriter, context string, err error, providerID string) {
	var se *ai.SaveError
	if errors.As(err, &se) {
		writeAIError(w, http.StatusUnprocessableEntity, se.Message, se.Code, se.Fix)
		return
	}
	if errors.Is(err, ai.ErrUnknownProvider) {
		writeError(w, http.StatusNotFound, "provider not found")
		return
	}
	if pe, ok := ai.AsProviderError(err); ok {
		msg, fix := pe.Explain()
		writeAIError(w, http.StatusBadGateway, msg, string(pe.Class), fix)
		return
	}
	rt.internalError(w, context, err, slog.String("provider", providerID))
}

func (rt *Router) requireAIVault(w http.ResponseWriter) bool {
	if rt.aiVault == nil {
		writeError(w, http.StatusNotImplemented, "the ai assistant is not configurable on this control plane (no master key set)")
		return false
	}
	return true
}

// handleListAIProviders handles GET /api/v1/settings/ai-assistant/providers.
func (rt *Router) handleListAIProviders(w http.ResponseWriter, r *http.Request) {
	if !rt.requireAIVault(w) {
		return
	}
	list, err := rt.aiVault.List(r.Context())
	if err != nil {
		rt.writeAIVaultError(w, "api: list ai providers failed", err, "")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type setAIProviderRequest struct {
	APIKey       string `json:"api_key"`
	BaseURL      string `json:"base_url"`
	DefaultModel string `json:"default_model"`
}

// handleSetAIProvider handles PUT .../providers/{id}. The route is root-only,
// so the caller may use a private base URL for an OpenAI compatible server;
// that fact is audited.
func (rt *Router) handleSetAIProvider(w http.ResponseWriter, r *http.Request) {
	if !rt.requireAIVault(w) {
		return
	}
	id := r.PathValue("id")
	var req setAIProviderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, aiProviderBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	view, res, err := rt.aiVault.Set(r.Context(), id, ai.SetInput{APIKey: req.APIKey, BaseURL: req.BaseURL, DefaultModel: req.DefaultModel}, true)
	if err != nil {
		rt.writeAIVaultError(w, "api: set ai provider failed", err, id)
		return
	}
	if res.PrivateBaseURL {
		rt.auditAIPrivateBaseURL(r, id, view.BaseURL)
	}
	writeJSON(w, http.StatusOK, view)
}

func (rt *Router) auditAIPrivateBaseURL(r *http.Request, id, baseURL string) {
	ptype, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		rt.logger.Warn("api: audit private ai base url: resolve caller failed", slog.String("provider", id), slog.String("error", err.Error()))
		return
	}
	host := ""
	if u, perr := url.Parse(baseURL); perr == nil {
		host = u.Host
	}
	rt.auditSignIn(r.Context(), r, signInActor{kind: ptype, id: pid, name: pid}, aiAuditPrivateBaseURL,
		"/api/v1/settings/ai-assistant/providers/"+id+" host="+host, http.StatusOK)
}

// handleTestAIProvider handles POST .../providers/{id}/test.
func (rt *Router) handleTestAIProvider(w http.ResponseWriter, r *http.Request) {
	if !rt.requireAIVault(w) {
		return
	}
	id := r.PathValue("id")
	res, err := rt.aiVault.Test(r.Context(), id)
	if err != nil {
		rt.writeAIVaultError(w, "api: test ai provider failed", err, id)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleDeleteAIProvider handles DELETE .../providers/{id}.
func (rt *Router) handleDeleteAIProvider(w http.ResponseWriter, r *http.Request) {
	if !rt.requireAIVault(w) {
		return
	}
	id := r.PathValue("id")
	if err := rt.aiVault.Remove(r.Context(), id); err != nil {
		rt.writeAIVaultError(w, "api: delete ai provider failed", err, id)
		return
	}
	view, err := rt.aiVault.Get(r.Context(), id)
	if err != nil {
		rt.writeAIVaultError(w, "api: delete ai provider failed", err, id)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handleListAIProviderModels handles GET .../providers/{id}/models.
func (rt *Router) handleListAIProviderModels(w http.ResponseWriter, r *http.Request) {
	if !rt.requireAIVault(w) {
		return
	}
	id := r.PathValue("id")
	models, err := rt.aiVault.Models(r.Context(), id)
	if err != nil {
		rt.writeAIVaultError(w, "api: list ai provider models failed", err, id)
		return
	}
	writeJSON(w, http.StatusOK, models)
}

type aiModelRefResource struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type aiLimitsResource struct {
	MaxSteps           int   `json:"max_steps"`
	MonthlyTokenBudget int64 `json:"monthly_token_budget,omitempty"`
}

type aiAssistantSettingsResource struct {
	Configured     bool                 `json:"configured"`
	Active         *aiModelRefResource  `json:"active,omitempty"`
	Fallback       []aiModelRefResource `json:"fallback"`
	Limits         aiLimitsResource     `json:"limits"`
	TokenUsageMont *int64               `json:"token_usage_month,omitempty"`
}

func monthStart(now time.Time) time.Time {
	n := now.UTC()
	return time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func (rt *Router) aiSettingsResource(ctx context.Context) (aiAssistantSettingsResource, error) {
	s, err := rt.aiVault.Settings(ctx)
	if err != nil {
		return aiAssistantSettingsResource{}, err
	}
	lim := ai.LimitsFromEnv()
	out := aiAssistantSettingsResource{
		Configured: s.Configured, Fallback: []aiModelRefResource{},
		Limits: aiLimitsResource{MaxSteps: lim.MaxSteps, MonthlyTokenBudget: lim.MonthlyTokenBudget},
	}
	if s.Active != nil {
		out.Active = &aiModelRefResource{Provider: s.Active.Provider, Model: s.Active.Model}
	}
	for _, f := range s.Fallback {
		out.Fallback = append(out.Fallback, aiModelRefResource{Provider: f.Provider, Model: f.Model})
	}
	if rt.aiUsage != nil {
		used, err := rt.aiUsage.SumAITokensSince(ctx, monthStart(time.Now()))
		if err != nil {
			return aiAssistantSettingsResource{}, err
		}
		out.TokenUsageMont = &used
	}
	return out, nil
}

type aiChoiceRequest struct {
	Active   *aiModelRefResource   `json:"active"`
	Fallback *[]aiModelRefResource `json:"fallback"`
}

func toStoreRef(r aiModelRefResource) store.AIModelRef {
	return store.AIModelRef{Provider: r.Provider, Model: r.Model}
}

// updateAIChoice applies a PUT /settings/ai-assistant body.
func (rt *Router) updateAIChoice(w http.ResponseWriter, r *http.Request, req aiChoiceRequest) {
	var active *store.AIModelRef
	if req.Active != nil {
		a := toStoreRef(*req.Active)
		active = &a
	}
	var fallback *[]store.AIModelRef
	if req.Fallback != nil {
		list := make([]store.AIModelRef, 0, len(*req.Fallback))
		for _, f := range *req.Fallback {
			list = append(list, toStoreRef(f))
		}
		fallback = &list
	}
	if err := rt.aiVault.SetChoice(r.Context(), active, fallback); err != nil {
		rt.writeAIVaultError(w, "api: set ai choice failed", err, "")
		return
	}
	out, err := rt.aiSettingsResource(r.Context())
	if err != nil {
		rt.writeAIVaultError(w, "api: get ai settings failed", err, "")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type aiUsageTotals struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	Requests         int64 `json:"requests"`
	Errors           int64 `json:"errors"`
}

type aiUsageResource struct {
	Days   []store.AIUsageDay `json:"days"`
	Totals aiUsageTotals      `json:"totals"`
}

// handleAIUsage handles GET /api/v1/ai/usage?days=30.
func (rt *Router) handleAIUsage(w http.ResponseWriter, r *http.Request) {
	if rt.aiUsage == nil {
		writeError(w, http.StatusNotImplemented, "usage metering is not available on this control plane")
		return
	}
	days := aiUsageDefaultDays
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > aiUsageMaxDays {
			writeError(w, http.StatusBadRequest, "days must be between 1 and "+strconv.Itoa(aiUsageMaxDays))
			return
		}
		days = n
	}
	since := time.Now().UTC().AddDate(0, 0, -(days - 1))
	since = time.Date(since.Year(), since.Month(), since.Day(), 0, 0, 0, 0, time.UTC)
	rows, err := rt.aiUsage.AIUsageByDay(r.Context(), since)
	if err != nil {
		rt.internalError(w, "api: ai usage failed", err)
		return
	}
	if rows == nil {
		rows = []store.AIUsageDay{}
	}
	out := aiUsageResource{Days: rows}
	for _, d := range rows {
		out.Totals.PromptTokens += d.PromptTokens
		out.Totals.CompletionTokens += d.CompletionTokens
		out.Totals.Requests += d.Requests
		out.Totals.Errors += d.Errors
	}
	writeJSON(w, http.StatusOK, out)
}
