package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Domain condition severities: dropped means the domain is not routed at all,
// degraded means it is routed but without something the operator configured.
const (
	DomainConditionDropped  = "dropped"
	DomainConditionDegraded = "degraded"
)

// Domain condition codes, one per way the ingress pass can fail a domain.
const (
	DomainCondCustomCertUnresolved = "custom_cert_unresolved"
	DomainCondBasicAuthUnresolved  = "basic_auth_unresolved"
	DomainCondDNS01Unavailable     = "dns01_unavailable"
	DomainCondTargetUnavailable    = "target_unavailable"
)

// DomainCondition is one domain's problem from the last ingress pass.
type DomainCondition struct {
	Domain   string `json:"domain"`
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// ReplaceDomainConditions records the problems of the pass that just applied.
func (db *DB) ReplaceDomainConditions(ctx context.Context, conds []DomainCondition) error {
	if conds == nil {
		conds = []DomainCondition{}
	}
	raw, err := json.Marshal(conds)
	if err != nil {
		return fmt.Errorf("store: marshal domain conditions: %w", err)
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO domain_conditions_state (id, conditions, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET conditions = excluded.conditions, updated_at = excluded.updated_at`,
		string(raw), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: replace domain conditions: %w", err)
	}
	return nil
}

// ListDomainConditions returns the last recorded problems and when they were
// recorded. known is false until an ingress pass has applied once.
func (db *DB) ListDomainConditions(ctx context.Context) (conds []DomainCondition, updatedAt time.Time, known bool, err error) {
	var raw, ts string
	err = db.QueryRowContext(ctx, `SELECT conditions, updated_at FROM domain_conditions_state WHERE id = 1`).Scan(&raw, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, false, nil
	}
	if err != nil {
		return nil, time.Time{}, false, fmt.Errorf("store: read domain conditions: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &conds); err != nil {
		return nil, time.Time{}, false, fmt.Errorf("store: decode domain conditions: %w", err)
	}
	updatedAt, err = time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return nil, time.Time{}, false, fmt.Errorf("store: parse domain conditions time: %w", err)
	}
	return conds, updatedAt, true, nil
}
