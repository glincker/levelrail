package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AIChatStore is the core internal/store surface the chat session routes
// need: always set, the same "core Store interface" shape certs/
// staticSites already establish, since a session/message/confirmation
// row's absence is always a legitimate, queryable "not found", not a
// "not configured" condition.
type AIChatStore interface {
	CreateAIChatSession(ctx context.Context, id string, now time.Time) (store.AIChatSession, error)
	GetAIChatSession(ctx context.Context, id string) (store.AIChatSession, error)
	ListAIChatMessages(ctx context.Context, sessionID string) ([]store.AIChatMessage, error)
	GetAIChatConfirmation(ctx context.Context, id string) (store.AIChatConfirmation, error)
}

// AIEngine is the internal/ai.Engine surface the chat routes need.
// nil is valid: every /api/v1/ai/... route returns 501 without one
// configured, the same "not configured" shape secrets/telemetry's
// absence already produce elsewhere in this package.
type AIEngine interface {
	IsConfigured(ctx context.Context) (bool, error)
	RunTurn(ctx context.Context, sessionID, userContent string, sink ai.Sink) error
	ResolveConfirmation(ctx context.Context, sessionID, confirmationID string, approve bool, sink ai.Sink) error
}

type aiChatSessionCreatedResource struct {
	ID string `json:"id"`
}

type aiChatMessageResource struct {
	ID        string                 `json:"id"`
	Role      string                 `json:"role"`
	Content   string                 `json:"content"`
	ToolCalls []store.AIChatToolCall `json:"tool_calls"`
	CreatedAt time.Time              `json:"created_at"`
}

type aiChatSessionResource struct {
	ID       string                  `json:"id"`
	Messages []aiChatMessageResource `json:"messages"`
}

func toAIChatMessageResource(m store.AIChatMessage) aiChatMessageResource {
	return aiChatMessageResource{ID: m.ID, Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls, CreatedAt: m.CreatedAt}
}

