package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrNoTurnCredential means a turn reached the tool layer without the
// short-lived credential of the human who sent it.
var ErrNoTurnCredential = errors.New("ai: no turn credential: tools run only as the human who sent the message")

// TurnCredential is the authority one chat turn's tool calls run with.
type TurnCredential struct {
	// Token is a short-lived bearer minted for this turn only, capped to the sender.
	Token string
	// ActingAs names the human principal the token stands in for.
	ActingAs string
	// AbilitiesUsed drains the abilities the API checked since its last call; nil is allowed.
	AbilitiesUsed func() []string
}

// ToolAttribution says who a tool call ran as and which abilities it used.
type ToolAttribution struct {
	ConfirmationID string
	ActingAs       string
	Abilities      []string
}

// AttributedSink is a Sink that also streams a tool result's attribution.
type AttributedSink interface {
	ToolResultAttributed(toolUseID, toolName string, result json.RawMessage, isError bool, attr ToolAttribution)
}

// ToolFactory builds the tool surface for one turn, authenticated with token.
type ToolFactory func(ctx context.Context, token string) (ToolExecutor, error)

type turnCredentialKey struct{}

// WithTurnCredential attaches the turn's credential to ctx.
func WithTurnCredential(ctx context.Context, c TurnCredential) context.Context {
	return context.WithValue(ctx, turnCredentialKey{}, c)
}

// TurnCredentialFrom returns the turn credential attached to ctx, if any.
func TurnCredentialFrom(ctx context.Context) (TurnCredential, bool) {
	c, ok := ctx.Value(turnCredentialKey{}).(TurnCredential)
	return c, ok
}

func turnCredentialFrom(ctx context.Context) TurnCredential {
	c, _ := TurnCredentialFrom(ctx)
	return c
}

// SetToolFactory makes every turn build its tools from the sender's own
// turn credential; a turn without one is refused rather than run with e.tools.
func (e *Engine) SetToolFactory(f ToolFactory) { e.factory = f }

// turnTools resolves the tool surface for the turn on ctx. release must be
// called once the turn ends.
func (e *Engine) turnTools(ctx context.Context) (tools ToolExecutor, release func(), err error) {
	if e.factory == nil {
		return e.tools, func() {}, nil
	}
	cred := turnCredentialFrom(ctx)
	if cred.Token == "" {
		return nil, nil, ErrNoTurnCredential
	}
	tools, err = e.factory(ctx, cred.Token)
	if err != nil {
		return nil, nil, fmt.Errorf("ai: build turn tools: %w", err)
	}
	release = func() {}
	if c, ok := tools.(io.Closer); ok {
		release = func() { _ = c.Close() }
	}
	return tools, release, nil
}

func (c TurnCredential) drain() []string {
	if c.AbilitiesUsed == nil {
		return nil
	}
	return c.AbilitiesUsed()
}

func emitToolResult(sink Sink, toolUseID, toolName string, result json.RawMessage, isError bool, attr ToolAttribution) {
	if s, ok := sink.(AttributedSink); ok {
		s.ToolResultAttributed(toolUseID, toolName, result, isError, attr)
		return
	}
	sink.ToolResult(toolUseID, toolName, result, isError)
}
