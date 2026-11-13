package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Errors returned by the cloud state store.
var (
	ErrCloudStateNotFound = errors.New("store: cloud state not found")
	ErrCloudStateConflict = errors.New("store: cloud state version changed")
	ErrCloudStateLocked   = errors.New("store: cloud state is locked")
	ErrCloudStateLockLost = errors.New("store: cloud state lock is not held")
	ErrCloudStateLockLive = errors.New("store: cloud state lock has not expired")
	ErrCloudStateBadInput = errors.New("store: cloud state input is invalid")
)

// CloudState is one sealed state record. Sealed is ciphertext only; nothing here is ever served by an API.
type CloudState struct {
	Account   string
	Stack     string
	Version   int64
	Fence     int64
	Sealed    []byte
	UpdatedAt time.Time
}

// CloudStateLock is a lease on one account and stack. Fence grows on every acquire and steal.
type CloudStateLock struct {
	Account   string
	Stack     string
	Holder    string
	Fence     int64
	ExpiresAt time.Time
}

type lockRow struct {
	holder  string
	fence   int64
	expires time.Time
}

func cloudStateKeyValid(account, stack string) bool { return account != "" && stack != "" }

func fmtLockTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func readLockRow(ctx context.Context, tx *sql.Tx, account, stack string) (lockRow, bool, error) {
	var r lockRow
	var exp string
	err := tx.QueryRowContext(ctx, "SELECT holder, fence, expires_at FROM cloud_state_locks WHERE account = ? AND stack = ?", account, stack).Scan(&r.holder, &r.fence, &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return lockRow{}, false, nil
	}
	if err != nil {
		return lockRow{}, false, fmt.Errorf("store: read cloud state lock: %w", err)
	}
	r.expires, _ = time.Parse(time.RFC3339Nano, exp)
	return r, true, nil
}

func (r lockRow) live(now time.Time) bool { return r.holder != "" && now.Before(r.expires) }