// handleCreateAIChatSession handles POST /api/v1/ai/sessions.
func (rt *Router) handleCreateAIChatSession(w http.ResponseWriter, r *http.Request) {
	id, err := store.NewAIChatSessionID()
	if err != nil {
		rt.logger.Error("api: create ai chat session: mint id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	session, err := rt.aiChat.CreateAIChatSession(r.Context(), id, time.Now().UTC())
	if err != nil {
		rt.logger.Error("api: create ai chat session failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, aiChatSessionCreatedResource{ID: session.ID})
}

// handleGetAIChatSession handles GET /api/v1/ai/sessions/{id}: full
// message history, oldest first.
func (rt *Router) handleGetAIChatSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	session, err := rt.aiChat.GetAIChatSession(r.Context(), id)
	if errors.Is(err, store.ErrAIChatSessionNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get ai chat session failed", slog.String("error", err.Error()), slog.String("session_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := rt.aiChat.ListAIChatMessages(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: list ai chat messages failed", slog.String("error", err.Error()), slog.String("session_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	messages := make([]aiChatMessageResource, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toAIChatMessageResource(row))
	}
	writeJSON(w, http.StatusOK, aiChatSessionResource{ID: session.ID, Messages: messages})
}

type createAIChatMessageRequest struct {
	Content string `json:"content"`
}

// handleCreateAIChatMessage handles
// POST /api/v1/ai/sessions/{id}/messages: appends content as a new user
// message and streams the assistant's turn over SSE (text_delta,
// tool_call_proposed, tool_result, done), the same framing convention
// (a bare "data: <json>\n\n" line, no named SSE event, a "type" field
// inside the JSON payload discriminates instead) deploy_attempts.go's
// own sseLogEvent already establishes.
func (rt *Router) handleCreateAIChatMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if rt.aiEngine == nil {
		writeError(w, http.StatusNotImplemented, "the ai assistant is not configured on this control plane (no master key set)")
		return
	}

	if _, err := rt.aiChat.GetAIChatSession(r.Context(), sessionID); errors.Is(err, store.ErrAIChatSessionNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	} else if err != nil {
		rt.logger.Error("api: create ai chat message: load session failed", slog.String("error", err.Error()), slog.String("session_id", sessionID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req createAIChatMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	configured, err := rt.aiEngine.IsConfigured(r.Context())
	if err != nil {
		rt.logger.Error("api: create ai chat message: check configured failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !configured {
		writeError(w, http.StatusNotImplemented, "the ai assistant is not configured: set a provider, model, and api key first")
		return
	}

	sink, ok := rt.startAISSE(w, sessionID)
	if !ok {
		return
	}
	if err := rt.aiEngine.RunTurn(r.Context(), sessionID, req.Content, sink); err != nil {
		rt.logger.Error("api: ai chat run turn failed", slog.String("error", err.Error()), slog.String("session_id", sessionID))
	}
	sink.done()
}

type resolveAIChatConfirmationRequest struct {
	Approve bool `json:"approve"`
}

// handleResolveAIChatConfirmation handles
// POST /api/v1/ai/sessions/{id}/confirmations/{confirmation_id}: executes
// (approve=true) or rejects (approve=false) a previously-proposed
// mutating tool call, then streams the turn's continuation over a fresh
// SSE response, the same event framing handleCreateAIChatMessage uses.
func (rt *Router) handleResolveAIChatConfirmation(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	confirmationID := r.PathValue("confirmation_id")
	if rt.aiEngine == nil {
		writeError(w, http.StatusNotImplemented, "the ai assistant is not configured on this control plane (no master key set)")
		return
	}

	conf, err := rt.aiChat.GetAIChatConfirmation(r.Context(), confirmationID)
	if errors.Is(err, store.ErrAIChatConfirmationNotFound) {
		writeError(w, http.StatusNotFound, "confirmation not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: resolve ai chat confirmation: load confirmation failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// A confirmation id that exists but belongs to another session is
	// treated as not-found, the same cross-resource-leak boundary
	// handleDeployLogStream's own doc comment establishes for a deploy
	// attempt id under the wrong app.
	if conf.SessionID != sessionID {
		writeError(w, http.StatusNotFound, "confirmation not found")
		return
	}
	if conf.Status != store.AIChatConfirmationPending {
		writeError(w, http.StatusConflict, "confirmation already resolved")
		return
	}

	var req resolveAIChatConfirmationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	sink, ok := rt.startAISSE(w, sessionID)
	if !ok {
		return
	}
	if err := rt.aiEngine.ResolveConfirmation(r.Context(), sessionID, confirmationID, req.Approve, sink); err != nil {
		rt.logger.Error("api: ai chat resolve confirmation failed", slog.String("error", err.Error()), slog.String("session_id", sessionID), slog.String("confirmation_id", confirmationID))
	}
	sink.done()
}

// startAISSE writes the SSE response headers and the "connected" priming
// byte (same zero-body-flush workaround as deploy_attempts.go's own
// serveFinishedDeployLog) and returns an aiSSESink ready to stream events.
// ok is false if the ResponseWriter can't be flushed, matching startSSE's
// own contract (it has already written the error response in that case).
func (rt *Router) startAISSE(w http.ResponseWriter, sessionID string) (*aiSSESink, bool) {
	flusher, ok := startSSE(w)
	if !ok {
		rt.logger.Error("api: ai chat stream: response writer does not support flushing", slog.String("session_id", sessionID))
		return nil, false
	}
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()
	return &aiSSESink{w: w, flusher: flusher}, true
}

// aiSSEEvent's Type values, one per SSE event the API contract documents.
const (
	aiSSEEventTextDelta        = "text_delta"
	aiSSEEventToolCallProposed = "tool_call_proposed"
	aiSSEEventToolResult       = "tool_result"
	aiSSEEventDone             = "done"
)

// aiSSESink implements ai.Sink by writing each event as one SSE message:
// "data: <json>\n\n", the payload's own "type" field discriminating which
// of the four shapes below it is, matching sseLogEvent's "everything
// arrives as the default message event" convention.
type aiSSESink struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

func (s *aiSSESink) TextDelta(text string) {
	s.write(struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}{aiSSEEventTextDelta, text})
}

func (s *aiSSESink) ToolCallProposed(confirmationID, toolUseID, toolName string, arguments json.RawMessage) {
	s.write(struct {
		Type           string          `json:"type"`
		ConfirmationID string          `json:"confirmation_id"`
		ToolUseID      string          `json:"tool_use_id"`
		Name           string          `json:"name"`
		Arguments      json.RawMessage `json:"arguments"`
		ReadOnly       bool            `json:"read_only"`
	}{aiSSEEventToolCallProposed, confirmationID, toolUseID, toolName, arguments, false})
}

func (s *aiSSESink) ToolResult(toolUseID, toolName string, result json.RawMessage, isError bool) {
	s.write(struct {
		Type      string          `json:"type"`
		ToolUseID string          `json:"tool_use_id"`
		Name      string          `json:"name"`
		Result    json.RawMessage `json:"result"`
		IsError   bool            `json:"is_error"`
	}{aiSSEEventToolResult, toolUseID, toolName, result, isError})
}

func (s *aiSSESink) done() {
	s.write(struct {
		Type string `json:"type"`
	}{aiSSEEventDone})
}

func (s *aiSSESink) write(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		slog.Default().Error("api: marshal ai chat sse event failed", slog.String("error", err.Error()))
		return
	}
	_, _ = fmt.Fprintf(s.w, "data: %s\n\n", data)
	s.flusher.Flush()
}
