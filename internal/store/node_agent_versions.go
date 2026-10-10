package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

// NodeAgentVersionChange is one observed change of a node's agent version
// (migrations/0401). FromVersion is empty for the first report.
type NodeAgentVersionChange struct {
	ID          string
	NodeID      string
	NodeName    string
	FromVersion string
	ToVersion   string
	ObservedAt  time.Time
}

func newNodeAgentVersionID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate node agent version id: %w", err)
	}
	return "nav_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// recordNodeAgentVersionChange appends a row when the node's stored agent
// version differs from reported. Missing nodes and unchanged versions write
// nothing. It runs inside UpdateNodeAgentInfo, before the column changes.
func (db *DB) recordNodeAgentVersionChange(ctx context.Context, nodeID, reported string, now time.Time) error {
	if reported == "" {
		return nil
	}
	id, err := newNodeAgentVersionID()
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO node_agent_versions (id, node_id, node_name, from_version, to_version, observed_at)
		SELECT ?, id, name, agent_version, ?, ? FROM nodes WHERE id = ? AND agent_version != ?
	`, id, reported, formatTime(now), nodeID, reported)
	if err != nil {
		return fmt.Errorf("store: record agent version change for node %q: %w", nodeID, err)
	}
	return nil
}

// ListNodeAgentVersionChanges returns recent agent version changes across
// nodes, newest first.
func (db *DB) ListNodeAgentVersionChanges(ctx context.Context, limit int) ([]NodeAgentVersionChange, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, node_id, node_name, from_version, to_version, observed_at
		FROM node_agent_versions ORDER BY seq DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("store: list node agent versions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []NodeAgentVersionChange
	for rows.Next() {
		var c NodeAgentVersionChange
		var observed string
		if err := rows.Scan(&c.ID, &c.NodeID, &c.NodeName, &c.FromVersion, &c.ToVersion, &observed); err != nil {
			return nil, fmt.Errorf("store: scan node agent version: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, observed)
		if err != nil {
			return nil, fmt.Errorf("store: parse node agent version time %q: %w", observed, err)
		}
		c.ObservedAt = t
		out = append(out, c)
	}
	return out, rows.Err()
}
