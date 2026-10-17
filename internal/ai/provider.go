// Package ai implements the BYOK assistant chat: a provider-agnostic turn engine.
// Mutating tool calls pause for explicit human confirmation; no AI runs in the reconciliation path.
package ai

import (
	"context"
	"encoding/json"
	"net/http"
)

// There is no tool role: a tool result is a tool_result block inside a user-role message, as the Messages API requires.
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

// ToolSpec is one tool's name, description, and input JSON Schema, as ToolCaller.ListTools returns it.
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

// TurnResult is the assistant's full text and requested tool calls for one model turn.
type TurnResult struct {
	Text       string
	ToolCalls  []ToolUseCall
	StopReason string
	Usage      Usage
}

// Usage is the token accounting a provider reported for one call. Zero
// means the provider reported nothing.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
}

// ModelInfo is one model a provider offers, normalized across adapters.
type ModelInfo struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	ContextWindow int    `json:"context_window,omitempty"`
	SupportsTools *bool  `json:"supports_tools,omitempty"`
}

// Backend is a Provider that can also list its models and validate its key.
type Backend interface {
	Provider
	ListModels(ctx context.Context) ([]ModelInfo, error)
	// Validate makes one cheap authenticated call and returns a *ProviderError on failure.
	Validate(ctx context.Context) error
}

// AdapterConfig is what every adapter needs to talk to one provider.
type AdapterConfig struct {
	ProviderID string
	BaseURL    string
	APIKey     string
	HTTP       *http.Client
}

func (c AdapterConfig) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return NewHTTPClient(false)
}

// TextDeltaFunc receives each streamed text chunk, so the SSE handler can forward it immediately.
type TextDeltaFunc func(delta string)

// Provider sends one turn to an LLM and streams its text back. Tests substitute a fake with no network.
type Provider interface {
	Complete(ctx context.Context, req ChatRequest, onText TextDeltaFunc) (TurnResult, error)
}
