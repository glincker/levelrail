package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// SecretsAPIKeyEnvKey is the internal/secrets envKey the BYOK LLM API
// key is stored under, keyed by store.AIAssistantSecretsKey(). Shared
// between this package (reading it back for a call) and
// internal/api/ai_settings.go (writing it on PUT), so there is exactly
// one definition of the envKey string.
const SecretsAPIKeyEnvKey = "api_key" //nolint:gosec // an envKey name, not a credential value

var (
	// ErrNotConfigured means no provider/model/key has been saved yet.
	// Callers (internal/api) map this to 501, the same "not configured"
	// shape WithEmailSecrets' absence produces.
	ErrNotConfigured = errors.New("ai: assistant is not configured")
	// ErrPendingConfirmation means the session's last assistant turn
	// still has a mutating tool call awaiting a confirmation click:
	// RunTurn refuses to start a new turn until it resolves, so the
	// conversation sent to the model is never missing a tool_result.
	ErrPendingConfirmation = errors.New("ai: a previous tool call is still awaiting confirmation")
	// ErrConfirmationSessionMismatch means the confirmation id exists but
	// belongs to a different session than the URL's {id} named.
	ErrConfirmationSessionMismatch = errors.New("ai: confirmation does not belong to this session")
	// ErrConfirmationAlreadyResolved means POST .../confirmations/{id}
	// was called a second time for an id that already left "pending".
	ErrConfirmationAlreadyResolved = errors.New("ai: confirmation already resolved")
)

// Sink receives events as Engine runs a turn, translated by the caller
// (internal/api) into the SSE event types the API contract documents:
// TextDelta -> text_delta, ToolCallProposed -> tool_call_proposed,
// ToolResult -> tool_result. The final "done" event is the caller's own
// responsibility once RunTurn/ResolveConfirmation returns, not part of
// this interface, since it must fire exactly once even on an error path.
type Sink interface {
	TextDelta(text string)
	ToolCallProposed(confirmationID, toolUseID, toolName string, arguments json.RawMessage)
	ToolResult(toolUseID, toolName string, result json.RawMessage, isError bool)
}

// Store is the narrow internal/store surface Engine needs, satisfied
// structurally by *store.DB, the same "narrow interface at the boundary"
// shape internal/secrets.Store already establishes for that package.
type Store interface {
	ListAIChatMessages(ctx context.Context, sessionID string) ([]store.AIChatMessage, error)
	SaveAIChatMessage(ctx context.Context, m store.AIChatMessage) error
	UpdateAIChatMessageToolCalls(ctx context.Context, messageID string, toolCalls []store.AIChatToolCall) error
	GetAIChatMessage(ctx context.Context, id string) (store.AIChatMessage, error)
	SaveAIChatConfirmation(ctx context.Context, c store.AIChatConfirmation) error
	GetAIChatConfirmation(ctx context.Context, id string) (store.AIChatConfirmation, error)
	ResolveAIChatConfirmation(ctx context.Context, id, status string, result json.RawMessage, resolvedAt time.Time) error
	ListPendingAIChatConfirmations(ctx context.Context, messageID string) ([]store.AIChatConfirmation, error)
	GetAIAssistantSettings(ctx context.Context) (store.AIAssistantSettings, error)
}

// SecretsResolver is the narrow internal/secrets.Manager surface Engine
// needs to fetch the BYOK key at call time; never cached across turns.
type SecretsResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
}

// ToolExecutor is the narrow ToolCaller surface Engine needs; *ToolCaller
// satisfies this structurally, a fake satisfies it for tests without a
// real in-process MCP session.
type ToolExecutor interface {
	ListTools(ctx context.Context) ([]ToolSpec, error)
	Call(ctx context.Context, name string, args json.RawMessage) (result string, isError bool, err error)
}

// ProviderFactory builds a Provider for one BYOK call, given the
// resolved provider name and API key. Swappable in tests to avoid a real
// network call.
type ProviderFactory func(provider, apiKey string) (Provider, error)

