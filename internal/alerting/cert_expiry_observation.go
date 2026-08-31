package alerting

import (
	"context"
	"fmt"
	"time"
)

// CertExpiryObservation is what EvaluateCertExpiry persists per
// (rule, domain) after every tick, so the next tick can tell "still
// stuck in the same unrenewed episode" apart from "just entered a fresh
// expiry warning because a real renewal landed." See EvaluateCertExpiry's
// own doc comment for how EpisodeNotAfter/EpisodeStartedAt are used.
type CertExpiryObservation struct {
	RuleID           string
	Domain           string
	Status           string
	NotAfter         time.Time
	EpisodeNotAfter  time.Time
	EpisodeStartedAt time.Time
	ObservedAt       time.Time
}

// UpsertCertExpiryObservation creates or replaces the (rule_id, domain)
// row.
func (db *DB) UpsertCertExpiryObservation(ctx context.Context, o CertExpiryObservation) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO cert_expiry_observations (
			rule_id, domain, status, not_after, episode_not_after, episode_started_at, observed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (rule_id, domain) DO UPDATE SET
			status = excluded.status,
			not_after = excluded.not_after,
			episode_not_after = excluded.episode_not_after,
			episode_started_at = excluded.episode_started_at,
			observed_at = excluded.observed_at
	`,
		o.RuleID, o.Domain, o.Status,
		o.NotAfter.UTC().Format(time.RFC3339Nano),
		o.EpisodeNotAfter.UTC().Format(time.RFC3339Nano),
		o.EpisodeStartedAt.UTC().Format(time.RFC3339Nano),
		o.ObservedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("alerting: upsert cert expiry observation (rule %q, domain %q): %w", o.RuleID, o.Domain, err)
	}
	return nil
}

// ListCertExpiryObservations returns every observation row for ruleID,
// one per domain, unordered.
func (db *DB) ListCertExpiryObservations(ctx context.Context, ruleID string) ([]CertExpiryObservation, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT rule_id, domain, status, not_after, episode_not_after, episode_started_at, observed_at
		FROM cert_expiry_observations
		WHERE rule_id = ?
	`, ruleID)
	if err != nil {
		return nil, fmt.Errorf("alerting: list cert expiry observations for rule %q: %w", ruleID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []CertExpiryObservation
	for rows.Next() {
		var o CertExpiryObservation
		var notAfter, episodeNotAfter, episodeStartedAt, observedAt string
		if err := rows.Scan(&o.RuleID, &o.Domain, &o.Status, &notAfter, &episodeNotAfter, &episodeStartedAt, &observedAt); err != nil {
			return nil, fmt.Errorf("alerting: scan cert expiry observation row: %w", err)
		}
		if o.NotAfter, err = time.Parse(time.RFC3339Nano, notAfter); err != nil {
			return nil, fmt.Errorf("alerting: parse cert expiry observation not_after: %w", err)
		}
		if o.EpisodeNotAfter, err = time.Parse(time.RFC3339Nano, episodeNotAfter); err != nil {
			return nil, fmt.Errorf("alerting: parse cert expiry observation episode_not_after: %w", err)
		}
		if o.EpisodeStartedAt, err = time.Parse(time.RFC3339Nano, episodeStartedAt); err != nil {
			return nil, fmt.Errorf("alerting: parse cert expiry observation episode_started_at: %w", err)
		}
		if o.ObservedAt, err = time.Parse(time.RFC3339Nano, observedAt); err != nil {
			return nil, fmt.Errorf("alerting: parse cert expiry observation observed_at: %w", err)
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alerting: iterate cert expiry observation rows: %w", err)
	}
	return out, nil
}
