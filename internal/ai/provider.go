// Package ai implements the BYOK AI assistant chat feature: a
// provider-agnostic turn engine (engine.go) that runs a conversation
// against an LLM (Provider below) with tool use backed by the existing
// MCP tool surface (toolcaller.go, internal/mcptools), pausing every
// mutating tool call for an explicit human confirmation click
// (classify.go). No AI in the reconciliation path: this package only
// ever calls the platform's own REST API the same way any other MCP
// client would (internal/apiclient), matching the CLAUDE.md section 2
// non-goal ("AI is a read-and-suggest layer on top of the API, nothing
// more").
package ai

import (
	"context"
	"encoding/json"
)

// Role values for Message, matching Anthropic's own role vocabulary.
// There is no "tool" role here: a tool result travels as a ContentBlock
// of type ContentBlockToolResult inside a user-role Message, exactly how
// the Anthropic Messages API itself expects it.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// ContentBlock kinds a Message can carry.
const (
	ContentBlockText       = "text"
	ContentBlockToolUse    = "tool_use"
	ContentBlockToolResult = "tool_result"
)

// ContentBlock is one block of a Message's content, mirroring the
// Anthropic Messages API's own content-block union closely enough that
// building a request is a direct translation, not a redesign.
type ContentBlock struct {
	Type string

	// Text is set when Type is ContentBlockText.
	Text string

	// ToolUseID, ToolName, ToolInput are set when Type is
	// ContentBlockToolUse: a tool the model wants to invoke.
	ToolUseID string
	ToolName  string
	ToolInput json.RawMessage

	// ToolResultForID, ToolResultContent, ToolResultIsError are set when
	// Type is ContentBlockToolResult: an already-executed tool's outcome,
	// keyed back to the ContentBlockToolUse.ToolUseID it answers.
	ToolResultForID   string
	ToolResultContent string
	ToolResultIsError bool
}

// Message is one turn in a conversation, provider-agnostic. Engine
// reconstructs this from store.AIChatMessage rows on every turn; Provider
// implementations translate it into their own wire format.
type Message struct {
	Role    string
	Content []ContentBlock
}

// ToolSpec is one callable tool's definition, as ToolCaller.ListTools
// returns it: name, description, and its input JSON Schema, the same
// three fields an MCP tool and an Anthropic tool definition both reduce
// to.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Traits      ToolTraits
}

// ChatRequest is one call to Provider.Complete: the full conversation so
// far (system prompt separate, matching Anthropic's own top-level
// `system` field), the tool catalog, and which model to use.
type ChatRequest struct {
	System   string
	Messages []Message
	Tools    []ToolSpec
	Model    string
}

// ToolUseCall is one tool invocation the model requested in a single
// turn, returned as part of TurnResult.
type ToolUseCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

// Stop reasons TurnResult.StopReason can carry, a small subset of
// Anthropic's own stop_reason values relevant to this engine's loop.
const (
	StopReasonEndTurn   = "end_turn"
	StopReasonToolUse   = "tool_use"
	StopReasonMaxTokens = "max_tokens"
	StopReasonOther     = "other"
)

// TurnResult is what Provider.Complete returns once a single model turn
// finishes: the assistant's full text for this turn (already streamed
// incrementally via the onText callback, repeated here for storage) plus
// any tool calls it wants to make.
type TurnResult struct {
	Text       string
	ToolCalls  []ToolUseCall
	StopReason string
}

// TextDeltaFunc receives one incremental chunk of assistant text as a
// Provider streams its response, so a caller (internal/api's SSE
// handler) can forward it as a text_delta event immediately rather than
// waiting for the whole turn to finish.
type TextDeltaFunc func(delta string)

// Provider sends one turn of a conversation to an LLM and streams its
// text back. Implementations: AnthropicProvider (anthropic.go) for the
// real API; tests substitute a fake satisfying just this interface, no
// network involved.
type Provider interface {
	Complete(ctx context.Context, req ChatRequest, onText TextDeltaFunc) (TurnResult, error)
}
