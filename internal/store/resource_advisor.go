package store

import (
	"context"
	"fmt"
	"time"
)

// Advisor finding states.
const (
	AdvisorFindingDismissed = "dismissed"
	AdvisorFindingSnoozed   = "snoozed"
)

// AdvisorFindingState is an operator's decision on one advisor finding.
type AdvisorFindingState struct {
	FindingID    string
	State        string
	SnoozedUntil time.Time
	Actor        string
	UpdatedAt    time.Time
}

// Hides reports whether the state still hides its finding at now.
func (s AdvisorFindingState) Hides(now time.Time) bool {
	return s.State == AdvisorFindingDismissed || (s.State == AdvisorFindingSnoozed && now.Before(s.SnoozedUntil))
}

// SetAdvisorFindingState records a dismissal or snooze, replacing any earlier one.
func (db *DB) SetAdvisorFindingState(ctx context.Context, s AdvisorFindingState) error {
	until := ""
	if s.State == AdvisorFindingSnoozed {
		until = s.SnoozedUntil.UTC().Format(time.RFC3339)
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO resource_advisor_finding_state (finding_id, state, snoozed_until, actor, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(finding_id) DO UPDATE SET state = excluded.state, snoozed_until = excluded.snoozed_until, actor = excluded.actor, updated_at = excluded.updated_at
	`, s.FindingID, s.State, until, s.Actor, s.UpdatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set advisor finding %q state: %w", s.FindingID, err)
	}
	return nil
}

// ClearAdvisorFindingState forgets a decision, so the finding shows again.
func (db *DB) ClearAdvisorFindingState(ctx context.Context, findingID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM resource_advisor_finding_state WHERE finding_id = ?`, findingID); err != nil {
		return fmt.Errorf("store: clear advisor finding %q state: %w", findingID, err)
	}
	return nil
}

// ListAdvisorFindingStates returns every recorded decision by finding id.
func (db *DB) ListAdvisorFindingStates(ctx context.Context) (map[string]AdvisorFindingState, error) {
	rows, err := db.QueryContext(ctx, `SELECT finding_id, state, snoozed_until, actor, updated_at FROM resource_advisor_finding_state`)
	if err != nil {
		return nil, fmt.Errorf("store: list advisor finding states: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]AdvisorFindingState{}
	for rows.Next() {
		var s AdvisorFindingState
		var until, updated string
		if err := rows.Scan(&s.FindingID, &s.State, &until, &s.Actor, &updated); err != nil {
			return nil, fmt.Errorf("store: scan advisor finding state: %w", err)
		}
		if until != "" {
			if s.SnoozedUntil, err = time.Parse(time.RFC3339, until); err != nil {
				return nil, fmt.Errorf("store: parse snooze of %q: %w", s.FindingID, err)
			}
		}
		if s.UpdatedAt, err = time.Parse(time.RFC3339, updated); err != nil {
			return nil, fmt.Errorf("store: parse update time of %q: %w", s.FindingID, err)
		}
		out[s.FindingID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list advisor finding states: %w", err)
	}
	return out, nil
}

// SetBetaFeature records the Settings choice for one beta feature.
func (db *DB) SetBetaFeature(ctx context.Context, feature string, enabled bool, actor string, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO beta_feature_settings (feature, enabled, actor, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(feature) DO UPDATE SET enabled = excluded.enabled, actor = excluded.actor, updated_at = excluded.updated_at
	`, feature, enabled, actor, now.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set beta feature %q: %w", feature, err)
	}
	return nil
}

// ListBetaFeatures returns the Settings choice for every recorded beta feature.
func (db *DB) ListBetaFeatures(ctx context.Context) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT feature, enabled FROM beta_feature_settings`)
	if err != nil {
		return nil, fmt.Errorf("store: list beta features: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var f string
		var on int
		if err := rows.Scan(&f, &on); err != nil {
			return nil, fmt.Errorf("store: scan beta feature: %w", err)
		}
		out[f] = on != 0
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list beta features: %w", err)
	}
	return out, nil
}
