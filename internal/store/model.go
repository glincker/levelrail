package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrModelNotFound is returned when no model has the given name.
var ErrModelNotFound = errors.New("store: model not found")

// ErrModelExists is returned by SaveModel when the name is taken.
var ErrModelExists = errors.New("store: model already exists")

// ErrModelDomainTaken is returned by SaveModel when another model owns
// the domain.
var ErrModelDomainTaken = errors.New("store: model domain already in use")

// ModelSecretsKey is the internal/secrets namespace holding a model's
// optional HuggingFace token.
func ModelSecretsKey(name string) string { return "model/" + name }

// Model is one AI model resource served from a GPU node. NodeID "" is the
// local node. GPUCount -1 means every GPU on the node; GPUDeviceIDs win
// over GPUCount when set.
type Model struct {
	Name          string
	Engine        string
	ModelRef      string
	NodeID        string
	GPUCount      int
	GPUDeviceIDs  []string
	ContextLength int
	Quantization  string
	Domain        string
	APIKeyHash    string
	APIKeyPrefix  string
	HFTokenSet    bool
	EndpointDial  string
	RestartNonce  int
	Deleting      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time

	// Residency is ResidencyAlways or ResidencyOnDemand. IdleTTLSeconds 0
	// means the platform default. LastActiveAt is the last gateway request
	// or wake; zero means never. ResidencyState is written by the reconciler.
	Residency      string
	IdleTTLSeconds int
	LastActiveAt   time.Time
	ResidencyState string
}

// Model residency modes and reconciler-observed states.
const (
	ResidencyAlways   = "always"
	ResidencyOnDemand = "on_demand"

	ResidencyAwake  = "awake"
	ResidencyAsleep = "asleep"
	ResidencyWaking = "waking"
)

const modelColumns = `name, engine, model_ref, node_id, gpu_count, gpu_device_ids, context_length, quantization, domain, api_key_hash, api_key_prefix, hf_token_set, endpoint_dial, restart_nonce, deleting, created_at, updated_at, residency, idle_ttl_seconds, last_active_at, residency_state`

