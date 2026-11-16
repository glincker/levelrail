package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Errors returned by the cluster store.
var (
	ErrK8sClusterExists   = errors.New("store: kubernetes cluster already exists")
	ErrK8sClusterNotFound = errors.New("store: kubernetes cluster not found")
)

// K8sClusterSecretsKey is the secrets namespace holding one cluster's credentials.
func K8sClusterSecretsKey(name string) string { return "k8s-cluster/" + name }

// K8sCluster is one connected cluster. It holds no credential material.
type K8sCluster struct {
	Name           string
	Server         string
	ServerName     string
	AuthMode       string
	CAFingerprint  string
	ContextName    string
	CreatedBy      string
	CreatedAt      time.Time
	LastCheckedAt  time.Time
	LastCheckClass string
}

const k8sClusterCols = "name, server, server_name, auth_mode, ca_fingerprint, context_name, created_by, created_at, last_checked_at, last_check_class" //nolint:gosec // column names, not credentials

func scanK8sCluster(sc interface{ Scan(...any) error }) (K8sCluster, error) {
	var c K8sCluster
	var created, checked string
	if err := sc.Scan(&c.Name, &c.Server, &c.ServerName, &c.AuthMode, &c.CAFingerprint, &c.ContextName, &c.CreatedBy, &created, &checked, &c.LastCheckClass); err != nil {
		return K8sCluster{}, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	c.LastCheckedAt, _ = time.Parse(time.RFC3339Nano, checked)
	return c, nil
}

// ListK8sClusters returns every cluster ordered by name.
func (db *DB) ListK8sClusters(ctx context.Context) ([]K8sCluster, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+k8sClusterCols+" FROM k8s_clusters ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("store: list k8s clusters: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []K8sCluster{}
	for rows.Next() {
		c, err := scanK8sCluster(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan k8s cluster: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetK8sCluster returns one cluster or ErrK8sClusterNotFound.
func (db *DB) GetK8sCluster(ctx context.Context, name string) (K8sCluster, error) {
	c, err := scanK8sCluster(db.QueryRowContext(ctx, "SELECT "+k8sClusterCols+" FROM k8s_clusters WHERE name = ?", name))
	if errors.Is(err, sql.ErrNoRows) {
		return K8sCluster{}, ErrK8sClusterNotFound
	}
	if err != nil {
		return K8sCluster{}, fmt.Errorf("store: get k8s cluster: %w", err)
	}
	return c, nil
}

// CreateK8sCluster inserts c, or returns ErrK8sClusterExists.
func (db *DB) CreateK8sCluster(ctx context.Context, c K8sCluster) error {
	_, err := db.ExecContext(ctx, `INSERT INTO k8s_clusters (name, server, server_name, auth_mode, ca_fingerprint, context_name, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name, c.Server, c.ServerName, c.AuthMode, c.CAFingerprint, c.ContextName, c.CreatedBy, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "PRIMARY KEY") {
			return ErrK8sClusterExists
		}
		return fmt.Errorf("store: create k8s cluster: %w", err)
	}
	return nil
}

// SetK8sClusterCheck records the outcome class of the latest connection test.
func (db *DB) SetK8sClusterCheck(ctx context.Context, name, class string, at time.Time) error {
	res, err := db.ExecContext(ctx, `UPDATE k8s_clusters SET last_checked_at = ?, last_check_class = ? WHERE name = ?`,
		at.UTC().Format(time.RFC3339Nano), class, name)
	if err != nil {
		return fmt.Errorf("store: set k8s cluster check: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrK8sClusterNotFound
	}
	return nil
}

// DeleteK8sCluster removes a cluster row; a missing one is ErrK8sClusterNotFound.
func (db *DB) DeleteK8sCluster(ctx context.Context, name string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM k8s_clusters WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store: delete k8s cluster: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrK8sClusterNotFound
	}
	return nil
}
