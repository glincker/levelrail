package ai

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrMaxSteps means a turn used its whole tool step budget without a final answer.
var ErrMaxSteps = errors.New("ai: turn stopped at the maximum number of tool steps")

// TurnErrorInfo is a failure explained for the operator.
type TurnErrorInfo struct {
	Code      string
	Message   string
	Fix       string
	Provider  string
	Retryable bool
}

// ErrorSink is an optional Sink extension that receives a typed turn failure.
type ErrorSink interface {
	TurnError(info TurnErrorInfo)
}

// DescribeError turns any turn error into a TurnErrorInfo.
func DescribeError(err error) TurnErrorInfo {
	if pe, ok := AsProviderError(err); ok {
		msg, fix := pe.Explain()
		if pe.Class == ClassBudgetExhausted && pe.Detail != "" {
			msg += " " + pe.Detail + "."
		}
		return TurnErrorInfo{Code: string(pe.Class), Message: msg, Fix: fix, Provider: pe.Provider, Retryable: pe.Retryable()}
	}
	switch {
	case errors.Is(err, ErrMaxSteps):
		return TurnErrorInfo{Code: "max_steps", Message: "The assistant used all of its tool steps without finishing.", Fix: "Ask a narrower question or raise " + EnvMaxSteps + "."}
	case errors.Is(err, ErrNotConfigured), errors.Is(err, ErrNoUsableChain):
		return TurnErrorInfo{Code: "not_configured", Message: "No usable AI provider is configured.", Fix: "Add a provider key in Settings, AI assistant."}
	case errors.Is(err, context.Canceled):
		return TurnErrorInfo{Code: "canceled", Message: "The request was canceled."}
	case errors.Is(err, context.DeadlineExceeded):
		return TurnErrorInfo{Code: "timeout", Message: "The model took too long to answer.", Fix: "Try again, or raise " + EnvStepTimeout + ".", Retryable: true}
	}
	return TurnErrorInfo{Code: "internal", Message: "The assistant hit an unexpected error."}
}

func reportError(sink Sink, err error) {
	if es, ok := sink.(ErrorSink); ok {
		es.TurnError(DescribeError(err))
	}
}

// newLLM returns the provider to call and the model to request. With a
// resolver the chain names its own models, so model is empty.
func (e *Engine) newLLM(ctx context.Context) (Provider, string, error) {
	if e.resolver != nil {
		llm, err := e.resolver.Chain(ctx)
		return llm, "", err
	}
	settings, apiKey, err := e.resolveSettings(ctx)
	if err != nil {
		return nil, "", err
	}
	llm, err := e.provider(settings.Provider, apiKey)
	if err != nil {
		return nil, "", fmt.Errorf("ai: build provider: %w", err)
	}
	return llm, settings.Model, nil
}

// checkBudget blocks a new turn once the month's tokens reach the budget.
func (e *Engine) checkBudget(ctx context.Context, sink Sink) error {
	if e.usage == nil || e.limits.MonthlyTokenBudget <= 0 {
		return nil
	}
	n := e.now().UTC()
	used, err := e.usage.SumAITokensSince(ctx, time.Date(n.Year(), n.Month(), 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return fmt.Errorf("ai: check token budget: %w", err)
	}
	if used < e.limits.MonthlyTokenBudget {
		return nil
	}
	err = &ProviderError{Class: ClassBudgetExhausted, Detail: fmt.Sprintf("%d of %d tokens used this month", used, e.limits.MonthlyTokenBudget)}
	reportError(sink, err)
	return err
}

func (e *Engine) recordAttempt(ctx context.Context, sessionID, messageID string, a Attempt) {
	if e.usage == nil {
		return
	}
	rec := store.AIUsageRecord{
		SessionID: sessionID, MessageID: messageID, CreatedAt: e.now().UTC(), Provider: a.Provider, Model: a.Model,
		PromptTokens: a.Usage.PromptTokens, CompletionTokens: a.Usage.CompletionTokens,
		LatencyMS: int(a.Latency.Milliseconds()), ErrorClass: string(a.ErrorClass),
	}
	// A canceled turn context must not lose the metering row.
	if err := e.usage.RecordAIUsage(context.WithoutCancel(ctx), rec); err != nil {
		e.logger.Warn("ai: record usage failed", slog.String("provider", a.Provider), slog.String("error", err.Error()))
	}
}

// mergeAdjacent joins consecutive same-role messages, which appear after an
// interrupted turn leaves two user messages in a row.
func mergeAdjacent(msgs []Message) []Message {
	var out []Message
	for _, m := range msgs {
		if n := len(out); n > 0 && out[n-1].Role == m.Role {
			out[n-1].Content = append(append([]ContentBlock(nil), out[n-1].Content...), m.Content...)
			continue
		}
		out = append(out, m)
	}
	return out
}