// cloudStateTx runs fn in a transaction and commits only when fn returns nil.
func (db *DB) cloudStateTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin cloud state tx: %w", err)
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// AcquireCloudStateLock takes a free lock. A held lock, even an expired one, is only taken by StealCloudStateLock.
func (db *DB) AcquireCloudStateLock(ctx context.Context, account, stack, holder string, ttl time.Duration, now time.Time) (CloudStateLock, error) {
	if !cloudStateKeyValid(account, stack) || holder == "" || ttl <= 0 {
		return CloudStateLock{}, ErrCloudStateBadInput
	}
	var out CloudStateLock
	err := db.cloudStateTx(ctx, func(tx *sql.Tx) error {
		row, ok, err := readLockRow(ctx, tx, account, stack)
		if err != nil {
			return err
		}
		if ok && row.holder != "" {
			return ErrCloudStateLocked
		}
		out = CloudStateLock{Account: account, Stack: stack, Holder: holder, Fence: row.fence + 1, ExpiresAt: now.Add(ttl)}
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_state_locks (account, stack, holder, fence, acquired_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (account, stack) DO UPDATE SET holder = excluded.holder, fence = excluded.fence, acquired_at = excluded.acquired_at, expires_at = excluded.expires_at`,
			account, stack, holder, out.Fence, fmtLockTime(now), fmtLockTime(out.ExpiresAt))
		if err != nil {
			return fmt.Errorf("store: acquire cloud state lock: %w", err)
		}
		return nil
	})
	return out, err
}

// RenewCloudStateLock extends a live lease. A lease that already expired is lost, so a stealer can rely on expiry.
func (db *DB) RenewCloudStateLock(ctx context.Context, lock CloudStateLock, ttl time.Duration, now time.Time) (CloudStateLock, error) {
	if ttl <= 0 {
		return CloudStateLock{}, ErrCloudStateBadInput
	}
	err := db.cloudStateTx(ctx, func(tx *sql.Tx) error {
		row, ok, err := readLockRow(ctx, tx, lock.Account, lock.Stack)
		if err != nil {
			return err
		}
		if !ok || !row.live(now) || row.holder != lock.Holder || row.fence != lock.Fence {
			return ErrCloudStateLockLost
		}
		lock.ExpiresAt = now.Add(ttl)
		_, err = tx.ExecContext(ctx, "UPDATE cloud_state_locks SET expires_at = ? WHERE account = ? AND stack = ?", fmtLockTime(lock.ExpiresAt), lock.Account, lock.Stack)
		return err
	})
	if err != nil {
		return CloudStateLock{}, err
	}
	return lock, nil
}

// ReleaseCloudStateLock frees the lock if this lease still owns it. The fence stays so the next holder gets a larger one.
func (db *DB) ReleaseCloudStateLock(ctx context.Context, lock CloudStateLock) error {
	res, err := db.ExecContext(ctx, "UPDATE cloud_state_locks SET holder = '', expires_at = '' WHERE account = ? AND stack = ? AND holder = ? AND fence = ?",
		lock.Account, lock.Stack, lock.Holder, lock.Fence)
	if err != nil {
		return fmt.Errorf("store: release cloud state lock: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCloudStateLockLost
	}
	return nil
}

// StealCloudStateLock takes an expired lease, bumps the fence and records the steal.
func (db *DB) StealCloudStateLock(ctx context.Context, account, stack, holder string, ttl time.Duration, now time.Time) (CloudStateLock, error) {
	if !cloudStateKeyValid(account, stack) || holder == "" || ttl <= 0 {
		return CloudStateLock{}, ErrCloudStateBadInput
	}
	var out CloudStateLock
	err := db.cloudStateTx(ctx, func(tx *sql.Tx) error {
		row, ok, err := readLockRow(ctx, tx, account, stack)
		if err != nil {
			return err
		}
		if !ok || row.holder == "" {
			return ErrCloudStateLockLost
		}
		if row.live(now) {
			return ErrCloudStateLockLive
		}
		out = CloudStateLock{Account: account, Stack: stack, Holder: holder, Fence: row.fence + 1, ExpiresAt: now.Add(ttl)}
		if _, err := tx.ExecContext(ctx, "UPDATE cloud_state_locks SET holder = ?, fence = ?, acquired_at = ?, expires_at = ? WHERE account = ? AND stack = ?",
			holder, out.Fence, fmtLockTime(now), fmtLockTime(out.ExpiresAt), account, stack); err != nil {
			return fmt.Errorf("store: steal cloud state lock: %w", err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_state_lock_steals (account, stack, previous_holder, previous_fence, new_holder, new_fence, stolen_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, account, stack, row.holder, row.fence, holder, out.Fence, fmtLockTime(now))
		return err
	})
	return out, err
}

// CountCloudStateLockSteals reports how many times the lock of one stack was stolen.
func (db *DB) CountCloudStateLockSteals(ctx context.Context, account, stack string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM cloud_state_lock_steals WHERE account = ? AND stack = ?", account, stack).Scan(&n)
	return n, err
}

// GetCloudState returns the sealed record or ErrCloudStateNotFound.
func (db *DB) GetCloudState(ctx context.Context, account, stack string) (CloudState, error) {
	s := CloudState{Account: account, Stack: stack}
	var updated string
	err := db.QueryRowContext(ctx, "SELECT version, fence, sealed, updated_at FROM cloud_states WHERE account = ? AND stack = ?", account, stack).
		Scan(&s.Version, &s.Fence, &s.Sealed, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudState{}, ErrCloudStateNotFound
	}
	if err != nil {
		return CloudState{}, fmt.Errorf("store: get cloud state: %w", err)
	}
	s.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return s, nil
}

// PutCloudState creates (expectVersion 0) or replaces the sealed record. It needs the live lease
// and the version the caller read, and returns the new version.
func (db *DB) PutCloudState(ctx context.Context, lock CloudStateLock, expectVersion int64, sealed []byte, now time.Time) (int64, error) {
	if !cloudStateKeyValid(lock.Account, lock.Stack) || len(sealed) == 0 || expectVersion < 0 {
		return 0, ErrCloudStateBadInput
	}
	next := expectVersion + 1
	err := db.cloudStateTx(ctx, func(tx *sql.Tx) error {
		row, ok, err := readLockRow(ctx, tx, lock.Account, lock.Stack)
		if err != nil {
			return err
		}
		if !ok || !row.live(now) || row.holder != lock.Holder || row.fence != lock.Fence {
			return ErrCloudStateLockLost
		}
		var cur, curFence int64
		err = tx.QueryRowContext(ctx, "SELECT version, fence FROM cloud_states WHERE account = ? AND stack = ?", lock.Account, lock.Stack).Scan(&cur, &curFence)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			cur = 0
		case err != nil:
			return fmt.Errorf("store: read cloud state version: %w", err)
		}
		// A record written under a newer fence means this lease is stale even if the lock row looks right.
		if cur != expectVersion || curFence > lock.Fence {
			return ErrCloudStateConflict
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO cloud_states (account, stack, version, fence, sealed, updated_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (account, stack) DO UPDATE SET version = excluded.version, fence = excluded.fence, sealed = excluded.sealed, updated_at = excluded.updated_at`,
			lock.Account, lock.Stack, next, lock.Fence, sealed, fmtLockTime(now))
		if err != nil {
			return fmt.Errorf("store: put cloud state: %w", err)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return next, nil
}

// ErrCloudPlanNotFound means the stack has no saved plan.
var ErrCloudPlanNotFound = errors.New("store: cloud plan not found")

// CloudPlan is the saved plan of one stack. Sealed is ciphertext; Hash is the digest of the plan bytes.
type CloudPlan struct {
	Account   string
	Stack     string
	Hash      string
	Sealed    []byte
	CreatedAt time.Time
}

// PutCloudPlan replaces the saved plan of a stack, so only the newest plan can ever be applied.
func (db *DB) PutCloudPlan(ctx context.Context, p CloudPlan) error {
	if !cloudStateKeyValid(p.Account, p.Stack) || p.Hash == "" || len(p.Sealed) == 0 {
		return ErrCloudStateBadInput
	}
	_, err := db.ExecContext(ctx, `INSERT INTO cloud_plans (account, stack, hash, sealed, created_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (account, stack) DO UPDATE SET hash = excluded.hash, sealed = excluded.sealed, created_at = excluded.created_at`,
		p.Account, p.Stack, p.Hash, p.Sealed, fmtLockTime(p.CreatedAt))
	if err != nil {
		return fmt.Errorf("store: put cloud plan: %w", err)
	}
	return nil
}

// GetCloudPlan returns the saved plan or ErrCloudPlanNotFound.
func (db *DB) GetCloudPlan(ctx context.Context, account, stack string) (CloudPlan, error) {
	p := CloudPlan{Account: account, Stack: stack}
	var created string
	err := db.QueryRowContext(ctx, "SELECT hash, sealed, created_at FROM cloud_plans WHERE account = ? AND stack = ?", account, stack).
		Scan(&p.Hash, &p.Sealed, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudPlan{}, ErrCloudPlanNotFound
	}
	if err != nil {
		return CloudPlan{}, fmt.Errorf("store: get cloud plan: %w", err)
	}
	p.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return CloudPlan{}, fmt.Errorf("store: cloud plan time: %w", err)
	}
	return p, nil
}

// DeleteCloudPlan removes the saved plan of a stack only if it still has this hash.
func (db *DB) DeleteCloudPlan(ctx context.Context, account, stack, hash string) error {
	_, err := db.ExecContext(ctx, "DELETE FROM cloud_plans WHERE account = ? AND stack = ? AND hash = ?", account, stack, hash)
	if err != nil {
		return fmt.Errorf("store: delete cloud plan: %w", err)
	}
	return nil
}