// SaveModel inserts a new model row and its default API key.
func (db *DB) SaveModel(ctx context.Context, m Model) error {
	ids, err := json.Marshal(nonNilStrings(m.GPUDeviceIDs))
	if err != nil {
		return fmt.Errorf("store: marshal model gpu device ids: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin save model %q: %w", m.Name, err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO models (`+modelColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0, 0, ?, ?, ?, ?, ?, ?)
	`, m.Name, m.Engine, m.ModelRef, m.NodeID, m.GPUCount, string(ids), m.ContextLength, m.Quantization,
		m.Domain, m.APIKeyHash, m.APIKeyPrefix, boolToInt(m.HFTokenSet), formatTime(now), formatTime(now),
		residencyOrDefault(m.Residency), m.IdleTTLSeconds, formatOptTime(m.LastActiveAt), ResidencyAwake)
	if err != nil {
		_ = tx.Rollback() // the pool has one connection, so release it before the lookup
		if _, getErr := db.GetModel(ctx, m.Name); getErr == nil {
			return ErrModelExists
		}
		if m.Domain != "" && strings.Contains(err.Error(), "models.domain") {
			return ErrModelDomainTaken
		}
		return fmt.Errorf("store: save model %q: %w", m.Name, err)
	}
	def := ModelKey{ID: DefaultModelKeyID(m.Name), ModelName: m.Name, Name: DefaultModelKeyName, KeyHash: m.APIKeyHash, KeyPrefix: m.APIKeyPrefix, CreatedAt: now}
	if err := insertModelKey(ctx, tx, def); err != nil {
		return fmt.Errorf("store: save default key of model %q: %w", m.Name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit save model %q: %w", m.Name, err)
	}
	return nil
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func scanModel(scan func(dest ...any) error) (*Model, error) {
	var (
		m                    Model
		ids                  string
		hfSet, deleting      int
		createdAt, updatedAt string
		lastActive           string
	)
	if err := scan(&m.Name, &m.Engine, &m.ModelRef, &m.NodeID, &m.GPUCount, &ids, &m.ContextLength, &m.Quantization,
		&m.Domain, &m.APIKeyHash, &m.APIKeyPrefix, &hfSet, &m.EndpointDial, &m.RestartNonce, &deleting, &createdAt, &updatedAt,
		&m.Residency, &m.IdleTTLSeconds, &lastActive, &m.ResidencyState); err != nil {
		return nil, err
	}
	if lastActive != "" {
		t, err := time.Parse(time.RFC3339Nano, lastActive)
		if err != nil {
			return nil, fmt.Errorf("parse last_active_at: %w", err)
		}
		m.LastActiveAt = t
	}
	if err := json.Unmarshal([]byte(ids), &m.GPUDeviceIDs); err != nil {
		return nil, fmt.Errorf("parse gpu_device_ids: %w", err)
	}
	if len(m.GPUDeviceIDs) == 0 {
		m.GPUDeviceIDs = nil
	}
	m.HFTokenSet = hfSet != 0
	m.Deleting = deleting != 0
	var err error
	if m.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if m.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &m, nil
}

// GetModel returns the model named name, or ErrModelNotFound.
func (db *DB) GetModel(ctx context.Context, name string) (*Model, error) {
	m, err := scanModel(db.QueryRowContext(ctx, `SELECT `+modelColumns+` FROM models WHERE name = ?`, name).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get model %q: %w", name, err)
	}
	return m, nil
}

// GetModelByDomain returns the live (not deleting) model serving domain.
func (db *DB) GetModelByDomain(ctx context.Context, domain string) (*Model, error) {
	m, err := scanModel(db.QueryRowContext(ctx, `SELECT `+modelColumns+` FROM models WHERE domain = ? AND domain <> '' AND deleting = 0`, domain).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrModelNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get model by domain %q: %w", domain, err)
	}
	return m, nil
}

// ListModels returns every model ordered by name, including those being
// deleted.
func (db *DB) ListModels(ctx context.Context) ([]Model, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+modelColumns+` FROM models ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list models: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Model
	for rows.Next() {
		m, err := scanModel(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan model row: %w", err)
		}
		out = append(out, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate model rows: %w", err)
	}
	return out, nil
}

func (db *DB) execModelUpdate(ctx context.Context, name, what, query string, args ...any) error {
	res, err := db.ExecContext(ctx, query, append(args, formatTime(time.Now().UTC()), name)...)
	if err != nil {
		return fmt.Errorf("store: %s model %q: %w", what, name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrModelNotFound
	}
	return nil
}

// SetModelEndpoint records the address the gateway proxies to. Empty
// clears it.
func (db *DB) SetModelEndpoint(ctx context.Context, name, dial string) error {
	return db.execModelUpdate(ctx, name, "set endpoint of", `UPDATE models SET endpoint_dial = ?, updated_at = ? WHERE name = ?`, dial)
}

// RotateModelAPIKey replaces the model's default key at once, keeping the
// legacy hash and prefix on the model row in step.
func (db *DB) RotateModelAPIKey(ctx context.Context, name, hash, prefix string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin rotate key of model %q: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `UPDATE models SET api_key_hash = ?, api_key_prefix = ?, updated_at = ? WHERE name = ?`, hash, prefix, formatTime(now), name)
	if err != nil {
		return fmt.Errorf("store: rotate key of model %q: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrModelNotFound
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM model_keys WHERE id = ?`, DefaultModelKeyID(name)); err != nil {
		return fmt.Errorf("store: drop old default key of model %q: %w", name, err)
	}
	def := ModelKey{ID: DefaultModelKeyID(name), ModelName: name, Name: DefaultModelKeyName, KeyHash: hash, KeyPrefix: prefix, CreatedAt: now}
	if err := insertModelKey(ctx, tx, def); err != nil {
		return fmt.Errorf("store: insert new default key of model %q: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit rotate key of model %q: %w", name, err)
	}
	return nil
}

// SetModelHFTokenSet records whether a HuggingFace token secret exists.
func (db *DB) SetModelHFTokenSet(ctx context.Context, name string, set bool) error {
	return db.execModelUpdate(ctx, name, "set hf token flag of", `UPDATE models SET hf_token_set = ?, updated_at = ? WHERE name = ?`, boolToInt(set))
}

// RestartModel bumps the restart nonce so the controller recreates the
// container.
func (db *DB) RestartModel(ctx context.Context, name string) error {
	return db.execModelUpdate(ctx, name, "restart", `UPDATE models SET restart_nonce = restart_nonce + 1, updated_at = ? WHERE name = ?`)
}

// MarkModelDeleting tombstones the model for the controller to tear down.
func (db *DB) MarkModelDeleting(ctx context.Context, name string) error {
	return db.execModelUpdate(ctx, name, "mark deleting", `UPDATE models SET deleting = 1, endpoint_dial = '', updated_at = ? WHERE name = ?`)
}

// DeleteModel removes the row with its keys and usage. Idempotent.
func (db *DB) DeleteModel(ctx context.Context, name string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin delete model %q: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{`DELETE FROM model_keys WHERE model_name = ?`, `DELETE FROM model_usage_hourly WHERE model_name = ?`, `DELETE FROM models WHERE name = ?`} {
		if _, err := tx.ExecContext(ctx, q, name); err != nil {
			return fmt.Errorf("store: delete model %q: %w", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit delete model %q: %w", name, err)
	}
	return nil
}

func residencyOrDefault(r string) string {
	if r == ResidencyOnDemand {
		return r
	}
	return ResidencyAlways
}

func formatOptTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return formatTime(t)
}

// SetModelResidency sets the residency mode and idle TTL. Switching to
// on-demand counts as activity so the model does not sleep at once;
// switching to always clears any asleep state so the reconciler starts it.
func (db *DB) SetModelResidency(ctx context.Context, name, residency string, idleTTLSeconds int, now time.Time) error {
	residency = residencyOrDefault(residency)
	if residency == ResidencyOnDemand {
		return db.execModelUpdate(ctx, name, "set residency of",
			`UPDATE models SET residency = ?, idle_ttl_seconds = ?, last_active_at = ?, updated_at = ? WHERE name = ?`,
			residency, idleTTLSeconds, formatTime(now))
	}
	return db.execModelUpdate(ctx, name, "set residency of",
		`UPDATE models SET residency = ?, idle_ttl_seconds = ?, residency_state = 'awake', updated_at = ? WHERE name = ?`,
		residency, idleTTLSeconds)
}

// TouchModel records gateway activity (or a wake request) without
// touching updated_at, which tracks configuration changes.
func (db *DB) TouchModel(ctx context.Context, name string, at time.Time) error {
	res, err := db.ExecContext(ctx, `UPDATE models SET last_active_at = ? WHERE name = ?`, formatTime(at), name)
	if err != nil {
		return fmt.Errorf("store: touch model %q: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrModelNotFound
	}
	return nil
}

// SleepModel marks a model idle at once by clearing its activity.
func (db *DB) SleepModel(ctx context.Context, name string) error {
	return db.execModelUpdate(ctx, name, "sleep", `UPDATE models SET last_active_at = '', updated_at = ? WHERE name = ?`)
}

// SetModelResidencyState records the reconciler-observed residency state.
func (db *DB) SetModelResidencyState(ctx context.Context, name, state string) error {
	res, err := db.ExecContext(ctx, `UPDATE models SET residency_state = ? WHERE name = ?`, state, name)
	if err != nil {
		return fmt.Errorf("store: set residency state of model %q: %w", name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrModelNotFound
	}
	return nil
}
