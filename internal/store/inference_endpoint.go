package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrEndpointNotFound is returned when a model has no external endpoint row.
var ErrEndpointNotFound = errors.New("store: inference endpoint not found")

// Endpoint API types.
const (
	EndpointAPIOpenAI = "openai"
	EndpointAPIOllama = "ollama"
)

// Endpoint health states.
const (
	EndpointUnknown     = "unknown"
	EndpointOnline      = "online"
	EndpointSlow        = "slow"
	EndpointUnreachable = "unreachable"
	EndpointAuthFailed  = "auth_failed"
	EndpointError       = "error"
)

// InferenceEndpoint is the connection and routing state of an external model server.
type InferenceEndpoint struct {
	Name            string
	BaseURL         string
	APIType         string
	CredentialSet   bool
	FallbackModel   string
	FallbackRef     string
	LatencyBudget   time.Duration
	HealthState     string
	HealthLatency   time.Duration
	HealthError     string
	HealthCheckedAt time.Time
	LastOKAt        time.Time
	Failures        int
	Discovered      []string
	DiscoveryError  string
	FallbackCount   int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

const endpointColumns = `name, base_url, api_type, credential_set, fallback_model, fallback_model_ref, latency_budget_ms, health_state, health_latency_ms, health_error, health_checked_at, last_ok_at, consecutive_failures, discovered_models, discovery_error, fallback_count, created_at, updated_at`

func scanEndpoint(scan func(dest ...any) error) (*InferenceEndpoint, error) {
	var (
		e                                         InferenceEndpoint
		cred                                      int
		budgetMS, latMS                           int64
		checked, lastOK, models, created, updated string
	)
	if err := scan(&e.Name, &e.BaseURL, &e.APIType, &cred, &e.FallbackModel, &e.FallbackRef, &budgetMS, &e.HealthState, &latMS, &e.HealthError,
		&checked, &lastOK, &e.Failures, &models, &e.DiscoveryError, &e.FallbackCount, &created, &updated); err != nil {
		return nil, err
	}
	e.CredentialSet = cred != 0
	e.LatencyBudget = time.Duration(budgetMS) * time.Millisecond
	e.HealthLatency = time.Duration(latMS) * time.Millisecond
	for dst, src := range map[*time.Time]string{&e.HealthCheckedAt: checked, &e.LastOKAt: lastOK, &e.CreatedAt: created, &e.UpdatedAt: updated} {
		if src == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, src)
		if err != nil {
			return nil, fmt.Errorf("parse endpoint time: %w", err)
		}
		*dst = t
	}
	if err := json.Unmarshal([]byte(models), &e.Discovered); err != nil {
		return nil, fmt.Errorf("parse discovered_models: %w", err)
	}
	return &e, nil
}

// SaveInferenceEndpoint inserts the model row (compute external) with its
// default key, then the endpoint row; a failed second insert removes the first.
func (db *DB) SaveInferenceEndpoint(ctx context.Context, m Model, e InferenceEndpoint) error {
	if err := db.SaveModel(ctx, m); err != nil {
		return err
	}
	disc, err := json.Marshal(nonNilStrings(e.Discovered))
	if err != nil {
		_ = db.DeleteModel(ctx, m.Name)
		return fmt.Errorf("store: marshal discovered models: %w", err)
	}
	now := formatTime(time.Now().UTC())
	_, err = db.ExecContext(ctx, `INSERT INTO inference_endpoints (`+endpointColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		e.Name, e.BaseURL, e.APIType, boolToInt(e.CredentialSet), e.FallbackModel, e.FallbackRef, e.LatencyBudget.Milliseconds(),
		orDefault(e.HealthState, EndpointUnknown), e.HealthLatency.Milliseconds(), e.HealthError, formatOptTime(e.HealthCheckedAt), formatOptTime(e.LastOKAt),
		e.Failures, string(disc), e.DiscoveryError, now, now)
	if err != nil {
		_ = db.DeleteModel(ctx, m.Name)
		return fmt.Errorf("store: save inference endpoint %q: %w", e.Name, err)
	}
	return nil
}

