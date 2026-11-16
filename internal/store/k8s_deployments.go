package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrK8sDeploymentNotFound is returned when no deployment row matches.
var ErrK8sDeploymentNotFound = errors.New("store: kubernetes deployment not found")

// K8sDeployment records one app applied to one cluster. It holds no env values.
type K8sDeployment struct {
	Cluster   string
	App       string
	Namespace string
	Image     string
	Port      int
	Replicas  int
	Host      string
	AllowRoot bool
	Objects   []string
	CreatedBy string
	CreatedAt time.Time
	UpdatedAt time.Time
}

const k8sDeploymentCols = "cluster_name, app, namespace, image, port, replicas, host, allow_root, objects, created_by, created_at, updated_at"

func scanK8sDeployment(sc interface{ Scan(...any) error }) (K8sDeployment, error) {
	var d K8sDeployment
	var root int
	var objs, created, updated string
	if err := sc.Scan(&d.Cluster, &d.App, &d.Namespace, &d.Image, &d.Port, &d.Replicas, &d.Host, &root, &objs, &d.CreatedBy, &created, &updated); err != nil {
		return K8sDeployment{}, err
	}
	d.AllowRoot = root != 0
	_ = json.Unmarshal([]byte(objs), &d.Objects)
	d.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	d.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return d, nil
}

// UpsertK8sDeployment records an apply. created_at and created_by survive a re-apply.
func (db *DB) UpsertK8sDeployment(ctx context.Context, d K8sDeployment) error {
	objs, err := json.Marshal(d.Objects)
	if err != nil {
		return err
	}
	root := 0
	if d.AllowRoot {
		root = 1
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = db.ExecContext(ctx, `INSERT INTO k8s_deployments (`+k8sDeploymentCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(cluster_name, app) DO UPDATE SET namespace = excluded.namespace, image = excluded.image, port = excluded.port,
replicas = excluded.replicas, host = excluded.host, allow_root = excluded.allow_root, objects = excluded.objects, updated_at = excluded.updated_at`,
		d.Cluster, d.App, d.Namespace, d.Image, d.Port, d.Replicas, d.Host, root, string(objs), d.CreatedBy, now, now)
	if err != nil {
		return fmt.Errorf("store: upsert k8s deployment: %w", err)
	}
	return nil
}

// GetK8sDeployment returns one deployment or ErrK8sDeploymentNotFound.
func (db *DB) GetK8sDeployment(ctx context.Context, cluster, app string) (K8sDeployment, error) {
	d, err := scanK8sDeployment(db.QueryRowContext(ctx, "SELECT "+k8sDeploymentCols+" FROM k8s_deployments WHERE cluster_name = ? AND app = ?", cluster, app))
	if errors.Is(err, sql.ErrNoRows) {
		return K8sDeployment{}, ErrK8sDeploymentNotFound
	}
	if err != nil {
		return K8sDeployment{}, fmt.Errorf("store: get k8s deployment: %w", err)
	}
	return d, nil
}

// ListK8sDeployments returns a cluster's deployments ordered by app.
func (db *DB) ListK8sDeployments(ctx context.Context, cluster string) ([]K8sDeployment, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+k8sDeploymentCols+" FROM k8s_deployments WHERE cluster_name = ? ORDER BY app", cluster)
	if err != nil {
		return nil, fmt.Errorf("store: list k8s deployments: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []K8sDeployment{}
	for rows.Next() {
		d, err := scanK8sDeployment(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan k8s deployment: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteK8sDeploymentsForCluster drops the records when a cluster is disconnected.
func (db *DB) DeleteK8sDeploymentsForCluster(ctx context.Context, cluster string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM k8s_deployments WHERE cluster_name = ?`, cluster); err != nil {
		return fmt.Errorf("store: delete k8s deployments: %w", err)
	}
	return nil
}
