package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrAppStreamNotFound is returned by GetAppStream/DeleteAppStream when id
// doesn't match any row.
var ErrAppStreamNotFound = errors.New("store: app stream not found")

// AppStreamProtocolTCP is the only protocol v1 supports. Stored as a
// string column (migrations/0279) so udp can be added later without a
// schema change.
const AppStreamProtocolTCP = "tcp"

// AppStream is one raw TCP port forward (migrations/0279_app_streams.sql):
// HostPort on this control plane's own host reaches ContainerPort on
// ServiceName's container, proxied by Caddy's layer4 app
// (internal/ingress) rather than a second proxy process.
type AppStream struct {
	ID            string
	ServiceName   string
	ContainerPort int
	HostPort      int
	Protocol      string
	CreatedAt     string
}

// SaveAppStream inserts a new app stream row. IDs are minted by the
// caller before this call, the same "generate before the INSERT" pattern
// FirewallRule uses.
func (db *DB) SaveAppStream(ctx context.Context, s AppStream) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO app_streams (id, service_name, container_port, host_port, protocol, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, s.ID, s.ServiceName, s.ContainerPort, s.HostPort, s.Protocol, s.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save app stream %q: %w", s.ID, err)
	}
	return nil
}

// GetAppStream returns the app stream with this ID, or ErrAppStreamNotFound.
func (db *DB) GetAppStream(ctx context.Context, id string) (AppStream, error) {
	var s AppStream
	err := db.QueryRowContext(ctx, `
		SELECT id, service_name, container_port, host_port, protocol, created_at
		FROM app_streams
		WHERE id = ?
	`, id).Scan(&s.ID, &s.ServiceName, &s.ContainerPort, &s.HostPort, &s.Protocol, &s.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AppStream{}, ErrAppStreamNotFound
	}
	if err != nil {
		return AppStream{}, fmt.Errorf("store: get app stream %q: %w", id, err)
	}
	return s, nil
}

// ListAppStreamsForService returns every stream targeting serviceName,
// oldest first. Used by internal/reconcile/application's container
// reconciler to decide which extra container ports to publish, and by
// GET /api/v1/apps/{name}/streams.
func (db *DB) ListAppStreamsForService(ctx context.Context, serviceName string) ([]AppStream, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, container_port, host_port, protocol, created_at
		FROM app_streams
		WHERE service_name = ?
		ORDER BY created_at
	`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list app streams for service %q: %w", serviceName, err)
	}
	return scanAppStreams(rows)
}

// ListAllAppStreams returns every app stream across every service,
// oldest first. Used by internal/reconcile/ingress to build one Caddy
// layer4 route per stream every reconcile pass, the same "list
// everything fresh, never cache" shape every other ingress input
// already follows.
func (db *DB) ListAllAppStreams(ctx context.Context) ([]AppStream, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, service_name, container_port, host_port, protocol, created_at
		FROM app_streams
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list all app streams: %w", err)
	}
	return scanAppStreams(rows)
}

func scanAppStreams(rows *sql.Rows) ([]AppStream, error) {
	defer func() {
		_ = rows.Close()
	}()

	var out []AppStream
	for rows.Next() {
		var s AppStream
		if err := rows.Scan(&s.ID, &s.ServiceName, &s.ContainerPort, &s.HostPort, &s.Protocol, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan app stream row: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate app stream rows: %w", err)
	}
	return out, nil
}

// DeleteAppStream removes an app stream row. It does not itself touch
// any running container's published ports: the next deploy or restart
// of the owning service picks up the change, the same "store is desired
// state, a fresh container creation converges to it" split
// DeleteFirewallRule's own doc comment describes for ufw. Returns
// ErrAppStreamNotFound if id doesn't exist.
func (db *DB) DeleteAppStream(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM app_streams WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete app stream %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrAppStreamNotFound, "delete app stream %q", id)
}
