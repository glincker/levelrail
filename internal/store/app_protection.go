package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AppProtection is a standing hold on an app (migrations/3219).
type AppProtection struct {
	AppName     string `json:"app_name"`
	Reason      string `json:"reason"`
	ProtectedBy string `json:"protected_by,omitempty"`
	ProtectedAt string `json:"protected_at"`
}

// GetAppProtection returns the app's protection, or nil when it is not protected.
func (db *DB) GetAppProtection(ctx context.Context, app string) (*AppProtection, error) {
	var p AppProtection
	err := db.QueryRowContext(ctx, `SELECT app_name, reason, protected_by, protected_at FROM app_protection WHERE app_name = ?`, app).
		Scan(&p.AppName, &p.Reason, &p.ProtectedBy, &p.ProtectedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get app protection: %w", err)
	}
	return &p, nil
}

// SetAppProtection protects app; protecting twice keeps the first reason and owner.
func (db *DB) SetAppProtection(ctx context.Context, app, reason, by string) error {
	_, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO app_protection (app_name, reason, protected_by, protected_at) VALUES (?, ?, ?, ?)`,
		app, reason, by, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: set app protection: %w", err)
	}
	return nil
}

// ClearAppProtection removes the protection and reports whether one existed.
func (db *DB) ClearAppProtection(ctx context.Context, app string) (bool, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM app_protection WHERE app_name = ?`, app)
	if err != nil {
		return false, fmt.Errorf("store: clear app protection: %w", err)
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
