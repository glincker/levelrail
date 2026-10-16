package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// AI provider status values stored on AIProviderConfig.Status.
const (
	AIProviderStatusUntested  = "untested"
	AIProviderStatusOK        = "ok"
	AIProviderStatusInvalid   = "invalid_key"
	AIProviderStatusUnreach   = "unreachable"
	AIProviderStatusExhausted = "quota_exhausted"
)

// ErrAIProviderConfigNotFound means no ai_providers row exists for the id.
var ErrAIProviderConfigNotFound = errors.New("store: ai provider config not found")

const aiTimeLayout = time.RFC3339

// AIModelRef names one provider:model pair in the active choice or fallback chain.
type AIModelRef struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// AIProviderConfig is one provider's non-secret settings. The API key is
// never here: it lives in internal/secrets.
type AIProviderConfig struct {
	ProviderID    string
	BaseURL       string
	DefaultModel  string
	PrivateURLOK  bool
	Status        string
	StatusDetail  string
	LastCheckedAt time.Time
	UpdatedAt     time.Time
}

// AIProviderSecretsKey is the internal/secrets service name holding a
// provider's API key under envKey "api_key". Anthropic keeps the legacy
// name so keys saved by older releases keep working.
func AIProviderSecretsKey(providerID string) string {
	if providerID == AIProviderAnthropic {
		return AIAssistantSecretsKey()
	}
	return "ai-assistant-provider-" + providerID
}

func formatAITime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(aiTimeLayout)
}