// DefaultProviderFactory supports store.AIProviderAnthropic only today;
// see that constant's own doc comment on why this switch is structured
// to make adding another provider a small addition, not a rewrite.
func DefaultProviderFactory(provider, apiKey string) (Provider, error) {
	switch provider {
	case store.AIProviderAnthropic:
		return NewAnthropicProvider(apiKey, nil), nil
	default:
		return nil, fmt.Errorf("ai: unsupported provider %q", provider)
	}
}

// defaultPlatformName is NewEngine's brandName fallback when the caller
// passes "". No literal product name here: section 3's brand
// indirection rule holds for this package exactly as it does for every
// other package under /internal, so the real name is always injected by
// the caller (from brand.Brand.Name), never hardcoded here.
const defaultPlatformName = "this self-hosted deployment platform"

// systemPromptTemplate is the fixed system prompt every turn sends, with
// %s substituted for brandName.
const systemPromptTemplate = "You are the AI assistant embedded in %s. " +
	"You can read the operator's apps, deploys, logs, metrics, and diagnostics through the tools available to you, " +
	"and you can propose actions (deploying, rolling back, restarting, and other changes). " +
	"Every mutating action you propose pauses for an explicit human confirmation before it runs: you will never see " +
	"its result until the operator approves it, so explain clearly what an action will do and why before proposing it."

// Engine runs the chat turn loop: send the conversation to the model,
// auto-execute any read-only tool call it requests, pause every mutating
// one for confirmation, and repeat until the model produces a plain
// reply with no further tool calls.
type Engine struct {
	store        Store
	secrets      SecretsResolver
	tools        ToolExecutor
	provider     ProviderFactory
	systemPrompt string
}

// NewEngine builds an Engine. provider defaults to DefaultProviderFactory
// if nil. brandName names the platform in the system prompt (the
// caller's own brand.Brand.Name, per section 3 of CLAUDE.md); "" falls
// back to defaultPlatformName rather than a hardcoded product name.
func NewEngine(s Store, secrets SecretsResolver, tools ToolExecutor, provider ProviderFactory, brandName string) *Engine {
	if provider == nil {
		provider = DefaultProviderFactory
	}
	if brandName == "" {
		brandName = defaultPlatformName
	}
	return &Engine{
		store: s, secrets: secrets, tools: tools, provider: provider,
		systemPrompt: fmt.Sprintf(systemPromptTemplate, brandName),
	}
}

// IsConfigured reports whether a provider, model, and key have all been
// saved, so the API layer can return 501 before ever starting an SSE
// response, rather than mid-stream.
func (e *Engine) IsConfigured(ctx context.Context) (bool, error) {
	settings, err := e.store.GetAIAssistantSettings(ctx)
	if err != nil {
		return false, fmt.Errorf("ai: is configured: %w", err)
	}
	if settings.Provider == "" || settings.Model == "" {
		return false, nil
	}
	ok, err := e.secrets.Exists(ctx, store.AIAssistantSecretsKey(), SecretsAPIKeyEnvKey)
	if err != nil {
		return false, fmt.Errorf("ai: is configured: check key: %w", err)
	}
	return ok, nil
}

// RunTurn appends userContent as a new user message and runs the turn
// loop until the model stops (no further tool calls) or a mutating tool
// call is proposed and needs confirmation. sink receives every event as
// it happens; the caller sends the final "done" SSE event once this
// returns, success or error.
func (e *Engine) RunTurn(ctx context.Context, sessionID, userContent string, sink Sink) error {
	rows, err := e.store.ListAIChatMessages(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("ai: run turn: load history: %w", err)
	}
	if hasPendingToolCalls(rows) {
		return ErrPendingConfirmation
	}

	settings, apiKey, err := e.resolveSettings(ctx)
	if err != nil {
		return err
	}
	llm, err := e.provider(settings.Provider, apiKey)
	if err != nil {
		return fmt.Errorf("ai: run turn: %w", err)
	}

	now := time.Now().UTC()
	userMsgID, err := store.NewAIChatMessageID()
	if err != nil {
		return fmt.Errorf("ai: run turn: %w", err)
	}
	if err := e.store.SaveAIChatMessage(ctx, store.AIChatMessage{
		ID: userMsgID, SessionID: sessionID, Role: store.AIChatRoleUser, Content: userContent, CreatedAt: now,
	}); err != nil {
		return fmt.Errorf("ai: run turn: save user message: %w", err)
	}

	return e.continueTurn(ctx, sessionID, settings.Model, llm, sink)
}

