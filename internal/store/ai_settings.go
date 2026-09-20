package store

import (
	"context"
	"fmt"
)

// AIProviderAnthropic is the only supported internal/ai provider today.
// A string enum (not an int) so a future "openai" addition is a new
// constant and a switch case, not a schema change.
const AIProviderAnthropic = "anthropic"

// AIAssistantSecretsKey is the internal/secrets serviceName the BYOK LLM
// API key is stored under (envKey "api_key"), the same sentinel-service
// pattern EmailSettingsSecretsKey already establishes for platform-wide,
// non-app credentials.
func AIAssistantSecretsKey() string {
	return "ai-assistant"
}

// AIInternalTokenSecretsKey is the internal/secrets serviceName the
// control plane's own self-call API token is stored under (envKey
// "token"): minted once so the embedded AI engine can call this
// instance's own REST API the same way any external MCP client would.
// Deliberately a separate serviceName from AIAssistantSecretsKey, not a
// second envKey under it: clearing the operator's BYOK LLM key
// (DELETE /api/v1/settings/ai-assistant) must never invalidate this
// platform-internal credential, since re-enabling the assistant later
// shouldn't require re-minting a root-scoped token.
func AIInternalTokenSecretsKey() string {
	return "ai-assistant-internal-token"
}

// AIAssistantSettings is the single platform-wide AI assistant config
// row. No key field: that goes through internal/secrets instead.
type AIAssistantSettings struct {
	Provider string
	Model    string
}

// GetAIAssistantSettings returns the single ai_assistant_settings row.
// Always succeeds: the migration itself inserts the row (id = 1).
func (db *DB) GetAIAssistantSettings(ctx context.Context) (AIAssistantSettings, error) {
	var s AIAssistantSettings
	err := db.QueryRowContext(ctx, `
		SELECT provider, model FROM ai_assistant_settings WHERE id = 1
	`).Scan(&s.Provider, &s.Model)
	if err != nil {
		return AIAssistantSettings{}, fmt.Errorf("store: get ai assistant settings: %w", err)
	}
	return s, nil
}

// UpdateAIAssistantSettings replaces the single ai_assistant_settings row
// in full, the same whole-record convention UpdateEmailSettings uses.
func (db *DB) UpdateAIAssistantSettings(ctx context.Context, s AIAssistantSettings) error {
	_, err := db.ExecContext(ctx, `
		UPDATE ai_assistant_settings SET provider = ?, model = ? WHERE id = 1
	`, s.Provider, s.Model)
	if err != nil {
		return fmt.Errorf("store: update ai assistant settings: %w", err)
	}
	return nil
}

// ClearAIAssistantSettings resets provider/model to empty, the DELETE
// counterpart to UpdateAIAssistantSettings. Callers also delete the
// underlying secret separately (internal/secrets.Manager has no
// per-key delete, only DeleteAll for a whole serviceName), matching how
// this sentinel service holds exactly one secret value.
func (db *DB) ClearAIAssistantSettings(ctx context.Context) error {
	return db.UpdateAIAssistantSettings(ctx, AIAssistantSettings{})
}
