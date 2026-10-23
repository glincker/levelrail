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

// AIChatStore is the core store surface the chat routes need, always set since missing rows are normal not-found.
type AIChatStore interface {
	CreateAIChatSession(ctx context.Context, id, owner string, now time.Time) (store.AIChatSession, error)
	GetAIChatSession(ctx context.Context, id string) (store.AIChatSession, error)
	ListAIChatSessions(ctx context.Context, owner string) ([]store.AIChatSession, error)
	DeleteAIChatSession(ctx context.Context, id, owner string) error
	ListAIChatMessages(ctx context.Context, sessionID string) ([]store.AIChatMessage, error)
	GetAIChatConfirmation(ctx context.Context, id string) (store.AIChatConfirmation, error)
}

// AIEngine is the internal/ai.Engine surface the chat routes need; nil makes every /api/v1/ai route return 501.
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

type aiChatSessionSummaryResource struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func toAIChatSessionSummaryResource(s store.AIChatSession) aiChatSessionSummaryResource {
	return aiChatSessionSummaryResource{ID: s.ID, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
}

func toAIChatMessageResource(m store.AIChatMessage) aiChatMessageResource {
	return aiChatMessageResource{ID: m.ID, Role: m.Role, Content: m.Content, ToolCalls: m.ToolCalls, CreatedAt: m.CreatedAt}
}

// aiChatOwner names the calling principal a chat session belongs to.
func (rt *Router) aiChatOwner(r *http.Request) (string, error) {
	ptype, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		return "", err
	}
	return ptype + ":" + pid, nil
}

// ownedAIChatSession loads sessionID for its owner only; another principal's
// session reads as not found so its existence never leaks.
func (rt *Router) ownedAIChatSession(w http.ResponseWriter, r *http.Request, sessionID string) (store.AIChatSession, bool) {
	owner, err := rt.aiChatOwner(r)
	if err != nil {
		rt.internalError(w, "api: ai chat: resolve caller failed", err)
		return store.AIChatSession{}, false
	}
	session, err := rt.aiChat.GetAIChatSession(r.Context(), sessionID)
	if errors.Is(err, store.ErrAIChatSessionNotFound) || (err == nil && session.Owner != owner) {
		writeError(w, http.StatusNotFound, "session not found")
		return store.AIChatSession{}, false
	}
	if err != nil {
		rt.internalError(w, "api: ai chat: load session failed", err, slog.String("session_id", sessionID))
		return store.AIChatSession{}, false
	}
	return session, true
}

