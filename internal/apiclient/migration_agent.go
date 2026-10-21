package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// MigrationAgentSession is one read-only migration agent session.
type MigrationAgentSession struct {
	ID               string     `json:"id"`
	Label            string     `json:"label"`
	State            string     `json:"state"`
	TokenExpiresAt   time.Time  `json:"token_expires_at"`
	SessionExpiresAt *time.Time `json:"session_expires_at,omitempty"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty"`
	AgentVersion     string     `json:"agent_version,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// MigrationAgentCreated is the one-time result of creating a session.
type MigrationAgentCreated struct {
	ID               string    `json:"id"`
	Token            string    `json:"token"`
	ExpiresAt        time.Time `json:"expires_at"`
	CAFingerprint    string    `json:"ca_fingerprint"`
	ControlPlaneAddr string    `json:"control_plane_addr,omitempty"`
	Binary           string    `json:"binary"`
	Command          string    `json:"command,omitempty"`
	SessionMaxHours  int       `json:"session_max_hours"`
}

// MigrationAgentMount is a container mount seen on the source.
type MigrationAgentMount struct {
	Type        string `json:"type"`
	Name        string `json:"name,omitempty"`
	Source      string `json:"source,omitempty"`
	Destination string `json:"destination"`
}

// MigrationAgentContainer is one container on the source.
type MigrationAgentContainer struct {
	Name            string                `json:"name"`
	Image           string                `json:"image"`
	State           string                `json:"state"`
	Mounts          []MigrationAgentMount `json:"mounts,omitempty"`
	DatabaseEngine  string                `json:"database_engine,omitempty"`
	DatabaseVersion string                `json:"database_version,omitempty"`
	Unlisted        bool                  `json:"unlisted"`
}

// MigrationAgentInventory is what the agent found on the source.
type MigrationAgentInventory struct {
	DockerVersion string                    `json:"docker_version"`
	Containers    []MigrationAgentContainer `json:"containers"`
	Volumes       []struct {
		Name   string `json:"name"`
		Driver string `json:"driver"`
	} `json:"volumes"`
	NetworkCount int `json:"network_count"`
}

// MigrationAgentDetail is a session with its latest discovery.
type MigrationAgentDetail struct {
	Session   MigrationAgentSession   `json:"session"`
	Inventory MigrationAgentInventory `json:"inventory"`
}

// MigrationAgentComparison is the result for one volume.
type MigrationAgentComparison struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	SourceFiles int64  `json:"source_files"`
	SourceBytes int64  `json:"source_bytes"`
	TargetFiles int64  `json:"target_files"`
	TargetBytes int64  `json:"target_bytes"`
	Status      string `json:"status"`
	TargetFrom  string `json:"target_from,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

// MigrationAgentVerifyTarget is an operator-supplied measurement of a volume on this side.
type MigrationAgentVerifyTarget struct {
	Name  string `json:"name"`
	Files int64  `json:"files"`
	Bytes int64  `json:"bytes"`
}

// MigrationAgentVerifyResult is the outcome of comparing one app's volumes.
type MigrationAgentVerifyResult struct {
	Comparisons []MigrationAgentComparison `json:"comparisons"`
	Advance     struct {
		Advanced bool     `json:"advanced"`
		Reason   string   `json:"reason,omitempty"`
		Evidence []string `json:"evidence,omitempty"`
	} `json:"advance"`
	Databases int                `json:"databases_on_source"`
	Checklist MigrationChecklist `json:"checklist"`
}

const migrationAgentPath = "/api/v1/migration/agent/sessions"

// CreateMigrationAgentSession calls POST /api/v1/migration/agent/sessions.
func (c *Client) CreateMigrationAgentSession(ctx context.Context, label, addr string) (MigrationAgentCreated, error) {
	var out MigrationAgentCreated
	err := c.do(ctx, http.MethodPost, migrationAgentPath, map[string]string{"label": label, "control_plane_addr": addr}, &out)
	return out, err
}

// ListMigrationAgentSessions calls GET /api/v1/migration/agent/sessions.
func (c *Client) ListMigrationAgentSessions(ctx context.Context) ([]MigrationAgentSession, error) {
	var out struct {
		Sessions []MigrationAgentSession `json:"sessions"`
	}
	err := c.do(ctx, http.MethodGet, migrationAgentPath, nil, &out)
	return out.Sessions, err
}

// GetMigrationAgentSession calls GET /api/v1/migration/agent/sessions/{id}.
func (c *Client) GetMigrationAgentSession(ctx context.Context, id string) (MigrationAgentDetail, error) {
	var out MigrationAgentDetail
	err := c.do(ctx, http.MethodGet, migrationAgentPath+"/"+url.PathEscape(id), nil, &out)
	return out, err
}

// DiscoverMigrationAgent calls POST /api/v1/migration/agent/sessions/{id}/discover.
func (c *Client) DiscoverMigrationAgent(ctx context.Context, id string) (MigrationAgentInventory, error) {
	var out MigrationAgentInventory
	err := c.do(ctx, http.MethodPost, migrationAgentPath+"/"+url.PathEscape(id)+"/discover", nil, &out)
	return out, err
}

// VerifyMigrationAgent calls POST /api/v1/migration/agent/sessions/{id}/verify.
func (c *Client) VerifyMigrationAgent(ctx context.Context, id, app string, targets []MigrationAgentVerifyTarget, ackEmpty bool) (MigrationAgentVerifyResult, error) {
	var out MigrationAgentVerifyResult
	body := struct {
		App         string                       `json:"app"`
		TargetStats []MigrationAgentVerifyTarget `json:"target_stats,omitempty"`
		AckEmpty    bool                         `json:"ack_empty_volumes,omitempty"`
	}{app, targets, ackEmpty}
	err := c.do(ctx, http.MethodPost, migrationAgentPath+"/"+url.PathEscape(id)+"/verify", body, &out)
	return out, err
}

// RevokeMigrationAgentSession calls DELETE /api/v1/migration/agent/sessions/{id}.
func (c *Client) RevokeMigrationAgentSession(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, migrationAgentPath+"/"+url.PathEscape(id), nil, nil)
}
