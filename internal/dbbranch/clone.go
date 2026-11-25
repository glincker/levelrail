package dbbranch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
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
	SetDatabaseBranchClone(ctx context.Context, id, clonePath, reason, now string) error
	SetDatabaseBranchSize(ctx context.Context, id string, bytes int64, now string) error
}

// maxReasonLen bounds the error text stored on a failed branch.
const maxReasonLen = 300

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
	// Container maps a database name to its container name.
	Container func(databaseName string) string
	// Verify is optional and runs after the restore, before the branch becomes ready.
	Verify func(ctx context.Context, engine, containerName string) error
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
	c.fail(ctx, b, cur, err)
	return err
}

func (c *Cloner) run(ctx context.Context, b store.DatabaseBranch, engine string, step func(to, reason string) error) error {
	if err := step(StateCloning, ""); err != nil {
		return err
	}
	if err := c.WaitReady(ctx, b.DatabaseName); err != nil {
		return fmt.Errorf("branch database did not become ready: %w", err)
	}
	dump, consistency, err := c.Dumper.Dump(ctx, engine, c.Container(b.ParentName), b.Mode == ModeSchema)
	if err != nil {
		return fmt.Errorf("dump parent: %w", err)
	}
	defer func() { _ = dump.Close() }()
	if err := c.Restorer.Restore(ctx, engine, c.Container(b.DatabaseName), dump); err != nil {
		return fmt.Errorf("restore into branch: %w", err)
	}
	reason := ""
	if consistency != "" && consistency != "snapshot" {
		reason = "consistency=" + consistency
	}
	if err := c.Store.SetDatabaseBranchClone(ctx, b.ID, "dump", reason, c.now()); err != nil {
		return err
	}
	// The branch database was created with its own generated credentials and the dumps carry no roles,
	// so nothing from the parent needs rotating; the step exists so the lifecycle matches the ADR.
	if err := step(StateCredentials, ""); err != nil {
		return err
	}
	if err := step(StateVerifying, ""); err != nil {
		return err
	}
	if c.Verify != nil {
		if err := c.Verify(ctx, engine, c.Container(b.DatabaseName)); err != nil {
			return fmt.Errorf("verify branch: %w", err)
		}
	}
	c.sample(ctx, b)
	if b.State == StateResetting {
		return Move(ctx, c.Store, b.ID, StateResetting, StateReady, "", c.now())
	}
	return Move(ctx, c.Store, b.ID, StateVerifying, StateReady, "", c.now())
}

func (c *Cloner) sample(ctx context.Context, b store.DatabaseBranch) {
	if c.MeasureSize == nil {
		return
	}
	if n, err := c.MeasureSize(ctx, b.DatabaseName); err == nil {
		_ = c.Store.SetDatabaseBranchSize(ctx, b.ID, n, c.now())
	}
}

// fail marks the row failed with a short reason and removes the half copied database. It uses a context
// that survives the clone timeout, and never logs the dump or any connection detail.
func (c *Cloner) fail(ctx context.Context, b store.DatabaseBranch, cur string, cause error) {
	ctx = context.WithoutCancel(ctx)
	reason := cause.Error()
	if len(reason) > maxReasonLen {
		reason = reason[:maxReasonLen]
	}
	if merr := Move(ctx, c.Store, b.ID, cur, StateFailed, reason, c.now()); merr != nil && !errors.Is(merr, store.ErrDatabaseBranchStateRace) && c.Log != nil {
		c.Log.Warn("dbbranch: mark failed", slog.String("branch", b.DatabaseName), slog.String("error", merr.Error()))
	}
	if c.Teardown != nil {
		if terr := c.Teardown(ctx, b.DatabaseName); terr != nil && c.Log != nil {
			c.Log.Warn("dbbranch: failure cleanup", slog.String("branch", b.DatabaseName), slog.String("error", terr.Error()))
		}
	}
}
