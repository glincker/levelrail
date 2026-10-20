package ai

import (
	"context"
	"log/slog"
	"time"
)

// Attempt describes one provider call inside a chain, for metering.
type Attempt struct {
	Provider   string
	Model      string
	Usage      Usage
	Latency    time.Duration
	ErrorClass ErrorClass
}

type attemptObserverKey struct{}

// WithAttemptObserver makes chains report every attempt to fn.
func WithAttemptObserver(ctx context.Context, fn func(Attempt)) context.Context {
	return context.WithValue(ctx, attemptObserverKey{}, fn)
}

func observeAttempt(ctx context.Context, a Attempt) {
	if fn, ok := ctx.Value(attemptObserverKey{}).(func(Attempt)); ok && fn != nil {
		fn(a)
	}
}

// ChainEntry is one provider:model the chain may call.
type ChainEntry struct {
	Provider      string
	Model         string
	Backend       Provider
	ContextWindow int
}

// ChainProvider tries entries in order and falls back only on key- and content-independent failures before any output streams.
type ChainProvider struct {
	Entries        []ChainEntry
	DefaultContext int
	Logger         *slog.Logger
	Now            func() time.Time
}

// Complete implements Provider. req.Model is ignored: each entry names its own.
func (c *ChainProvider) Complete(ctx context.Context, req ChatRequest, onText TextDeltaFunc) (TurnResult, error) {
	if len(c.Entries) == 0 {
		return TurnResult{}, ErrNotConfigured
	}
	now := c.Now
	if now == nil {
		now = time.Now
	}
	logger := c.Logger
	if logger == nil {
		logger = slog.Default()
	}
	streamed := false
	wrapped := func(s string) {
		streamed = true
		if onText != nil {
			onText(s)
		}
	}
	var lastErr error
	for i, e := range c.Entries {
		window := e.ContextWindow
		if window <= 0 {
			window = c.DefaultContext
		}
		r := req
		r.Model = e.Model
		msgs, rep, gerr := GuardContext(req.System, req.Tools, req.Messages, window)
		if gerr != nil {
			pe, _ := AsProviderError(gerr)
			pe.Provider = e.Provider
			return TurnResult{}, pe
		}
		if rep.DroppedToolResults+rep.DroppedMessages > 0 {
			logger.Info("ai: trimmed conversation to fit context", slog.String("provider", e.Provider), slog.String("model", e.Model),
				slog.Int("dropped_tool_results", rep.DroppedToolResults), slog.Int("dropped_messages", rep.DroppedMessages))
		}
		r.Messages = msgs

		start := now()
		res, err := e.Backend.Complete(ctx, r, wrapped)
		att := Attempt{Provider: e.Provider, Model: e.Model, Usage: res.Usage, Latency: now().Sub(start)}
		if err != nil {
			att.ErrorClass = ClassNetwork
			if pe, ok := AsProviderError(err); ok {
				att.ErrorClass = pe.Class
			}
		}
		observeAttempt(ctx, att)
		if err == nil {
			return res, nil
		}
		lastErr = err
		pe, ok := AsProviderError(err)
		if !ok || !pe.FallbackEligible() || streamed || ctx.Err() != nil || i == len(c.Entries)-1 {
			return res, err
		}
		logger.Warn("ai: provider failed, trying next in chain", slog.String("provider", e.Provider),
			slog.String("model", e.Model), slog.String("class", string(pe.Class)), slog.String("next_provider", c.Entries[i+1].Provider))
	}
	return TurnResult{}, lastErr
}
