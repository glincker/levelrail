package store

import (
	"context"
	"encoding/json"
	"fmt"
)

// AIProviderAnthropic is the provider id whose key lives in the AIAssistantSecretsKey slot.
const AIProviderAnthropic = "anthropic"

// AIAssistantSecretsKey is the internal/secrets serviceName holding the
// Anthropic key (envKey "api_key"). Secrets are bound to their slot, so the
// name cannot change without re-encrypting stored keys.
func AIAssistantSecretsKey() string {
	return "ai-assistant"
}

// AIAssistantSettings is the single platform-wide AI assistant config
// row. No key field: that goes through internal/secrets instead.
type AIAssistantSettings struct {
	Provider string
	Model    string
	Fallback []AIModelRef
}

// GetAIAssistantSettings returns the single ai_assistant_settings row.
// Always succeeds: the migration itself inserts the row (id = 1).
func (db *DB) GetAIAssistantSettings(ctx context.Context) (AIAssistantSettings, error) {
	var s AIAssistantSettings
	var fallback string
	err := db.QueryRowContext(ctx, `
		SELECT provider, model, fallback FROM ai_assistant_settings WHERE id = 1
	`).Scan(&s.Provider, &s.Model, &fallback)
	if err != nil {
		return AIAssistantSettings{}, fmt.Errorf("store: get ai assistant settings: %w", err)
	}
	if err := json.Unmarshal([]byte(fallback), &s.Fallback); err != nil {
		return AIAssistantSettings{}, fmt.Errorf("store: decode ai fallback chain: %w", err)
	}
	return s, nil
}

// UpdateAIAssistantSettings replaces the single ai_assistant_settings row
// in full, the same whole-record convention UpdateEmailSettings uses.
func (db *DB) UpdateAIAssistantSettings(ctx context.Context, s AIAssistantSettings) error {
	if s.Fallback == nil {
		s.Fallback = []AIModelRef{}
	}
	fallback, err := json.Marshal(s.Fallback)
	if err != nil {
		return fmt.Errorf("store: encode ai fallback chain: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		UPDATE ai_assistant_settings SET provider = ?, model = ?, fallback = ? WHERE id = 1
	`, s.Provider, s.Model, string(fallback))
	if err != nil {
		return fmt.Errorf("store: update ai assistant settings: %w", err)
	}
	return nil
}