// ResolveConfirmation executes (approve=true) or rejects (approve=false)
// a previously-proposed mutating tool call, then, once every mutating
// call from that same assistant turn has been resolved, continues the
// turn using the result(s), streamed to sink the same way RunTurn does.
func (e *Engine) ResolveConfirmation(ctx context.Context, sessionID, confirmationID string, approve bool, sink Sink) error {
	conf, err := e.store.GetAIChatConfirmation(ctx, confirmationID)
	if err != nil {
		return fmt.Errorf("ai: resolve confirmation: %w", err)
	}
	if conf.SessionID != sessionID {
		return ErrConfirmationSessionMismatch
	}
	if conf.Status != store.AIChatConfirmationPending {
		return ErrConfirmationAlreadyResolved
	}

	status, resultRaw, isError := e.executeConfirmation(ctx, conf, approve)
	now := time.Now().UTC()
	if err := e.store.ResolveAIChatConfirmation(ctx, confirmationID, status, resultRaw, now); err != nil {
		return fmt.Errorf("ai: resolve confirmation: %w", err)
	}
	if err := e.applyConfirmationResult(ctx, conf, status, resultRaw, isError); err != nil {
		return err
	}
	sink.ToolResult(conf.ToolUseID, conf.ToolName, resultRaw, isError)

	pending, err := e.store.ListPendingAIChatConfirmations(ctx, conf.MessageID)
	if err != nil {
		return fmt.Errorf("ai: resolve confirmation: %w", err)
	}
	if len(pending) > 0 {
		return nil
	}

	settings, apiKey, err := e.resolveSettings(ctx)
	if err != nil {
		return err
	}
	llm, err := e.provider(settings.Provider, apiKey)
	if err != nil {
		return fmt.Errorf("ai: resolve confirmation: %w", err)
	}
	return e.continueTurn(ctx, sessionID, settings.Model, llm, sink)
}

func (e *Engine) executeConfirmation(ctx context.Context, conf store.AIChatConfirmation, approve bool) (status string, result json.RawMessage, isError bool) {
	if !approve {
		return store.AIChatConfirmationRejected, toJSONRawMessage("User declined to run this action."), true
	}
	text, callIsError, err := e.tools.Call(ctx, conf.ToolName, conf.ToolInput)
	if err != nil {
		return store.AIChatConfirmationApproved, toJSONRawMessage(err.Error()), true
	}
	return store.AIChatConfirmationApproved, toJSONRawMessage(text), callIsError
}

// applyConfirmationResult writes the resolved outcome back onto the
// assistant message's ToolCalls entry in place, so a later turn's
// history reconstruction (buildProviderMessages) sees it.
func (e *Engine) applyConfirmationResult(ctx context.Context, conf store.AIChatConfirmation, status string, result json.RawMessage, isError bool) error {
	msg, err := e.store.GetAIChatMessage(ctx, conf.MessageID)
	if err != nil {
		return fmt.Errorf("ai: apply confirmation result: %w", err)
	}
	for i := range msg.ToolCalls {
		if msg.ToolCalls[i].ID == conf.ToolUseID {
			msg.ToolCalls[i].Status = status
			msg.ToolCalls[i].Result = result
			msg.ToolCalls[i].IsError = isError
		}
	}
	if err := e.store.UpdateAIChatMessageToolCalls(ctx, conf.MessageID, msg.ToolCalls); err != nil {
		return fmt.Errorf("ai: apply confirmation result: %w", err)
	}
	return nil
}

