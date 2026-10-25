package store

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	kitids "github.com/GLINCKER/levelrail/kit/ids"
)

// Stack store errors. internal/iac imports this package, so the contract
// errors there (ErrStackLocked, ErrLedgerConflict) cannot be returned here;
// the API layer maps these onto them.
var (
	ErrStackNotFound         = errors.New("store: stack not found")
	ErrStackExists           = errors.New("store: stack already exists")
	ErrStackRevisionNotFound = errors.New("store: stack revision not found")
	ErrStackBadTransition    = errors.New("store: invalid stack revision status transition")
	ErrStackLedgerConflict   = errors.New("store: resource is owned by another stack")
	ErrStackLocked           = errors.New("store: stack is locked by another apply")
	ErrStackLockLost         = errors.New("store: stack lock is not held")
	ErrStackSecretLeak       = errors.New("store: revision documents contain a secret value")
)

// Stack revision statuses; the string values match internal/iac.
const (
	StackRevPendingApproval = "pending_approval"
	StackRevApplying        = "applying"
	StackRevApplied         = "applied"
	StackRevPartial         = "partial"
	StackRevFailed          = "failed"
	StackRevSuperseded      = "superseded"
)

// stackRevNext mirrors iac.RevisionStatus.CanTransition (store cannot import iac).
var stackRevNext = map[string][]string{
	StackRevPendingApproval: {StackRevApplying, StackRevFailed},
	StackRevApplying:        {StackRevApplied, StackRevPartial, StackRevFailed},
	StackRevApplied:         {StackRevSuperseded},
	StackRevPartial:         {StackRevSuperseded},
	StackRevFailed:          {},
	StackRevSuperseded:      {},
}

// StackRevisionCanTransition reports whether a revision may move from -> to.
func StackRevisionCanTransition(from, to string) bool {
	for _, n := range stackRevNext[from] {
		if n == to {
			return true
		}
	}
	return false
}

// StackRecord is one row of stacks.
type StackRecord struct {
	ID              string
	Name            string
	ProjectID       string
	EnvironmentID   string
	AllowPrune      bool
	DriftPolicy     string
	PinnedRevision  int
	RequireApproval bool
	CreatedAt       time.Time
	CreatedBy       string
}

// StackRevisionRecord is one numbered apply. RenderedDocs is plain bytes here;
// the store gzips it on write and inflates it on read.
type StackRevisionRecord struct {
	StackID        string
	Number         int
	Status         string
	PlanHash       string
	PlanJSON       []byte
	RenderedDocs   []byte
	RollbackOf     int
	IdempotencyKey string
	Actor          string
	TokenID        string
	// RequesterUserID is the session user or token owner; Actor is display only.
	RequesterUserID string
	GitRepo         string
	GitRef          string
	GitSHA          string
	CIRunURL        string
	CreatedAt       time.Time
	FinishedAt      *time.Time
	ResultJSON      []byte
	// Replayed is set when AppendStackRevision matched an earlier idempotency key.
	Replayed bool
}

// NewStackRevision is the input to AppendStackRevision.
type NewStackRevision struct {
	StackID         string
	Status          string
	PlanHash        string
	PlanJSON        []byte
	RenderedDocs    []byte
	RollbackOf      int
	IdempotencyKey  string
	Actor           string
	TokenID         string
	RequesterUserID string
	GitRepo         string
	GitRef          string
	GitSHA          string
	CIRunURL        string
	// Secrets maps secret names to values; none may appear in RenderedDocs or PlanJSON.
	Secrets map[string]string
}

// StackLedgerEntry records the one stack owning a resource.
type StackLedgerEntry struct {
	StackID       string
	Kind          string
	Key           string
	LiveID        string
	SinceRevision int
}

// StackLockRecord is the per-stack apply lease.
type StackLockRecord struct {
	StackID   string
	Holder    string
	Revision  int
	ExpiresAt time.Time
}

// Expired reports whether another holder may take the lease over.
func (l StackLockRecord) Expired(now time.Time) bool { return !now.Before(l.ExpiresAt) }

func stackTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

const stackCols = `id, name, project_id, environment_id, allow_prune, drift_policy, pinned_revision, require_approval, created_at, created_by`

