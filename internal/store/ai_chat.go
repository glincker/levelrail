package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AI chat role values. "tool" carries a tool_result turn (either an
// auto-executed read-only call's result, or a confirmed/rejected
// mutating call's result), never issued directly by the operator.
const (
	AIChatRoleUser      = "user"
	AIChatRoleAssistant = "assistant"
	AIChatRoleTool      = "tool"
)

// AI chat confirmation status values.
const (
	AIChatConfirmationPending  = "pending"
	AIChatConfirmationApproved = "approved"
	AIChatConfirmationRejected = "rejected"
)

var (
	// ErrAIChatSessionNotFound means no ai_chat_sessions row exists for
	// the given id.
	ErrAIChatSessionNotFound = errors.New("store: ai chat session not found")
	// ErrAIChatMessageNotFound means no ai_chat_messages row exists for
	// the given id.
	ErrAIChatMessageNotFound = errors.New("store: ai chat message not found")
	// ErrAIChatConfirmationNotFound means no ai_chat_confirmations row
	// exists for the given id.
	ErrAIChatConfirmationNotFound = errors.New("store: ai chat confirmation not found")
)

// AIChatSession is one chat conversation's identity and timestamps. The
// message history itself lives in AIChatMessage rows, not embedded here.
type AIChatSession struct {
	ID        string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// AIChatToolCall is one tool invocation an assistant turn proposed,
// stored as part of AIChatMessage.ToolCalls and returned verbatim as the
// wire "tool_calls" array GET /api/v1/ai/sessions/{id} documents.
type AIChatToolCall struct {
	// ID is the model's own tool_use id, stable across the propose/
	// execute/result cycle for a single tool call.
	ID string `json:"id"`
	// ConfirmationID is set only for a mutating call: the id
	// POST .../confirmations/{id} resolves.
	ConfirmationID string          `json:"confirmation_id,omitempty"`
	Name           string          `json:"name"`
	Arguments      json.RawMessage `json:"arguments"`
	ReadOnly       bool            `json:"read_only"`
	// Status is "auto_executed" (read-only, already ran), "pending"
	// (mutating, awaiting confirmation), "approved", or "rejected".
	Status  string          `json:"status"`
	Result  json.RawMessage `json:"result,omitempty"`
	IsError bool            `json:"is_error,omitempty"`
}

// AIChatMessage is one turn in a chat session's history: a plain user
// message, an assistant reply (Content plus, optionally, ToolCalls it
// proposed), or a tool-role message carrying one tool_result back to the
// model. ToolUseID is set only on a tool-role message and is never
// serialized over the API: it exists purely so internal/ai can
// reconstruct the Anthropic-format request from stored history.
type AIChatMessage struct {
	ID        string
	SessionID string
	Role      string
	Content   string
	ToolUseID string
	ToolCalls []AIChatToolCall
	CreatedAt time.Time
}

// AIChatConfirmation is one pending-or-resolved mutating tool call,
// gating execution behind POST /api/v1/ai/sessions/{id}/confirmations/{id}.
// A tool call never executes until this row's Status leaves "pending".
type AIChatConfirmation struct {
	ID         string
	SessionID  string
	MessageID  string
	ToolUseID  string
	ToolName   string
	ToolInput  json.RawMessage
	Status     string
	Result     json.RawMessage
	CreatedAt  time.Time
	ResolvedAt *time.Time
}

// newAIChatID mints an opaque, random identifier prefixed for the kind of
// row it names, the same shape NewDeployAttemptID already establishes for
// deploy attempts: no database-assigned sequence, so an id is know-able
// (and usable as a confirmation_id in an SSE event) before any row is
// persisted.
func newAIChatID(prefix string) (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate %s id: %w", prefix, err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// NewAIChatSessionID mints a session id.
func NewAIChatSessionID() (string, error) { return newAIChatID("aisess_") }

// NewAIChatMessageID mints a message id.
func NewAIChatMessageID() (string, error) { return newAIChatID("aimsg_") }

// NewAIChatConfirmationID mints a confirmation id.
func NewAIChatConfirmationID() (string, error) { return newAIChatID("aiconf_") }

// CreateAIChatSession inserts a new, empty session row.
func (db *DB) CreateAIChatSession(ctx context.Context, id string, now time.Time) (AIChatSession, error) {
	_, err := db.ExecContext(ctx, `
		INSERT INTO ai_chat_sessions (id, created_at, updated_at) VALUES (?, ?, ?)
	`, id, now, now)
	if err != nil {
		return AIChatSession{}, fmt.Errorf("store: create ai chat session: %w", err)
	}
	return AIChatSession{ID: id, CreatedAt: now, UpdatedAt: now}, nil
}

// GetAIChatSession returns one session by id, or ErrAIChatSessionNotFound.
func (db *DB) GetAIChatSession(ctx context.Context, id string) (AIChatSession, error) {
	var s AIChatSession
	err := db.QueryRowContext(ctx, `
		SELECT id, created_at, updated_at FROM ai_chat_sessions WHERE id = ?
	`, id).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AIChatSession{}, ErrAIChatSessionNotFound
	}
	if err != nil {
		return AIChatSession{}, fmt.Errorf("store: get ai chat session: %w", err)
	}
	return s, nil
}

// touchAIChatSession bumps a session's updated_at, called by
// SaveAIChatMessage so GET (a future session-list endpoint) can sort by
// recency.
func touchAIChatSession(ctx context.Context, tx *sql.Tx, sessionID string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `UPDATE ai_chat_sessions SET updated_at = ? WHERE id = ?`, now, sessionID)
	if err != nil {
		return fmt.Errorf("store: touch ai chat session: %w", err)
	}
	return nil
}

// SaveAIChatMessage inserts a new message row and bumps its session's
// updated_at in the same transaction.
func (db *DB) SaveAIChatMessage(ctx context.Context, m AIChatMessage) error {
	var toolCallsJSON []byte
	if m.ToolCalls != nil {
		var err error
		toolCallsJSON, err = json.Marshal(m.ToolCalls)
		if err != nil {
			return fmt.Errorf("store: marshal ai chat tool calls: %w", err)
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: save ai chat message: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO ai_chat_messages (id, session_id, role, content, tool_use_id, tool_calls, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, m.ID, m.SessionID, m.Role, m.Content, m.ToolUseID, nullableJSON(toolCallsJSON), m.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save ai chat message: %w", err)
	}

	if err := touchAIChatSession(ctx, tx, m.SessionID, m.CreatedAt); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: save ai chat message: commit: %w", err)
	}
	return nil
}

// nullableJSON returns nil (so the column stores SQL NULL) for an empty
// marshal result, rather than an empty byte slice.
func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}

// UpdateAIChatMessageToolCalls replaces an existing message's tool_calls
// column in place, used once a pending confirmation resolves (the
// assistant message row that proposed it is updated with the outcome,
// see internal/ai's applyConfirmationResult).
func (db *DB) UpdateAIChatMessageToolCalls(ctx context.Context, messageID string, toolCalls []AIChatToolCall) error {
	toolCallsJSON, err := json.Marshal(toolCalls)
	if err != nil {
		return fmt.Errorf("store: update ai chat message tool calls: marshal: %w", err)
	}
	res, err := db.ExecContext(ctx, `
		UPDATE ai_chat_messages SET tool_calls = ? WHERE id = ?
	`, nullableJSON(toolCallsJSON), messageID)
	if err != nil {
		return fmt.Errorf("store: update ai chat message tool calls: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update ai chat message tool calls: rows affected: %w", err)
	}
	if n == 0 {
		return ErrAIChatMessageNotFound
	}
	return nil
}

// ListAIChatMessages returns every message in sessionID, oldest first.
func (db *DB) ListAIChatMessages(ctx context.Context, sessionID string) ([]AIChatMessage, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, session_id, role, content, tool_use_id, tool_calls, created_at
		FROM ai_chat_messages WHERE session_id = ? ORDER BY created_at ASC, id ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("store: list ai chat messages: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []AIChatMessage
	for rows.Next() {
		m, err := scanAIChatMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list ai chat messages: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list ai chat messages: %w", err)
	}
	return out, nil
}

// GetAIChatMessage returns one message by id, or ErrAIChatMessageNotFound.
func (db *DB) GetAIChatMessage(ctx context.Context, id string) (AIChatMessage, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, session_id, role, content, tool_use_id, tool_calls, created_at
		FROM ai_chat_messages WHERE id = ?
	`, id)
	m, err := scanAIChatMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AIChatMessage{}, ErrAIChatMessageNotFound
	}
	if err != nil {
		return AIChatMessage{}, fmt.Errorf("store: get ai chat message: %w", err)
	}
	return m, nil
}

// scanAIChatMessage uses the existing rowScanner interface
// (backup_verification.go) that *sql.Row/*sql.Rows both satisfy.
func scanAIChatMessage(row rowScanner) (AIChatMessage, error) {
	var m AIChatMessage
	var toolCallsJSON sql.NullString
	if err := row.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.ToolUseID, &toolCallsJSON, &m.CreatedAt); err != nil {
		return AIChatMessage{}, err
	}
	if toolCallsJSON.Valid && toolCallsJSON.String != "" {
		if err := json.Unmarshal([]byte(toolCallsJSON.String), &m.ToolCalls); err != nil {
			return AIChatMessage{}, fmt.Errorf("unmarshal tool_calls: %w", err)
		}
	}
	return m, nil
}

// SaveAIChatConfirmation inserts a new, pending confirmation row.
func (db *DB) SaveAIChatConfirmation(ctx context.Context, c AIChatConfirmation) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO ai_chat_confirmations
			(id, session_id, message_id, tool_use_id, tool_name, tool_input, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, c.ID, c.SessionID, c.MessageID, c.ToolUseID, c.ToolName, string(c.ToolInput), c.Status, c.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save ai chat confirmation: %w", err)
	}
	return nil
}

// GetAIChatConfirmation returns one confirmation by id, or
// ErrAIChatConfirmationNotFound.
func (db *DB) GetAIChatConfirmation(ctx context.Context, id string) (AIChatConfirmation, error) {
	c, err := scanAIChatConfirmation(db.QueryRowContext(ctx, `
		SELECT id, session_id, message_id, tool_use_id, tool_name, tool_input, status, result, created_at, resolved_at
		FROM ai_chat_confirmations WHERE id = ?
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return AIChatConfirmation{}, ErrAIChatConfirmationNotFound
	}
	if err != nil {
		return AIChatConfirmation{}, fmt.Errorf("store: get ai chat confirmation: %w", err)
	}
	return c, nil
}

// ListPendingAIChatConfirmations returns every still-pending confirmation
// for messageID, so a caller can tell whether an assistant turn that
// proposed several mutating calls in one response still has any
// unresolved before it's safe to continue that turn.
func (db *DB) ListPendingAIChatConfirmations(ctx context.Context, messageID string) ([]AIChatConfirmation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, session_id, message_id, tool_use_id, tool_name, tool_input, status, result, created_at, resolved_at
		FROM ai_chat_confirmations WHERE message_id = ? AND status = ?
	`, messageID, AIChatConfirmationPending)
	if err != nil {
		return nil, fmt.Errorf("store: list pending ai chat confirmations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []AIChatConfirmation
	for rows.Next() {
		c, err := scanAIChatConfirmation(rows)
		if err != nil {
			return nil, fmt.Errorf("store: list pending ai chat confirmations: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list pending ai chat confirmations: %w", err)
	}
	return out, nil
}

func scanAIChatConfirmation(row rowScanner) (AIChatConfirmation, error) {
	var c AIChatConfirmation
	var toolInput string
	var result sql.NullString
	var resolvedAt sql.NullTime
	if err := row.Scan(&c.ID, &c.SessionID, &c.MessageID, &c.ToolUseID, &c.ToolName, &toolInput, &c.Status, &result, &c.CreatedAt, &resolvedAt); err != nil {
		return AIChatConfirmation{}, err
	}
	c.ToolInput = json.RawMessage(toolInput)
	if result.Valid {
		c.Result = json.RawMessage(result.String)
	}
	if resolvedAt.Valid {
		t := resolvedAt.Time
		c.ResolvedAt = &t
	}
	return c, nil
}

// ResolveAIChatConfirmation transitions a pending confirmation to
// approved or rejected, recording its execution result (nil for a
// rejection). Returns ErrAIChatConfirmationNotFound if id doesn't exist;
// resolving an already-resolved confirmation is idempotent at the SQL
// layer (the WHERE clause below doesn't gate on current status), the
// caller (internal/ai) is what enforces "only once" by checking Status
// before calling this.
func (db *DB) ResolveAIChatConfirmation(ctx context.Context, id, status string, result json.RawMessage, resolvedAt time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE ai_chat_confirmations SET status = ?, result = ?, resolved_at = ? WHERE id = ?
	`, status, nullableJSON(result), resolvedAt, id)
	if err != nil {
		return fmt.Errorf("store: resolve ai chat confirmation: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: resolve ai chat confirmation: rows affected: %w", err)
	}
	if n == 0 {
		return ErrAIChatConfirmationNotFound
	}
	return nil
}