func parseAITime(s string) time.Time {
	t, err := time.Parse(aiTimeLayout, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

const aiProviderColumns = `provider_id, base_url, default_model, private_url_ok, status, status_detail, last_checked_at, updated_at`

func scanAIProvider(sc interface{ Scan(...any) error }) (AIProviderConfig, error) {
	var c AIProviderConfig
	var priv int
	var checked, updated string
	if err := sc.Scan(&c.ProviderID, &c.BaseURL, &c.DefaultModel, &priv, &c.Status, &c.StatusDetail, &checked, &updated); err != nil {
		return AIProviderConfig{}, err
	}
	c.PrivateURLOK = priv != 0
	c.LastCheckedAt = parseAITime(checked)
	c.UpdatedAt = parseAITime(updated)
	return c, nil
}

// UpsertAIProviderConfig inserts or fully replaces one provider's settings row.
func (db *DB) UpsertAIProviderConfig(ctx context.Context, c AIProviderConfig) error {
	priv := 0
	if c.PrivateURLOK {
		priv = 1
	}
	if c.Status == "" {
		c.Status = AIProviderStatusUntested
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO `+"ai_providers"+` (`+aiProviderColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(provider_id) DO UPDATE SET base_url = excluded.base_url, default_model = excluded.default_model,
			private_url_ok = excluded.private_url_ok, status = excluded.status, status_detail = excluded.status_detail,
			last_checked_at = excluded.last_checked_at, updated_at = excluded.updated_at
	`, c.ProviderID, c.BaseURL, c.DefaultModel, priv, c.Status, c.StatusDetail, formatAITime(c.LastCheckedAt), formatAITime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: upsert ai provider %q: %w", c.ProviderID, err)
	}
	return nil
}

// GetAIProviderConfig returns one provider's row or ErrAIProviderConfigNotFound.
func (db *DB) GetAIProviderConfig(ctx context.Context, id string) (AIProviderConfig, error) {
	c, err := scanAIProvider(db.QueryRowContext(ctx, `SELECT `+aiProviderColumns+` FROM ai_providers WHERE provider_id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return AIProviderConfig{}, ErrAIProviderConfigNotFound
	}
	if err != nil {
		return AIProviderConfig{}, fmt.Errorf("store: get ai provider %q: %w", id, err)
	}
	return c, nil
}

// ListAIProviderConfigs returns every configured provider row.
func (db *DB) ListAIProviderConfigs(ctx context.Context) ([]AIProviderConfig, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+aiProviderColumns+` FROM ai_providers ORDER BY provider_id`)
	if err != nil {
		return nil, fmt.Errorf("store: list ai providers: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AIProviderConfig
	for rows.Next() {
		c, err := scanAIProvider(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan ai provider: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeleteAIProviderConfig removes one provider's row; a missing row is not an error.
func (db *DB) DeleteAIProviderConfig(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM ai_providers WHERE provider_id = ?`, id); err != nil {
		return fmt.Errorf("store: delete ai provider %q: %w", id, err)
	}
	return nil
}

// SetAIProviderStatus records the outcome of a key test on an existing row.
func (db *DB) SetAIProviderStatus(ctx context.Context, id, status, detail string, at time.Time) error {
	_, err := db.ExecContext(ctx, `UPDATE ai_providers SET status = ?, status_detail = ?, last_checked_at = ? WHERE provider_id = ?`,
		status, detail, formatAITime(at), id)
	if err != nil {
		return fmt.Errorf("store: set ai provider status %q: %w", id, err)
	}
	return nil
}

// GetAIFallbackChain returns the ordered fallback list.
func (db *DB) GetAIFallbackChain(ctx context.Context) ([]AIModelRef, error) {
	var raw string
	if err := db.QueryRowContext(ctx, `SELECT fallback FROM ai_assistant_settings WHERE id = 1`).Scan(&raw); err != nil {
		return nil, fmt.Errorf("store: get ai fallback chain: %w", err)
	}
	var out []AIModelRef
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("store: decode ai fallback chain: %w", err)
	}
	return out, nil
}

// SetAIFallbackChain replaces the ordered fallback list.
func (db *DB) SetAIFallbackChain(ctx context.Context, chain []AIModelRef) error {
	if chain == nil {
		chain = []AIModelRef{}
	}
	raw, err := json.Marshal(chain)
	if err != nil {
		return fmt.Errorf("store: encode ai fallback chain: %w", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE ai_assistant_settings SET fallback = ? WHERE id = 1`, string(raw)); err != nil {
		return fmt.Errorf("store: set ai fallback chain: %w", err)
	}
	return nil
}

// AIUsageRecord is one model call's metering row.
type AIUsageRecord struct {
	SessionID        string
	MessageID        string
	CreatedAt        time.Time
	Provider         string
	Model            string
	PromptTokens     int
	CompletionTokens int
	LatencyMS        int
	ErrorClass       string
}

// AIUsageDay is one (date, provider, model) aggregate.
type AIUsageDay struct {
	Date             string `json:"date"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	Requests         int64  `json:"requests"`
	Errors           int64  `json:"errors"`
}

// RecordAIUsage appends one metering row.
func (db *DB) RecordAIUsage(ctx context.Context, r AIUsageRecord) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO ai_usage (session_id, message_id, created_at, provider, model, prompt_tokens, completion_tokens, latency_ms, error_class)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, r.SessionID, r.MessageID, formatAITime(r.CreatedAt), r.Provider, r.Model, r.PromptTokens, r.CompletionTokens, r.LatencyMS, r.ErrorClass)
	if err != nil {
		return fmt.Errorf("store: record ai usage: %w", err)
	}
	return nil
}

// SumAITokensSince returns prompt plus completion tokens recorded at or after since.
func (db *DB) SumAITokensSince(ctx context.Context, since time.Time) (int64, error) {
	var n int64
	err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(prompt_tokens + completion_tokens), 0) FROM ai_usage WHERE created_at >= ?`,
		formatAITime(since)).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: sum ai tokens: %w", err)
	}
	return n, nil
}

// AIUsageByDay aggregates usage since the given time by day, provider and model.
func (db *DB) AIUsageByDay(ctx context.Context, since time.Time) ([]AIUsageDay, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT substr(created_at, 1, 10) AS d, provider, model, SUM(prompt_tokens), SUM(completion_tokens),
			COUNT(*), SUM(CASE WHEN error_class <> '' THEN 1 ELSE 0 END)
		FROM ai_usage WHERE created_at >= ? GROUP BY d, provider, model ORDER BY d, provider, model
	`, formatAITime(since))
	if err != nil {
		return nil, fmt.Errorf("store: ai usage by day: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []AIUsageDay{}
	for rows.Next() {
		var d AIUsageDay
		if err := rows.Scan(&d.Date, &d.Provider, &d.Model, &d.PromptTokens, &d.CompletionTokens, &d.Requests, &d.Errors); err != nil {
			return nil, fmt.Errorf("store: scan ai usage: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