// handleCreateAIChatSession handles POST /api/v1/ai/sessions.
func (rt *Router) handleCreateAIChatSession(w http.ResponseWriter, r *http.Request) {
	owner, err := rt.aiChatOwner(r)
	if err != nil {
		rt.internalError(w, "api: create ai chat session: resolve caller failed", err)
		return
	}
	id, err := store.NewAIChatSessionID()
	if err != nil {
		rt.internalError(w, "api: create ai chat session: mint id failed", err)
		return
	}
	session, err := rt.aiChat.CreateAIChatSession(r.Context(), id, owner, time.Now().UTC())
	if err != nil {
		rt.internalError(w, "api: create ai chat session failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, aiChatSessionCreatedResource{ID: session.ID})
}

// handleGetAIChatSession handles GET /api/v1/ai/sessions/{id}: full
// message history, oldest first.
func (rt *Router) handleGetAIChatSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	session, ok := rt.ownedAIChatSession(w, r, id)
	if !ok {
		return
	}

	rows, err := rt.aiChat.ListAIChatMessages(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: list ai chat messages failed", err, slog.String("session_id", id))
		return
	}

	messages := make([]aiChatMessageResource, 0, len(rows))
	for _, row := range rows {
		messages = append(messages, toAIChatMessageResource(row))
	}
	writeJSON(w, http.StatusOK, aiChatSessionResource{ID: session.ID, Messages: messages})
}

// handleListAIChatSessions handles GET /api/v1/ai/sessions: the caller's
// own sessions, most recently updated first, without message content.
func (rt *Router) handleListAIChatSessions(w http.ResponseWriter, r *http.Request) {
	owner, err := rt.aiChatOwner(r)
	if err != nil {
		rt.internalError(w, "api: list ai chat sessions: resolve caller failed", err)
		return
	}
	sessions, err := rt.aiChat.ListAIChatSessions(r.Context(), owner)
	if err != nil {
		rt.internalError(w, "api: list ai chat sessions failed", err)
		return
	}
	out := make([]aiChatSessionSummaryResource, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, toAIChatSessionSummaryResource(s))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteAIChatSession handles DELETE /api/v1/ai/sessions/{id}:
// removes the session and its full message/confirmation history.
func (rt *Router) handleDeleteAIChatSession(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	owner, err := rt.aiChatOwner(r)
	if err != nil {
		rt.internalError(w, "api: delete ai chat session: resolve caller failed", err)
		return
	}
	err = rt.aiChat.DeleteAIChatSession(r.Context(), id, owner)
	if errors.Is(err, store.ErrAIChatSessionNotFound) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete ai chat session failed", err, slog.String("session_id", id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type createAIChatMessageRequest struct {
	Content string `json:"content"`
}

// Streams the assistant turn as unnamed SSE data events whose JSON type field discriminates them.
func (rt *Router) handleCreateAIChatMessage(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	if rt.aiEngine == nil {
		writeError(w, http.StatusNotImplemented, "the ai assistant is not configured on this control plane (no master key set)")
		return
	}

	if _, ok := rt.ownedAIChatSession(w, r, sessionID); !ok {
		return
	}

	var req createAIChatMessageRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	configured, err := rt.aiEngine.IsConfigured(r.Context())
	if err != nil {
		rt.internalError(w, "api: create ai chat message: check configured failed", err)
		return
	}
	if !configured {
		writeError(w, http.StatusNotImplemented, "the ai assistant is not configured: set a provider, model, and api key first")
		return
	}

	r, endTurn, ok := rt.startAITurn(w, r)
	if !ok {
		return
	}
	defer endTurn()
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

// Streams the continuation of the turn over a fresh SSE response, using the same framing.
func (rt *Router) handleResolveAIChatConfirmation(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	confirmationID := r.PathValue("confirmation_id")
	if rt.aiEngine == nil {
		writeError(w, http.StatusNotImplemented, "the ai assistant is not configured on this control plane (no master key set)")
		return
	}
	if _, ok := rt.ownedAIChatSession(w, r, sessionID); !ok {
		return
	}

	conf, err := rt.aiChat.GetAIChatConfirmation(r.Context(), confirmationID)
	if errors.Is(err, store.ErrAIChatConfirmationNotFound) {
		writeError(w, http.StatusNotFound, "confirmation not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: resolve ai chat confirmation: load confirmation failed", err)
		return
	}
	// A confirmation owned by another session reports not-found, not forbidden, to avoid leaking IDs.
	if conf.SessionID != sessionID {
		writeError(w, http.StatusNotFound, "confirmation not found")
		return
	}
	if conf.Status != store.AIChatConfirmationPending {
		writeError(w, http.StatusConflict, "confirmation already resolved")
		return
	}

	var req resolveAIChatConfirmationRequest
	if !decodeReq(w, r, &req) {
		return
	}

	r, endTurn, ok := rt.startAITurn(w, r)
	if !ok {
		return
	}
	defer endTurn()
	sink, ok := rt.startAISSE(w, sessionID)
	if !ok {
		return
	}
	if err := rt.aiEngine.ResolveConfirmation(r.Context(), sessionID, confirmationID, req.Approve, sink); err != nil {
		rt.logger.Error("api: ai chat resolve confirmation failed", slog.String("error", err.Error()), slog.String("session_id", sessionID), slog.String("confirmation_id", confirmationID))
	}
	sink.done()
}

// Writes the SSE headers and priming byte; ok is false if the writer cannot flush.
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
	aiSSEEventError            = "error"
	aiSSEEventDone             = "done"
)

// aiSSESink writes each event as one unnamed SSE data message, with the JSON type field discriminating.
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

func (s *aiSSESink) ToolResultAttributed(toolUseID, toolName string, result json.RawMessage, isError bool, attr ai.ToolAttribution) {
	s.write(struct {
		Type           string          `json:"type"`
		ToolUseID      string          `json:"tool_use_id"`
		ConfirmationID string          `json:"confirmation_id,omitempty"`
		Name           string          `json:"name"`
		Result         json.RawMessage `json:"result"`
		IsError        bool            `json:"is_error"`
		ActingAs       string          `json:"acting_as,omitempty"`
		Abilities      []string        `json:"abilities,omitempty"`
	}{aiSSEEventToolResult, toolUseID, attr.ConfirmationID, toolName, result, isError, attr.ActingAs, attr.Abilities})
}

// TurnError implements ai.ErrorSink: a typed failure with a plain
// explanation and a suggested fix, sent just before "done".
func (s *aiSSESink) TurnError(info ai.TurnErrorInfo) {
	s.write(struct {
		Type      string `json:"type"`
		Code      string `json:"code"`
		Message   string `json:"message"`
		Fix       string `json:"fix,omitempty"`
		Provider  string `json:"provider,omitempty"`
		Retryable bool   `json:"retryable"`
	}{aiSSEEventError, info.Code, info.Message, info.Fix, info.Provider, info.Retryable})
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
