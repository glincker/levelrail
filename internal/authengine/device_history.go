package authengine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	theauth "github.com/glincker/theauth-go/v2"

	"github.com/GLINCKER/levelrail/internal/store"
)

// DeviceRequest is one device login request in any state. It never carries
// the user code or the device code.
type DeviceRequest struct {
	ID          string
	ClientName  string
	RequesterIP string
	UserAgent   string
	State       string
	CreatedAt   time.Time
	ExpiresAt   time.Time
}

const deviceRequestColumns = `id, status, client_name, requester_ip, requester_ua, created_at, expires_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanDeviceRequest(r rowScanner, now time.Time) (DeviceRequest, error) {
	var (
		d                        DeviceRequest
		status                   string
		createdMicro, expiresMic int64
	)
	if err := r.Scan(&d.ID, &status, &d.ClientName, &d.RequesterIP, &d.UserAgent, &createdMicro, &expiresMic); err != nil {
		return DeviceRequest{}, fmt.Errorf("authengine: scan device request: %w", err)
	}
	d.CreatedAt = time.UnixMicro(createdMicro).UTC()
	d.ExpiresAt = time.UnixMicro(expiresMic).UTC()
	d.State = deviceRequestState(status, d.ExpiresAt, now)
	return d, nil
}

func deviceRequestState(status string, expiresAt, now time.Time) string {
	switch status {
	case theauth.DeviceStatusDenied:
		return store.DeviceLoginStateDenied
	case theauth.DeviceStatusApproved, theauth.DeviceStatusRedeemed:
		return store.DeviceLoginStateApproved
	}
	if expiresAt.After(now) {
		return store.DeviceLoginStateWaiting
	}
	return store.DeviceLoginStateExpired
}

// RecentDevices lists requests created at or after since, newest first,
// with their derived state.
func (e *Engine) RecentDevices(ctx context.Context, since, now time.Time, limit int) ([]DeviceRequest, error) {
	rows, err := e.db.QueryContext(ctx, `SELECT `+deviceRequestColumns+` FROM theauth_device_codes
		WHERE created_at >= ? ORDER BY created_at DESC, id DESC LIMIT ?`, since.UTC().UnixMicro(), limit)
	if err != nil {
		return nil, fmt.Errorf("authengine: list recent device requests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeviceRequest
	for rows.Next() {
		d, err := scanDeviceRequest(rows, now)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("authengine: iterate recent device requests: %w", err)
	}
	return out, nil
}

// LapsedDevices lists requests still pending past their expiry whose expiry
// has not been recorded yet, oldest first.
func (e *Engine) LapsedDevices(ctx context.Context, now time.Time, limit int) ([]DeviceRequest, error) {
	rows, err := e.db.QueryContext(ctx, `SELECT `+deviceRequestColumns+` FROM theauth_device_codes c
		WHERE c.status = ? AND c.expires_at <= ?
		AND NOT EXISTS (SELECT 1 FROM device_login_expiries x WHERE x.request_id = c.id)
		ORDER BY c.expires_at ASC LIMIT ?`, theauth.DeviceStatusPending, now.UTC().UnixMicro(), limit)
	if err != nil {
		return nil, fmt.Errorf("authengine: list lapsed device requests: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeviceRequest
	for rows.Next() {
		d, err := scanDeviceRequest(rows, now)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("authengine: iterate lapsed device requests: %w", err)
	}
	return out, nil
}

// DeviceRequestByCode finds a request by its user code, or reports
// ErrDeviceNotFound.
func (e *Engine) DeviceRequestByCode(ctx context.Context, userCode string, now time.Time) (DeviceRequest, error) {
	row := e.db.QueryRowContext(ctx, `SELECT `+deviceRequestColumns+` FROM theauth_device_codes WHERE user_code = ?`, normalizeCode(userCode))
	d, err := scanDeviceRequest(row, now)
	if err != nil {
		return DeviceRequest{}, ErrDeviceNotFound
	}
	return d, nil
}

// DeviceRequestByID finds a request by its row id, or reports ErrDeviceNotFound.
func (e *Engine) DeviceRequestByID(ctx context.Context, id string, now time.Time) (DeviceRequest, error) {
	row := e.db.QueryRowContext(ctx, `SELECT `+deviceRequestColumns+` FROM theauth_device_codes WHERE id = ?`, id)
	d, err := scanDeviceRequest(row, now)
	if err != nil {
		return DeviceRequest{}, ErrDeviceNotFound
	}
	return d, nil
}

// DeviceRequestByDeviceCode finds a request by the CLI's device code.
func (e *Engine) DeviceRequestByDeviceCode(ctx context.Context, deviceCode string, now time.Time) (DeviceRequest, error) {
	hash := sha256.Sum256([]byte(deviceCode))
	row := e.db.QueryRowContext(ctx, `SELECT `+deviceRequestColumns+` FROM theauth_device_codes WHERE device_code_hash = ?`, hash[:])
	d, err := scanDeviceRequest(row, now)
	if err != nil {
		return DeviceRequest{}, ErrDeviceNotFound
	}
	return d, nil
}

// PruneDevices deletes requests that expired before the cutoff.
func (e *Engine) PruneDevices(ctx context.Context, before time.Time) (int, error) {
	n, err := e.store.DeleteExpiredDeviceCodes(ctx, before)
	if err != nil {
		return 0, fmt.Errorf("authengine: prune device requests: %w", err)
	}
	return n, nil
}
