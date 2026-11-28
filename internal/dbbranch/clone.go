package dbbranch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

// Dumper streams a consistent dump of a live parent and labels its consistency.
type Dumper interface {
	Dump(ctx context.Context, engine, containerName string, schemaOnly bool) (io.ReadCloser, string, error)
}

// Restorer applies a dump into the branch container.
type Restorer interface {
	Restore(ctx context.Context, engine, containerName string, dump io.Reader) error
}

// CloneStore is the store surface the clone worker needs.
type CloneStore interface {
	Store
	GetDatabaseBranch(ctx context.Context, name string) (store.DatabaseBranch, error)
	SetDatabaseBranchClone(ctx context.Context, id, clonePath, reason, now string) error
	SetDatabaseBranchSize(ctx context.Context, id string, bytes int64, now string) error
}

// ErrLostOwnership is returned when the branch row no longer belongs to this clone, for example after a delete.
var ErrLostOwnership = errors.New("dbbranch: branch row is no longer owned by this clone")

// ErrDumpTruncated marks a dump stream that errored, was empty or passed the byte cap.
var ErrDumpTruncated = errors.New("dbbranch: dump stream failed or was cut short")

// stepError names the step that failed. Only the step and engine reach the stored reason, because engine
// output can carry row data; the full error goes to the debug log.
type stepError struct {
	step string
	err  error
}

func (e *stepError) Error() string { return "clone failed at step " + e.step }
func (e *stepError) Unwrap() error { return e.err }

// trackedReader remembers the first stream error, which the exec stdin copy would otherwise drop, and enforces a byte cap.
type trackedReader struct {
	r   io.Reader
	n   int64
	max int64
	err error
}

func (t *trackedReader) Read(p []byte) (int, error) {
	if t.max > 0 && t.n > t.max {
		t.err = ErrDumpTruncated
		return 0, t.err
	}
	n, err := t.r.Read(p)
	t.n += int64(n)
	if err != nil && !errors.Is(err, io.EOF) {
		t.err = err
	}
	return n, err
}

// Cloner runs the slow path: dump the parent and restore into the branch's own new database.
// The branch database is created by the caller with no published port and no links, so it is
// unreachable except by container name until the row is ready.
type Cloner struct {
	Store    CloneStore
	Dumper   Dumper
	Restorer Restorer
	// WaitReady blocks until the branch database reports ready.
	WaitReady func(ctx context.Context, databaseName string) error
	// Teardown removes the branch container and volume after a failure.
	Teardown func(ctx context.Context, databaseName string) error
	// Cleanup, when set, runs instead of Teardown after a failure: it removes the container and volume
	// but keeps the database and branch rows, so the reason stays visible until failed_retention.
	Cleanup func(ctx context.Context, databaseName string) error
	// Container maps a database name to its container name.
	Container func(databaseName string) string
	// Count returns the table or key count of a container, used to compare branch with parent before ready.
	Count func(ctx context.Context, engine, containerName string) (int64, error)
	// MaxBytes caps the dump stream; zero or nil means no cap.
	MaxBytes func(ctx context.Context) int64
	// MeasureSize is optional and reports the branch volume use in bytes.
	MeasureSize func(ctx context.Context, databaseName string) (int64, error)
	Timeout     func(ctx context.Context) time.Duration
	Now         func() time.Time
	Log         *slog.Logger
}

