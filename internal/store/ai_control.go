package store

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// AI control modes, from least to most permissive.
const (
	AIControlModeOff     = "off"
	AIControlModeObserve = "observe"
	AIControlModeOperate = "operate"
	AIControlModeAdmin   = "admin"
)

// EnvironmentKinds lists every environment kind an AI control rule can name.
var EnvironmentKinds = []string{"dev", "test", "uat", "production", "preview", "custom"}

// AIControlSettings is the singleton row that governs agent and AI access.
type AIControlSettings struct {
	Mode            string
	AllowedEnvKinds []string
	UpdatedAt       string
	UpdatedBy       string
}

// ValidAIControlMode reports whether mode is one of the four known modes.
func ValidAIControlMode(mode string) bool {
	return slices.Contains([]string{AIControlModeOff, AIControlModeObserve, AIControlModeOperate, AIControlModeAdmin}, mode)
}

// GetAIControlSettings loads the singleton row.
func (db *DB) GetAIControlSettings(ctx context.Context) (AIControlSettings, error) {
	var s AIControlSettings
	var kinds string
	err := db.QueryRowContext(ctx, `SELECT mode, allowed_env_kinds, updated_at, updated_by FROM ai_control_settings WHERE id = 1`).
		Scan(&s.Mode, &kinds, &s.UpdatedAt, &s.UpdatedBy)
	if err != nil {
		return AIControlSettings{}, fmt.Errorf("store: get ai control settings: %w", err)
	}
	if err := json.Unmarshal([]byte(kinds), &s.AllowedEnvKinds); err != nil {
		return AIControlSettings{}, fmt.Errorf("store: decode ai control env kinds: %w", err)
	}
	return s, nil
}

// UpdateAIControlSettings replaces the mode and allowed environment kinds.
func (db *DB) UpdateAIControlSettings(ctx context.Context, mode string, kinds []string, updatedBy string) error {
	if !ValidAIControlMode(mode) {
		return fmt.Errorf("store: update ai control settings: unknown mode %q", mode)
	}
	if kinds == nil {
		kinds = []string{}
	}
	raw, err := json.Marshal(kinds)
	if err != nil {
		return fmt.Errorf("store: encode ai control env kinds: %w", err)
	}
	_, err = db.ExecContext(ctx, `UPDATE ai_control_settings SET mode = ?, allowed_env_kinds = ?, updated_at = ?, updated_by = ? WHERE id = 1`,
		mode, string(raw), time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), updatedBy)
	if err != nil {
		return fmt.Errorf("store: update ai control settings: %w", err)
	}
	return nil
}

// RevokeAgentTokens revokes every live token with an agent label, except the
// internal token the in-app assistant holds, and returns how many it revoked.
func (db *DB) RevokeAgentTokens(ctx context.Context, internalName, internalAgent string) (int, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE api_tokens SET revoked_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE agent_name != '' AND revoked_at IS NULL AND NOT (name = ? AND agent_name = ?)`, internalName, internalAgent)
	if err != nil {
		return 0, fmt.Errorf("store: revoke agent tokens: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: revoke agent tokens: rows affected: %w", err)
	}
	return int(n), nil
}

// CountAgentTokens counts live tokens with an agent label, excluding the internal assistant token.
func (db *DB) CountAgentTokens(ctx context.Context, internalName, internalAgent string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM api_tokens
		WHERE agent_name != '' AND revoked_at IS NULL AND NOT (name = ? AND agent_name = ?)`, internalName, internalAgent).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count agent tokens: %w", err)
	}
	return n, nil
}

// LabelUnlabeledTokensByName sets the agent label on live tokens of that name that have none.
func (db *DB) LabelUnlabeledTokensByName(ctx context.Context, name, agentName string) error {
	_, err := db.ExecContext(ctx, `UPDATE api_tokens SET agent_name = ? WHERE name = ? AND agent_name = '' AND revoked_at IS NULL`, agentName, name)
	if err != nil {
		return fmt.Errorf("store: label tokens %q: %w", name, err)
	}
	return nil
}
