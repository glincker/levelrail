package alerting

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Rule severities, used by silence matchers and shown in history.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityCritical = "critical"
)

func validSeverity(s string) bool {
	return s == SeverityInfo || s == SeverityWarning || s == SeverityCritical
}

func severityOrDefault(s string) string {
	if s == "" {
		return SeverityWarning
	}
	return s
}

func encodeLabels(l map[string]string) string {
	if len(l) == 0 {
		return ""
	}
	b, err := json.Marshal(l)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeLabels(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// AlertContext is what a silence matcher or maintenance window is
// evaluated against: the rule plus where its app is placed.
type AlertContext struct {
	RuleID   string
	Kind     string
	App      string
	NodeID   string
	NodeName string
	Labels   map[string]string
	Severity string
}

// SilenceMatcher selects alerts. Every non-empty field must match (AND);
// within a list field any entry may match (OR).
type SilenceMatcher struct {
	RuleIDs    []string          `json:"rule_ids,omitempty"`
	Kinds      []string          `json:"kinds,omitempty"`
	Apps       []string          `json:"apps,omitempty"`
	Nodes      []string          `json:"nodes,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Severities []string          `json:"severities,omitempty"`
}

// IsEmpty reports whether the matcher has no criteria at all.
func (m SilenceMatcher) IsEmpty() bool {
	return len(m.RuleIDs) == 0 && len(m.Kinds) == 0 && len(m.Apps) == 0 &&
		len(m.Nodes) == 0 && len(m.Labels) == 0 && len(m.Severities) == 0
}

// Validate rejects a matcher that would match everything or names an unknown severity.
func (m SilenceMatcher) Validate() error {
	if m.IsEmpty() {
		return errors.New("silence needs at least one matcher (rule_ids, kinds, apps, nodes, labels or severities)")
	}
	for _, s := range m.Severities {
		if !validSeverity(s) {
			return fmt.Errorf("severity %q must be %q, %q or %q", s, SeverityInfo, SeverityWarning, SeverityCritical)
		}
	}
	return nil
}

// Matches reports whether c satisfies every criterion in m.
func (m SilenceMatcher) Matches(c AlertContext) bool {
	if len(m.RuleIDs) > 0 && !containsString(m.RuleIDs, c.RuleID) {
		return false
	}
	if len(m.Kinds) > 0 && !containsString(m.Kinds, c.Kind) {
		return false
	}
	if len(m.Apps) > 0 && (c.App == "" || !containsString(m.Apps, c.App)) {
		return false
	}
	if len(m.Nodes) > 0 {
		if c.NodeID == "" && c.NodeName == "" {
			return false
		}
		if !containsString(m.Nodes, c.NodeID) && !containsString(m.Nodes, c.NodeName) {
			return false
		}
	}
	for k, v := range m.Labels {
		if got, ok := c.Labels[k]; !ok || got != v {
			return false
		}
	}
	if len(m.Severities) > 0 && !containsString(m.Severities, severityOrDefault(c.Severity)) {
		return false
	}
	return true
}

func containsString(list []string, v string) bool {
	if v == "" {
		return false
	}
	for _, s := range list {
		if strings.EqualFold(s, v) {
			return true
		}
	}
	return false
}

// Silence status values.
const (
	SilencePending = "pending"
	SilenceActive  = "active"
	SilenceExpired = "expired"
)

// Silence suppresses notifications for matching alerts between StartsAt and EndsAt.
type Silence struct {
	ID        string
	Matchers  SilenceMatcher
	StartsAt  time.Time
	EndsAt    time.Time
	CreatedBy string
	Reason    string
	CreatedAt time.Time
	ExpiredAt *time.Time
}

// EffectiveEnd is when the silence stopped or will stop applying.
func (s Silence) EffectiveEnd() time.Time {
	if s.ExpiredAt != nil && s.ExpiredAt.Before(s.EndsAt) {
		return *s.ExpiredAt
	}
	return s.EndsAt
}

// Status reports pending, active or expired at now.
func (s Silence) Status(now time.Time) string {
	switch {
	case !now.Before(s.EffectiveEnd()):
		return SilenceExpired
	case now.Before(s.StartsAt):
		return SilencePending
	default:
		return SilenceActive
	}
}

// ErrSilenceNotFound is returned when no silence has that ID.
var ErrSilenceNotFound = errors.New("alerting: silence not found")

// NewSilenceID generates a random silence identifier.
func NewSilenceID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("alerting: generate silence id: %w", err)
	}
	return "sil_" + hex.EncodeToString(b), nil
}

// Validate checks the silence's matchers and time range.
func (s Silence) Validate() error {
	if err := s.Matchers.Validate(); err != nil {
		return err
	}
	if !s.EndsAt.After(s.StartsAt) {
		return errors.New("ends_at must be after starts_at")
	}
	return nil
}

// CreateSilence inserts a new silence. Never updates in place: expiring
// early is ExpireSilence, so history stays intact.
func (db *DB) CreateSilence(ctx context.Context, s Silence) error {
	matchers, err := json.Marshal(s.Matchers)
	if err != nil {
		return fmt.Errorf("alerting: encode silence matchers: %w", err)
	}
	created := s.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO alert_silences (id, matchers_json, starts_at, ends_at, created_by, reason, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.ID, string(matchers), fmtTime(s.StartsAt), fmtTime(s.EndsAt), s.CreatedBy, s.Reason, fmtTime(created))
	if err != nil {
		return fmt.Errorf("alerting: create silence %q: %w", s.ID, err)
	}
	return nil
}

const silenceColumns = `SELECT id, matchers_json, starts_at, ends_at, created_by, reason, created_at, expired_at FROM alert_silences`

func scanSilence(scan func(dest ...any) error) (*Silence, error) {
	var (
		s                             Silence
		matchers, starts, ends, added string
		expired                       sql.NullString
	)
	if err := scan(&s.ID, &matchers, &starts, &ends, &s.CreatedBy, &s.Reason, &added, &expired); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(matchers), &s.Matchers); err != nil {
		return nil, fmt.Errorf("decode matchers: %w", err)
	}
	var err error
	if s.StartsAt, err = time.Parse(time.RFC3339Nano, starts); err != nil {
		return nil, fmt.Errorf("parse starts_at: %w", err)
	}
	if s.EndsAt, err = time.Parse(time.RFC3339Nano, ends); err != nil {
		return nil, fmt.Errorf("parse ends_at: %w", err)
	}
	if s.CreatedAt, err = time.Parse(time.RFC3339Nano, added); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if s.ExpiredAt, err = parseNullableTime(expired); err != nil {
		return nil, fmt.Errorf("parse expired_at: %w", err)
	}
	return &s, nil
}

// GetSilence returns one silence or ErrSilenceNotFound.
func (db *DB) GetSilence(ctx context.Context, id string) (*Silence, error) {
	s, err := scanSilence(db.QueryRowContext(ctx, silenceColumns+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSilenceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("alerting: get silence %q: %w", id, err)
	}
	return s, nil
}

// ListSilences returns silences newest first, capped at limit. When
// includeExpired is false only pending and active ones at now are returned.
func (db *DB) ListSilences(ctx context.Context, now time.Time, includeExpired bool, limit int) ([]Silence, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := db.QueryContext(ctx, silenceColumns+` ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("alerting: list silences: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Silence
	for rows.Next() {
		s, err := scanSilence(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("alerting: scan silence row: %w", err)
		}
		if !includeExpired && s.Status(now) == SilenceExpired {
			continue
		}
		out = append(out, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alerting: iterate silence rows: %w", err)
	}
	return out, nil
}

// ListActiveSilences returns only the silences applying at now.
func (db *DB) ListActiveSilences(ctx context.Context, now time.Time) ([]Silence, error) {
	all, err := db.ListSilences(ctx, now, false, 1000)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, s := range all {
		if s.Status(now) == SilenceActive {
			out = append(out, s)
		}
	}
	return out, nil
}

// ExpireSilence ends a silence at now. Expiring an already-expired
// silence is a no-op; the row is kept for history.
func (db *DB) ExpireSilence(ctx context.Context, id string, now time.Time) (*Silence, error) {
	s, err := db.GetSilence(ctx, id)
	if err != nil {
		return nil, err
	}
	if s.Status(now) == SilenceExpired {
		return s, nil
	}
	if _, err := db.ExecContext(ctx, `UPDATE alert_silences SET expired_at = ? WHERE id = ?`, fmtTime(now), id); err != nil {
		return nil, fmt.Errorf("alerting: expire silence %q: %w", id, err)
	}
	n := now.UTC()
	s.ExpiredAt = &n
	return s, nil
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