func scanStack(r rowScanner) (StackRecord, error) {
	var s StackRecord
	var prune, appr int
	var created string
	if err := r.Scan(&s.ID, &s.Name, &s.ProjectID, &s.EnvironmentID, &prune, &s.DriftPolicy, &s.PinnedRevision, &appr, &created, &s.CreatedBy); err != nil {
		return StackRecord{}, err
	}
	s.AllowPrune, s.RequireApproval = prune != 0, appr != 0
	s.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return s, nil
}

// CreateStack inserts s, assigning its id and creation time.
func (db *DB) CreateStack(ctx context.Context, s StackRecord, now time.Time) (StackRecord, error) {
	if strings.TrimSpace(s.Name) == "" || s.ProjectID == "" {
		return StackRecord{}, errors.New("store: create stack: name and project are required")
	}
	suffix, err := kitids.Hex(8)
	if err != nil {
		return StackRecord{}, fmt.Errorf("store: create stack: id: %w", err)
	}
	s.ID = "stk_" + suffix
	if s.DriftPolicy == "" {
		s.DriftPolicy = "warn"
	}
	s.CreatedAt = now.UTC()
	_, err = db.ExecContext(ctx, `INSERT INTO stacks (`+stackCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Name, s.ProjectID, s.EnvironmentID, boolInt(s.AllowPrune), s.DriftPolicy, s.PinnedRevision,
		boolInt(s.RequireApproval), stackTime(now), s.CreatedBy)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return StackRecord{}, ErrStackExists
		}
		return StackRecord{}, fmt.Errorf("store: create stack %q: %w", s.Name, err)
	}
	return s, nil
}

// GetStackByName returns the stack called name.
func (db *DB) GetStackByName(ctx context.Context, name string) (StackRecord, error) {
	s, err := scanStack(db.QueryRowContext(ctx, `SELECT `+stackCols+` FROM stacks WHERE name = ?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return StackRecord{}, ErrStackNotFound
	}
	if err != nil {
		return StackRecord{}, fmt.Errorf("store: get stack %q: %w", name, err)
	}
	return s, nil
}

// GetStack returns the stack with id.
func (db *DB) GetStack(ctx context.Context, id string) (StackRecord, error) {
	s, err := scanStack(db.QueryRowContext(ctx, `SELECT `+stackCols+` FROM stacks WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return StackRecord{}, ErrStackNotFound
	}
	if err != nil {
		return StackRecord{}, fmt.Errorf("store: get stack %q: %w", id, err)
	}
	return s, nil
}

// ListStacks returns every stack ordered by name.
func (db *DB) ListStacks(ctx context.Context) ([]StackRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+stackCols+` FROM stacks ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list stacks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []StackRecord{}
	for rows.Next() {
		s, err := scanStack(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan stack: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list stacks: %w", err)
	}
	return out, nil
}

// UpdateStackSettings changes the admin-owned settings of a stack.
func (db *DB) UpdateStackSettings(ctx context.Context, id string, allowPrune bool, driftPolicy string, requireApproval bool) error {
	res, err := db.ExecContext(ctx, `UPDATE stacks SET allow_prune = ?, drift_policy = ?, require_approval = ? WHERE id = ?`,
		boolInt(allowPrune), driftPolicy, boolInt(requireApproval), id)
	if err != nil {
		return fmt.Errorf("store: update stack settings: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStackNotFound
	}
	return nil
}

// DeleteStack removes the stack with its revisions, ledger and lock and
// returns the ledger it held. mode ("forget" or "destroy", empty means forget)
// is validated here; destroying the returned resources is the caller's job.
func (db *DB) DeleteStack(ctx context.Context, id, mode string) ([]StackLedgerEntry, error) {
	if mode != "" && mode != "forget" && mode != "destroy" {
		return nil, fmt.Errorf("store: delete stack: unknown mode %q", mode)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("store: delete stack: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	held, err := listLedger(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM stacks WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("store: delete stack: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrStackNotFound
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: delete stack: commit: %w", err)
	}
	return held, nil
}

// SetStackPin freezes the stack to revision, which must exist.
func (db *DB) SetStackPin(ctx context.Context, id string, revision int) error {
	if revision < 1 {
		return errors.New("store: pin revision must be positive")
	}
	res, err := db.ExecContext(ctx, `UPDATE stacks SET pinned_revision = ?
		WHERE id = ? AND EXISTS (SELECT 1 FROM stack_revisions WHERE stack_id = ? AND number = ?)`, revision, id, id, revision)
	if err != nil {
		return fmt.Errorf("store: pin stack: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, gerr := db.GetStack(ctx, id); gerr != nil {
			return gerr
		}
		return ErrStackRevisionNotFound
	}
	return nil
}

// ClearStackPin removes the pin.
func (db *DB) ClearStackPin(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `UPDATE stacks SET pinned_revision = 0 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: unpin stack: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStackNotFound
	}
	return nil
}