func (c *Cloner) now() string {
	if c.Now != nil {
		return c.Now().UTC().Format(time.RFC3339)
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func (c *Cloner) owns(ctx context.Context, b store.DatabaseBranch) error {
	cur, err := c.Store.GetDatabaseBranch(ctx, b.DatabaseName)
	if err != nil || cur.ID != b.ID {
		return ErrLostOwnership
	}
	return nil
}

// Run clones into branch b, whose row is requested (new) or resetting (reset). It records the outcome in the row
// and returns the error it recorded. State moves go only through Move.
func (c *Cloner) Run(ctx context.Context, b store.DatabaseBranch, engine string) error {
	if c.Timeout != nil {
		if d := c.Timeout(ctx); d > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}
	}
	cur := b.State
	step := func(to, reason string) error {
		if cur == StateResetting {
			return nil
		}
		if err := Move(ctx, c.Store, b.ID, cur, to, reason, c.now()); err != nil {
			return err
		}
		cur = to
		return nil
	}
	err := c.run(ctx, b, engine, step)
	if err == nil {
		return nil
	}
	if c.Log != nil {
		c.Log.Debug("dbbranch: clone step failed", slog.String("branch", b.DatabaseName), slog.String("error", err.Error()))
	}
	var se *stepError
	stepName := "state"
	if errors.As(err, &se) {
		stepName = se.step
	}
	c.fail(ctx, b, cur, engine, stepName)
	return &stepError{step: stepName, err: err}
}

func (c *Cloner) run(ctx context.Context, b store.DatabaseBranch, engine string, step func(to, reason string) error) error {
	fail := func(name string, err error) error { return &stepError{step: name, err: err} }
	if err := step(StateCloning, ""); err != nil {
		return fail("state", err)
	}
	if err := c.WaitReady(ctx, b.DatabaseName); err != nil {
		return fail("wait_ready", err)
	}
	if err := c.owns(ctx, b); err != nil {
		return fail("ownership", err)
	}
	dump, consistency, err := c.Dumper.Dump(ctx, engine, c.Container(b.ParentName), b.Mode == ModeSchema)
	if err != nil {
		return fail("dump", err)
	}
	defer func() { _ = dump.Close() }()
	tr := &trackedReader{r: dump}
	if c.MaxBytes != nil {
		tr.max = c.MaxBytes(ctx)
	}
	if err := c.owns(ctx, b); err != nil {
		return fail("ownership", err)
	}
	if err := c.Restorer.Restore(ctx, engine, c.Container(b.DatabaseName), tr); err != nil {
		return fail("restore", err)
	}
	// The restore tool may stop reading early or hide a failed stdin copy, so read to the end here.
	_, _ = io.Copy(io.Discard, tr)
	if tr.err != nil || tr.n == 0 {
		return fail("dump", errors.Join(ErrDumpTruncated, tr.err))
	}
	reason := ""
	if consistency != "" && consistency != "snapshot" {
		reason = "consistency=" + consistency
	}
	if err := c.Store.SetDatabaseBranchClone(ctx, b.ID, "dump", reason, c.now()); err != nil {
		return fail("state", err)
	}
	// The branch database was created with its own generated credentials and the dumps carry no roles,
	// so nothing from the parent needs rotating; the step exists so the lifecycle matches the ADR.
	if err := step(StateCredentials, ""); err != nil {
		return fail("state", err)
	}
	if err := step(StateVerifying, ""); err != nil {
		return fail("state", err)
	}
	if err := c.verify(ctx, b, engine); err != nil {
		return fail("verify", err)
	}
	c.sample(ctx, b)
	if b.State == StateResetting {
		return Move(ctx, c.Store, b.ID, StateResetting, StateReady, "", c.now())
	}
	return Move(ctx, c.Store, b.ID, StateVerifying, StateReady, "", c.now())
}

// verify refuses a branch that came out empty when its parent has content.
func (c *Cloner) verify(ctx context.Context, b store.DatabaseBranch, engine string) error {
	if c.Count == nil {
		return nil
	}
	parent, err := c.Count(ctx, engine, c.Container(b.ParentName))
	if err != nil {
		return err
	}
	got, err := c.Count(ctx, engine, c.Container(b.DatabaseName))
	if err != nil {
		return err
	}
	if parent > 0 && got == 0 {
		return fmt.Errorf("branch is empty but the parent has %d items", parent)
	}
	return nil
}

func (c *Cloner) sample(ctx context.Context, b store.DatabaseBranch) {
	if c.MeasureSize == nil {
		return
	}
	if n, err := c.MeasureSize(ctx, b.DatabaseName); err == nil {
		_ = c.Store.SetDatabaseBranchSize(ctx, b.ID, n, c.now())
	}
}

// fail marks the row failed with a fixed reason code and removes the half copied database, but only while this
// clone still owns the row, so a deleted or replaced branch name is never torn down by a stale worker.
func (c *Cloner) fail(ctx context.Context, b store.DatabaseBranch, cur, engine, step string) {
	ctx = context.WithoutCancel(ctx)
	reason := fmt.Sprintf("clone failed at step %s (%s)", step, engine)
	if err := Move(ctx, c.Store, b.ID, cur, StateFailed, reason, c.now()); err != nil {
		if c.Log != nil && !errors.Is(err, store.ErrDatabaseBranchStateRace) {
			c.Log.Warn("dbbranch: mark failed", slog.String("branch", b.DatabaseName), slog.String("error", err.Error()))
		}
		return
	}
	cleanup := c.Teardown
	if c.Cleanup != nil {
		cleanup = c.Cleanup
	}
	if cleanup != nil {
		if terr := cleanup(ctx, b.DatabaseName); terr != nil && c.Log != nil {
			c.Log.Warn("dbbranch: failure cleanup", slog.String("branch", b.DatabaseName), slog.String("error", terr.Error()))
		}
	}
}