// GetInferenceEndpoint returns the endpoint of model name.
func (db *DB) GetInferenceEndpoint(ctx context.Context, name string) (*InferenceEndpoint, error) {
	e, err := scanEndpoint(db.QueryRowContext(ctx, `SELECT `+endpointColumns+` FROM inference_endpoints WHERE name = ?`, name).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrEndpointNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get inference endpoint %q: %w", name, err)
	}
	return e, nil
}

// ListInferenceEndpoints returns every endpoint ordered by name.
func (db *DB) ListInferenceEndpoints(ctx context.Context) ([]InferenceEndpoint, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+endpointColumns+` FROM inference_endpoints ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list inference endpoints: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []InferenceEndpoint
	for rows.Next() {
		e, err := scanEndpoint(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan inference endpoint: %w", err)
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (db *DB) execEndpointUpdate(ctx context.Context, name, what, query string, args ...any) error {
	res, err := db.ExecContext(ctx, query, append(args, formatTime(time.Now().UTC()), name)...)
	if err != nil {
		return fmt.Errorf("store: %s endpoint %q: %w", what, name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrEndpointNotFound
	}
	return nil
}

// SetInferenceEndpointRouting sets the fallback model and latency budget.
func (db *DB) SetInferenceEndpointRouting(ctx context.Context, name, fallback, fallbackRef string, budget time.Duration) error {
	return db.execEndpointUpdate(ctx, name, "set routing of",
		`UPDATE inference_endpoints SET fallback_model = ?, fallback_model_ref = ?, latency_budget_ms = ?, updated_at = ? WHERE name = ?`, fallback, fallbackRef, budget.Milliseconds())
}

// SetInferenceEndpointCredential records whether a credential is stored.
func (db *DB) SetInferenceEndpointCredential(ctx context.Context, name string, set bool) error {
	return db.execEndpointUpdate(ctx, name, "set credential of",
		`UPDATE inference_endpoints SET credential_set = ?, updated_at = ? WHERE name = ?`, boolToInt(set))
}

// EndpointHealth is one health probe result. KeepModels leaves the stored
// model list alone when discovery failed.
type EndpointHealth struct {
	State          string
	Latency        time.Duration
	Error          string
	CheckedAt      time.Time
	Models         []string
	DiscoveryError string
	KeepModels     bool
}

// RecordInferenceEndpointHealth stores a probe result, counting consecutive
// failures and keeping the last time the endpoint answered.
func (db *DB) RecordInferenceEndpointHealth(ctx context.Context, name string, h EndpointHealth) error {
	ok := h.State == EndpointOnline || h.State == EndpointSlow
	lastOK := ""
	failures := "consecutive_failures + 1"
	if ok {
		lastOK = formatTime(h.CheckedAt)
		failures = "0"
	}
	models := ""
	if !h.KeepModels {
		b, err := json.Marshal(nonNilStrings(h.Models))
		if err != nil {
			return fmt.Errorf("store: marshal discovered models: %w", err)
		}
		models = string(b)
	}
	_, err := db.ExecContext(ctx, `UPDATE inference_endpoints SET health_state = ?, health_latency_ms = ?, health_error = ?, health_checked_at = ?,
		last_ok_at = CASE WHEN ? <> '' THEN ? ELSE last_ok_at END, consecutive_failures = `+failures+`,
		discovered_models = CASE WHEN ? <> '' THEN ? ELSE discovered_models END, discovery_error = ? WHERE name = ?`,
		h.State, h.Latency.Milliseconds(), h.Error, formatTime(h.CheckedAt), lastOK, lastOK, models, models, h.DiscoveryError, name)
	if err != nil {
		return fmt.Errorf("store: record endpoint health %q: %w", name, err)
	}
	return nil
}

// AddInferenceEndpointFallback counts one request served by the fallback.
func (db *DB) AddInferenceEndpointFallback(ctx context.Context, name string) error {
	_, err := db.ExecContext(ctx, `UPDATE inference_endpoints SET fallback_count = fallback_count + 1 WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store: count endpoint fallback %q: %w", name, err)
	}
	return nil
}