func gzipBytes(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gunzipBytes(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	return io.ReadAll(zr)
}

// leakedStackSecret mirrors iac.LeakedSecret, sorted so the reported name is stable.
func leakedStackSecret(secrets map[string]string, docs ...[]byte) (string, bool) {
	names := make([]string, 0, len(secrets))
	for n := range secrets {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		v := secrets[n]
		if v == "" {
			continue
		}
		// Rendered docs and plans are JSON, so also look for the escaped form.
		esc, _ := json.Marshal(v)
		esc = esc[1 : len(esc)-1]
		for _, d := range docs {
			if bytes.Contains(d, []byte(v)) || bytes.Contains(d, esc) {
				return n, true
			}
		}
	}
	return "", false
}

const stackRevCols = `stack_id, number, status, plan_hash, plan_json, rendered_docs_gz, rollback_of, idempotency_key, actor, token_id, git_repo, git_ref, git_sha, ci_run_url, created_at, finished_at, result_json, requester_user_id`

func scanStackRevision(r rowScanner, withDocs bool) (StackRevisionRecord, error) {
	var v StackRevisionRecord
	var gz []byte
	var created string
	var finished sql.NullString
	if err := r.Scan(&v.StackID, &v.Number, &v.Status, &v.PlanHash, &v.PlanJSON, &gz, &v.RollbackOf, &v.IdempotencyKey,
		&v.Actor, &v.TokenID, &v.GitRepo, &v.GitRef, &v.GitSHA, &v.CIRunURL, &created, &finished, &v.ResultJSON, &v.RequesterUserID); err != nil {
		return StackRevisionRecord{}, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if finished.Valid {
		t, _ := time.Parse(time.RFC3339Nano, finished.String)
		v.FinishedAt = &t
	}
	if withDocs {
		docs, err := gunzipBytes(gz)
		if err != nil {
			return StackRevisionRecord{}, fmt.Errorf("inflate rendered docs: %w", err)
		}
		v.RenderedDocs = docs
	}
	return v, nil
}

// AppendStackRevision stores the next revision of a stack, numbered from 1 in
// one transaction. It refuses documents or plans holding a secret value. A
// repeated idempotency key returns the earlier revision with Replayed set.
func (db *DB) AppendStackRevision(ctx context.Context, in NewStackRevision, now time.Time) (StackRevisionRecord, error) {
	if in.Status == "" {
		in.Status = StackRevApplying
	}
	if _, ok := stackRevNext[in.Status]; !ok {
		return StackRevisionRecord{}, fmt.Errorf("store: append revision: unknown status %q", in.Status)
	}
	if name, leaked := leakedStackSecret(in.Secrets, in.RenderedDocs, in.PlanJSON); leaked {
		return StackRevisionRecord{}, fmt.Errorf("%w: %s", ErrStackSecretLeak, name)
	}
	gz, err := gzipBytes(in.RenderedDocs)
	if err != nil {
		return StackRevisionRecord{}, fmt.Errorf("store: append revision: compress: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return StackRevisionRecord{}, fmt.Errorf("store: append revision: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if in.IdempotencyKey != "" {
		prev, err := scanStackRevision(tx.QueryRowContext(ctx, `SELECT `+stackRevCols+` FROM stack_revisions WHERE stack_id = ? AND idempotency_key = ?`,
			in.StackID, in.IdempotencyKey), true)
		if err == nil {
			prev.Replayed = true
			return prev, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return StackRevisionRecord{}, fmt.Errorf("store: append revision: idempotency lookup: %w", err)
		}
	}
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM stacks WHERE id = ?`, in.StackID).Scan(&exists); err != nil {
		return StackRevisionRecord{}, fmt.Errorf("store: append revision: stack lookup: %w", err)
	}
	if exists == 0 {
		return StackRevisionRecord{}, ErrStackNotFound
	}
	var number int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(number), 0) + 1 FROM stack_revisions WHERE stack_id = ?`, in.StackID).Scan(&number); err != nil {
		return StackRevisionRecord{}, fmt.Errorf("store: append revision: number: %w", err)
	}
	if in.RollbackOf < 0 || in.RollbackOf >= number {
		return StackRevisionRecord{}, errors.New("store: append revision: rollback_of must name an earlier revision")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO stack_revisions (`+stackRevCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL, ?)`,
		in.StackID, number, in.Status, in.PlanHash, in.PlanJSON, gz, in.RollbackOf, in.IdempotencyKey, in.Actor, in.TokenID,
		in.GitRepo, in.GitRef, in.GitSHA, in.CIRunURL, stackTime(now), in.RequesterUserID); err != nil {
		return StackRevisionRecord{}, fmt.Errorf("store: append revision: insert: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return StackRevisionRecord{}, fmt.Errorf("store: append revision: commit: %w", err)
	}
	return StackRevisionRecord{
		StackID: in.StackID, Number: number, Status: in.Status, PlanHash: in.PlanHash, PlanJSON: in.PlanJSON,
		RenderedDocs: in.RenderedDocs, RollbackOf: in.RollbackOf, IdempotencyKey: in.IdempotencyKey, Actor: in.Actor,
		TokenID: in.TokenID, RequesterUserID: in.RequesterUserID, GitRepo: in.GitRepo, GitRef: in.GitRef, GitSHA: in.GitSHA,
		CIRunURL: in.CIRunURL, CreatedAt: now.UTC(),
	}, nil
}

// SetStackRevisionStatus moves a revision to next if the lifecycle allows it,
// stamping finished_at and result on a terminal status. Reaching applied or
// partial supersedes every older applied or partial revision of the stack.
func (db *DB) SetStackRevisionStatus(ctx context.Context, stackID string, number int, next string, result []byte, now time.Time) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set revision status: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var cur string
	err = tx.QueryRowContext(ctx, `SELECT status FROM stack_revisions WHERE stack_id = ? AND number = ?`, stackID, number).Scan(&cur)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStackRevisionNotFound
	}
	if err != nil {
		return fmt.Errorf("store: set revision status: load: %w", err)
	}
	if !StackRevisionCanTransition(cur, next) {
		return fmt.Errorf("%w: %s to %s", ErrStackBadTransition, cur, next)
	}
	finished := sql.NullString{}
	if next != StackRevApplying {
		finished = sql.NullString{String: stackTime(now), Valid: true}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE stack_revisions SET status = ?, finished_at = COALESCE(?, finished_at),
		result_json = COALESCE(?, result_json) WHERE stack_id = ? AND number = ?`, next, finished, nullBytes(result), stackID, number); err != nil {
		return fmt.Errorf("store: set revision status: update: %w", err)
	}
	if next == StackRevApplied || next == StackRevPartial {
		if _, err := tx.ExecContext(ctx, `UPDATE stack_revisions SET status = ? WHERE stack_id = ? AND number < ? AND status IN (?, ?)`,
			StackRevSuperseded, stackID, number, StackRevApplied, StackRevPartial); err != nil {
			return fmt.Errorf("store: set revision status: supersede: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set revision status: commit: %w", err)
	}
	return nil
}

func nullBytes(b []byte) any {
	if b == nil {
		return nil
	}
	return b
}

// GetStackRevision returns one revision with its inflated documents.
func (db *DB) GetStackRevision(ctx context.Context, stackID string, number int) (StackRevisionRecord, error) {
	v, err := scanStackRevision(db.QueryRowContext(ctx, `SELECT `+stackRevCols+` FROM stack_revisions WHERE stack_id = ? AND number = ?`, stackID, number), true)
	if errors.Is(err, sql.ErrNoRows) {
		return StackRevisionRecord{}, ErrStackRevisionNotFound
	}
	if err != nil {
		return StackRevisionRecord{}, fmt.Errorf("store: get revision: %w", err)
	}
	return v, nil
}

// ListStackRevisions returns revisions newest first without documents, so a
// history page stays small. limit <= 0 means 100.
func (db *DB) ListStackRevisions(ctx context.Context, stackID string, limit int) ([]StackRevisionRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := db.QueryContext(ctx, `SELECT `+stackRevCols+` FROM stack_revisions WHERE stack_id = ? ORDER BY number DESC LIMIT ?`, stackID, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list revisions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []StackRevisionRecord{}
	for rows.Next() {
		v, err := scanStackRevision(rows, false)
		if err != nil {
			return nil, fmt.Errorf("store: scan revision: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list revisions: %w", err)
	}
	return out, nil
}

type stackQueryer interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
}

func listLedger(ctx context.Context, q stackQueryer, stackID string) ([]StackLedgerEntry, error) {
	rows, err := q.QueryContext(ctx, `SELECT stack_id, kind, key, live_id, since_revision FROM stack_resources WHERE stack_id = ? ORDER BY kind, key`, stackID)
	if err != nil {
		return nil, fmt.Errorf("store: list ledger: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []StackLedgerEntry{}
	for rows.Next() {
		var e StackLedgerEntry
		if err := rows.Scan(&e.StackID, &e.Kind, &e.Key, &e.LiveID, &e.SinceRevision); err != nil {
			return nil, fmt.Errorf("store: scan ledger: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list ledger: %w", err)
	}
	return out, nil
}

// ListStackLedger returns the resources a stack owns.
func (db *DB) ListStackLedger(ctx context.Context, stackID string) ([]StackLedgerEntry, error) {
	return listLedger(ctx, db, stackID)
}

// UpsertStackResource records e as owned by its stack. A resource owned by a
// different stack returns ErrStackLedgerConflict; re-claiming one's own
// resource refreshes live_id and keeps the original since_revision.
func (db *DB) UpsertStackResource(ctx context.Context, e StackLedgerEntry) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: upsert ledger: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var owner string
	err = tx.QueryRowContext(ctx, `SELECT stack_id FROM stack_resources WHERE kind = ? AND key = ?`, e.Kind, e.Key).Scan(&owner)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO stack_resources (stack_id, kind, key, live_id, since_revision) VALUES (?, ?, ?, ?, ?)`,
			e.StackID, e.Kind, e.Key, e.LiveID, e.SinceRevision); err != nil {
			return fmt.Errorf("store: upsert ledger: insert: %w", err)
		}
	case err != nil:
		return fmt.Errorf("store: upsert ledger: lookup: %w", err)
	case owner != e.StackID:
		return fmt.Errorf("%w: %s/%s belongs to %s", ErrStackLedgerConflict, e.Kind, e.Key, owner)
	default:
		if _, err := tx.ExecContext(ctx, `UPDATE stack_resources SET live_id = ? WHERE kind = ? AND key = ?`, e.LiveID, e.Kind, e.Key); err != nil {
			return fmt.Errorf("store: upsert ledger: update: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: upsert ledger: commit: %w", err)
	}
	return nil
}

// DeleteStackResource releases one resource from a stack's ledger.
func (db *DB) DeleteStackResource(ctx context.Context, stackID, kind, key string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM stack_resources WHERE stack_id = ? AND kind = ? AND key = ?`, stackID, kind, key); err != nil {
		return fmt.Errorf("store: delete ledger entry: %w", err)
	}
	return nil
}

// DeleteStackLedger releases every resource a stack owns.
func (db *DB) DeleteStackLedger(ctx context.Context, stackID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM stack_resources WHERE stack_id = ?`, stackID); err != nil {
		return fmt.Errorf("store: delete ledger: %w", err)
	}
	return nil
}

// GetStackResourceOwner returns the ledger entry owning kind/key, if any.
func (db *DB) GetStackResourceOwner(ctx context.Context, kind, key string) (StackLedgerEntry, bool, error) {
	var e StackLedgerEntry
	err := db.QueryRowContext(ctx, `SELECT stack_id, kind, key, live_id, since_revision FROM stack_resources WHERE kind = ? AND key = ?`, kind, key).
		Scan(&e.StackID, &e.Kind, &e.Key, &e.LiveID, &e.SinceRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return StackLedgerEntry{}, false, nil
	}
	if err != nil {
		return StackLedgerEntry{}, false, fmt.Errorf("store: ledger owner: %w", err)
	}
	return e, true, nil
}

func scanStackLock(r rowScanner) (StackLockRecord, error) {
	var l StackLockRecord
	var exp string
	if err := r.Scan(&l.StackID, &l.Holder, &l.Revision, &exp); err != nil {
		return StackLockRecord{}, err
	}
	l.ExpiresAt, _ = time.Parse(time.RFC3339Nano, exp)
	return l, nil
}

// GetStackLock returns the current lease, if any (it may be expired).
func (db *DB) GetStackLock(ctx context.Context, stackID string) (StackLockRecord, bool, error) {
	l, err := scanStackLock(db.QueryRowContext(ctx, `SELECT stack_id, holder, revision, expires_at FROM stack_locks WHERE stack_id = ?`, stackID))
	if errors.Is(err, sql.ErrNoRows) {
		return StackLockRecord{}, false, nil
	}
	if err != nil {
		return StackLockRecord{}, false, fmt.Errorf("store: get stack lock: %w", err)
	}
	return l, true, nil
}

// AcquireStackLock takes the lease for holder for ttl. It succeeds when no
// lease exists, the lease is expired, or holder already owns it; otherwise it
// returns ErrStackLocked. The caller supplies ttl (APP_STACK_LOCK_TTL).
func (db *DB) AcquireStackLock(ctx context.Context, stackID, holder string, revision int, ttl time.Duration, now time.Time) (StackLockRecord, error) {
	if holder == "" || ttl <= 0 {
		return StackLockRecord{}, errors.New("store: acquire stack lock: holder and positive ttl are required")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return StackLockRecord{}, fmt.Errorf("store: acquire stack lock: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	cur, err := scanStackLock(tx.QueryRowContext(ctx, `SELECT stack_id, holder, revision, expires_at FROM stack_locks WHERE stack_id = ?`, stackID))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return StackLockRecord{}, fmt.Errorf("store: acquire stack lock: load: %w", err)
	}
	if err == nil && cur.Holder != holder && !cur.Expired(now) {
		return StackLockRecord{}, ErrStackLocked
	}
	l := StackLockRecord{StackID: stackID, Holder: holder, Revision: revision, ExpiresAt: now.Add(ttl).UTC()}
	if _, err := tx.ExecContext(ctx, `INSERT INTO stack_locks (stack_id, holder, revision, expires_at) VALUES (?, ?, ?, ?)
		ON CONFLICT(stack_id) DO UPDATE SET holder = excluded.holder, revision = excluded.revision, expires_at = excluded.expires_at`,
		stackID, holder, revision, stackTime(l.ExpiresAt)); err != nil {
		if strings.Contains(err.Error(), "FOREIGN KEY") {
			return StackLockRecord{}, ErrStackNotFound
		}
		return StackLockRecord{}, fmt.Errorf("store: acquire stack lock: write: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return StackLockRecord{}, fmt.Errorf("store: acquire stack lock: commit: %w", err)
	}
	return l, nil
}

// RenewStackLock extends a lease holder still owns. It returns ErrStackLockLost
// when another holder has taken over or the lease was released.
func (db *DB) RenewStackLock(ctx context.Context, stackID, holder string, ttl time.Duration, now time.Time) (StackLockRecord, error) {
	exp := now.Add(ttl).UTC()
	res, err := db.ExecContext(ctx, `UPDATE stack_locks SET expires_at = ? WHERE stack_id = ? AND holder = ?`, stackTime(exp), stackID, holder)
	if err != nil {
		return StackLockRecord{}, fmt.Errorf("store: renew stack lock: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return StackLockRecord{}, ErrStackLockLost
	}
	l, _, err := db.GetStackLock(ctx, stackID)
	return l, err
}

// ReleaseStackLock drops the lease if holder owns it; releasing a lease one
// no longer owns is a no-op so a late defer cannot free another holder's lock.
func (db *DB) ReleaseStackLock(ctx context.Context, stackID, holder string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM stack_locks WHERE stack_id = ? AND holder = ?`, stackID, holder); err != nil {
		return fmt.Errorf("store: release stack lock: %w", err)
	}
	return nil
}

// ForceReleaseStackLock drops the lease whoever holds it (the unlock endpoint).
func (db *DB) ForceReleaseStackLock(ctx context.Context, stackID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM stack_locks WHERE stack_id = ?`, stackID); err != nil {
		return fmt.Errorf("store: force release stack lock: %w", err)
	}
	return nil
}
