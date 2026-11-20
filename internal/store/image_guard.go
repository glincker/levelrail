package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrDesiredImageRefused means the image guard refused a save.
var ErrDesiredImageRefused = errors.New("store: image not allowed for this app")

// DesiredImageGuard vets an image before it becomes the desired image of an app.
// A returned error refuses the save.
type DesiredImageGuard func(ctx context.Context, app, image string) error

// SetDesiredImageGuard installs the guard every desired-state write passes through,
// so no deploy path can skip it. Nil removes it.
func (db *DB) SetDesiredImageGuard(g DesiredImageGuard) {
	if g == nil {
		db.imageGuard.Store(nil)
		return
	}
	db.imageGuard.Store(&g)
}

// checkDesiredImage runs the guard only when the save changes the image.
func (db *DB) checkDesiredImage(ctx context.Context, svc DesiredService) error {
	gp := db.imageGuard.Load()
	if gp == nil || svc.Image == "" {
		return nil
	}
	var prev string
	err := db.QueryRowContext(ctx, `SELECT image FROM desired_services WHERE name = ?`, svc.Name).Scan(&prev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("store: read current image of %q: %w", svc.Name, err)
	}
	if err == nil && prev == svc.Image {
		return nil
	}
	if err := (*gp)(ctx, svc.Name, svc.Image); err != nil {
		return fmt.Errorf("%w: %w", ErrDesiredImageRefused, err)
	}
	return nil
}