func (e *Engine) resolveSettings(ctx context.Context) (store.AIAssistantSettings, string, error) {
	settings, err := e.store.GetAIAssistantSettings(ctx)
	if err != nil {
		return store.AIAssistantSettings{}, "", fmt.Errorf("ai: resolve settings: %w", err)
	}
	if settings.Provider == "" || settings.Model == "" {
		return store.AIAssistantSettings{}, "", ErrNotConfigured
	}
	apiKey, err := e.secrets.Resolve(ctx, store.AIAssistantSecretsKey(), SecretsAPIKeyEnvKey)
	if err != nil {
		return store.AIAssistantSettings{}, "", fmt.Errorf("ai: resolve settings: resolve key: %w", err)
	}
	return settings, apiKey, nil
}

// continueTurn calls the model once, auto-executes every read-only tool
// call it requests, pauses every mutating one for confirmation, and
// loops back for another model call only when a full round of read-only
// calls resolved with nothing left pending.
func (e *Engine) continueTurn(ctx context.Context, sessionID, model string, llm Provider, sink Sink) error {
	for {
		rows, err := e.store.ListAIChatMessages(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("ai: continue turn: load history: %w", err)
		}
		tools, err := e.tools.ListTools(ctx)
		if err != nil {
			return fmt.Errorf("ai: continue turn: list tools: %w", err)
		}

		turn, err := llm.Complete(ctx, ChatRequest{
			System: e.systemPrompt, Messages: buildProviderMessages(rows), Tools: tools, Model: model,
		}, sink.TextDelta)
		if err != nil {
			return fmt.Errorf("ai: continue turn: %w", err)
		}

		msgID, err := store.NewAIChatMessageID()
		if err != nil {
			return fmt.Errorf("ai: continue turn: %w", err)
		}
		now := time.Now().UTC()

		if len(turn.ToolCalls) == 0 {
			if err := e.store.SaveAIChatMessage(ctx, store.AIChatMessage{
				ID: msgID, SessionID: sessionID, Role: store.AIChatRoleAssistant, Content: turn.Text, CreatedAt: now,
			}); err != nil {
				return fmt.Errorf("ai: continue turn: save assistant message: %w", err)
			}
			return nil
		}

		// The message row must exist before dispatchToolCalls can save a
		// confirmation referencing it (ai_chat_confirmations.message_id
		// is a foreign key), so it's saved here first with a placeholder
		// tool_calls list, then updated in place once dispatch knows each
		// call's real status.
		if err := e.store.SaveAIChatMessage(ctx, store.AIChatMessage{
			ID: msgID, SessionID: sessionID, Role: store.AIChatRoleAssistant, Content: turn.Text, CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("ai: continue turn: save assistant message: %w", err)
		}

		toolCalls, anyPending, err := e.dispatchToolCalls(ctx, sessionID, msgID, now, turn.ToolCalls, sink)
		if err != nil {
			return err
		}
		if err := e.store.UpdateAIChatMessageToolCalls(ctx, msgID, toolCalls); err != nil {
			return fmt.Errorf("ai: continue turn: update assistant message tool calls: %w", err)
		}

		if anyPending {
			return nil
		}
		// Every tool call this turn was read-only and already ran:
		// loop back with its result(s) in history.
	}
}

// dispatchToolCalls classifies and (for read-only calls) executes every
// tool call a single model turn proposed. Mutating calls are recorded as
// pending confirmations and never executed here.
func (e *Engine) dispatchToolCalls(ctx context.Context, sessionID, messageID string, now time.Time, calls []ToolUseCall, sink Sink) (records []store.AIChatToolCall, anyPending bool, err error) {
	records = make([]store.AIChatToolCall, 0, len(calls))
	for _, call := range calls {
		readOnly := IsReadOnly(call.Name)
		record := store.AIChatToolCall{ID: call.ID, Name: call.Name, Arguments: call.Input, ReadOnly: readOnly}

		if readOnly {
			text, isError, callErr := e.tools.Call(ctx, call.Name, call.Input)
			if callErr != nil {
				text, isError = callErr.Error(), true
			}
			record.Status = "auto_executed"
			record.Result = toJSONRawMessage(text)
			record.IsError = isError
			sink.ToolResult(call.ID, call.Name, record.Result, isError)
			records = append(records, record)
			continue
		}

		confirmationID, idErr := store.NewAIChatConfirmationID()
		if idErr != nil {
			return nil, false, fmt.Errorf("ai: dispatch tool calls: %w", idErr)
		}
		record.ConfirmationID = confirmationID
		record.Status = store.AIChatConfirmationPending
		if err := e.store.SaveAIChatConfirmation(ctx, store.AIChatConfirmation{
			ID: confirmationID, SessionID: sessionID, MessageID: messageID,
			ToolUseID: call.ID, ToolName: call.Name, ToolInput: call.Input,
			Status: store.AIChatConfirmationPending, CreatedAt: now,
		}); err != nil {
			return nil, false, fmt.Errorf("ai: dispatch tool calls: save confirmation: %w", err)
		}
		sink.ToolCallProposed(confirmationID, call.ID, call.Name, call.Input)
		anyPending = true
		records = append(records, record)
	}
	return records, anyPending, nil
}

// hasPendingToolCalls reports whether rows' last message is an assistant
// turn with any tool call still awaiting confirmation.
func hasPendingToolCalls(rows []store.AIChatMessage) bool {
	if len(rows) == 0 {
		return false
	}
	last := rows[len(rows)-1]
	if last.Role != store.AIChatRoleAssistant {
		return false
	}
	for _, tc := range last.ToolCalls {
		if tc.Status == store.AIChatConfirmationPending {
			return true
		}
	}
	return false
}

// buildProviderMessages reconstructs the provider-agnostic conversation
// from stored history. Role "tool" rows are skipped: they exist purely
// for GET /api/v1/ai/sessions/{id}'s transcript, not for this
// reconstruction, since an assistant message's own ToolCalls already
// carries every tool_use's authoritative result once resolved (updated
// in place by applyConfirmationResult), which is the single source of
// truth used here instead.
func buildProviderMessages(rows []store.AIChatMessage) []Message {
	var out []Message
	for _, row := range rows {
		switch row.Role {
		case store.AIChatRoleUser:
			out = append(out, Message{Role: RoleUser, Content: []ContentBlock{{Type: ContentBlockText, Text: row.Content}}})
		case store.AIChatRoleAssistant:
			out = append(out, buildAssistantMessages(row)...)
		case store.AIChatRoleTool:
			// Transcript-only, see doc comment above.
		}
	}
	return out
}

func buildAssistantMessages(row store.AIChatMessage) []Message {
	var content []ContentBlock
	if row.Content != "" {
		content = append(content, ContentBlock{Type: ContentBlockText, Text: row.Content})
	}
	for _, tc := range row.ToolCalls {
		content = append(content, ContentBlock{Type: ContentBlockToolUse, ToolUseID: tc.ID, ToolName: tc.Name, ToolInput: tc.Arguments})
	}

	var messages []Message
	if len(content) > 0 {
		messages = append(messages, Message{Role: RoleAssistant, Content: content})
	}

	var results []ContentBlock
	for _, tc := range row.ToolCalls {
		if tc.Status == store.AIChatConfirmationPending {
			continue // caller (hasPendingToolCalls) refuses to reach this state for anything but the very last row
		}
		results = append(results, ContentBlock{
			Type: ContentBlockToolResult, ToolResultForID: tc.ID,
			ToolResultContent: toolResultText(tc.Result), ToolResultIsError: tc.IsError,
		})
	}
	if len(results) > 0 {
		messages = append(messages, Message{Role: RoleUser, Content: results})
	}
	return messages
}

// toJSONRawMessage returns s as-is if it's already valid JSON (the
// common case: MCP tool results are JSON text), or a JSON-encoded string
// otherwise (an error message, most likely), so the stored/wire value is
// always valid JSON either way.
func toJSONRawMessage(s string) json.RawMessage {
	if json.Valid([]byte(s)) {
		return json.RawMessage(s)
	}
	encoded, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return json.RawMessage(encoded)
}

// toolResultText reverses toJSONRawMessage for the common non-JSON-string
// case: if raw is a JSON-encoded string, returns its decoded value;
// otherwise returns raw's own text verbatim (raw JSON content, passed
// through unchanged).
func toolResultText(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}
