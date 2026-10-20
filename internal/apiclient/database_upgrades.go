package apiclient

import (
	"context"
	"net/http"
)

// DBUpgradeTarget is one version the advisor offers.
type DBUpgradeTarget struct {
	Version    string   `json:"version"`
	Kind       string   `json:"kind"`
	Security   bool     `json:"security"`
	Advisories []string `json:"advisories,omitempty"`
	EOL        string   `json:"eol,omitempty"`
	Automatic  bool     `json:"automatic"`
}

// DBUpgradeAdvice is the advisor's view of a database's version.
type DBUpgradeAdvice struct {
	Engine         string            `json:"engine"`
	Current        string            `json:"current"`
	Comparable     bool              `json:"comparable"`
	Floating       bool              `json:"floating"`
	Line           string            `json:"line,omitempty"`
	EOL            string            `json:"eol,omitempty"`
	Support        string            `json:"support"`
	Advisories     []string          `json:"advisories,omitempty"`
	Targets        []DBUpgradeTarget `json:"targets"`
	AutoMax        string            `json:"auto_max"`
	ImageRevert    bool              `json:"image_revert"`
	ManualReason   string            `json:"manual_reason,omitempty"`
	Notes          string            `json:"notes,omitempty"`
	Note           string            `json:"note,omitempty"`
	CatalogUpdated string            `json:"catalog_updated"`
}

// DBUpgradePolicy is a database's or the platform default's upgrade policy.
type DBUpgradePolicy struct {
	Inherit               bool     `json:"inherit,omitempty"`
	AutoUpgrade           string   `json:"auto_upgrade"`
	WindowCron            string   `json:"window_cron"`
	WindowDurationSeconds int64    `json:"window_duration_seconds"`
	WindowTimezone        string   `json:"window_timezone"`
	BackupBefore          bool     `json:"backup_before"`
	VerifyAfter           bool     `json:"verify_after"`
	RevertOnFailure       bool     `json:"revert_on_failure"`
	Notify                []string `json:"notify"`
	Inherited             bool     `json:"inherited,omitempty"`
}

// DBUpgradeRun is one upgrade attempt.
type DBUpgradeRun struct {
	ID              string            `json:"id"`
	DatabaseName    string            `json:"database_name"`
	Engine          string            `json:"engine"`
	FromVersion     string            `json:"from_version"`
	ToVersion       string            `json:"to_version"`
	Kind            string            `json:"kind"`
	Source          string            `json:"source"`
	State           string            `json:"state"`
	Phase           string            `json:"phase,omitempty"`
	VerifyAfter     bool              `json:"verify_after"`
	RevertOnFailure bool              `json:"revert_on_failure"`
	BackupID        string            `json:"backup_id,omitempty"`
	VerificationID  string            `json:"verification_id,omitempty"`
	FromImageDigest string            `json:"from_image_digest,omitempty"`
	SnapshotVolume  string            `json:"snapshot_volume,omitempty"`
	RevertPath      string            `json:"revert_path,omitempty"`
	Reason          string            `json:"reason,omitempty"`
	RequestedBy     string            `json:"requested_by,omitempty"`
	Timings         map[string]string `json:"timings"`
	CreatedAt       string            `json:"created_at"`
	FinishedAt      string            `json:"finished_at,omitempty"`
}

// DBUpgradesResource is GET /api/v1/databases/{name}/upgrades.
type DBUpgradesResource struct {
	Database   string           `json:"database"`
	Engine     string           `json:"engine"`
	Version    string           `json:"version"`
	Advice     DBUpgradeAdvice  `json:"advice"`
	Policy     DBUpgradePolicy  `json:"policy"`
	WindowOpen bool             `json:"window_open"`
	NextWindow string           `json:"next_window,omitempty"`
	NextTarget *DBUpgradeTarget `json:"next_target,omitempty"`
	Blockers   []string         `json:"blockers"`
	Active     *DBUpgradeRun    `json:"active,omitempty"`
	History    []DBUpgradeRun   `json:"history"`
}

// DBUpgradeSummaryItem is one database in the upgrade summary.
type DBUpgradeSummaryItem struct {
	Database    string   `json:"database"`
	Engine      string   `json:"engine"`
	Version     string   `json:"version"`
	Support     string   `json:"support"`
	EOL         string   `json:"eol,omitempty"`
	Security    bool     `json:"security"`
	Advisories  []string `json:"advisories,omitempty"`
	Available   int      `json:"available"`
	ActiveState string   `json:"active_state,omitempty"`
	LastState   string   `json:"last_state,omitempty"`
	LastReason  string   `json:"last_reason,omitempty"`
}

// DBUpgradeSummary is GET /api/v1/databases/upgrade-summary.
type DBUpgradeSummary struct {
	Items         []DBUpgradeSummaryItem `json:"items"`
	SecurityCount int                    `json:"security_count"`
	EOLCount      int                    `json:"eol_count"`
}

// GetDatabaseUpgrades calls GET /api/v1/databases/{name}/upgrades.
func (c *Client) GetDatabaseUpgrades(ctx context.Context, name string) (DBUpgradesResource, error) {
	var out DBUpgradesResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name)+"/upgrades", nil, &out)
	return out, err
}

// SetDatabaseUpgradePolicy calls PUT /api/v1/databases/{name}/upgrade-policy.
func (c *Client) SetDatabaseUpgradePolicy(ctx context.Context, name string, p DBUpgradePolicy) (DBUpgradePolicy, error) {
	var out DBUpgradePolicy
	err := c.do(ctx, http.MethodPut, "/api/v1/databases/"+PathEscape(name)+"/upgrade-policy", p, &out)
	return out, err
}

// UpgradeDatabaseNow calls POST /api/v1/databases/{name}/upgrade-now.
func (c *Client) UpgradeDatabaseNow(ctx context.Context, name, version, confirm string) (DBUpgradeRun, error) {
	var out DBUpgradeRun
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+PathEscape(name)+"/upgrade-now", map[string]string{"version": version, "confirm": confirm}, &out)
	return out, err
}

// DatabaseUpgradeSummary calls GET /api/v1/databases/upgrade-summary.
func (c *Client) DatabaseUpgradeSummary(ctx context.Context) (DBUpgradeSummary, error) {
	var out DBUpgradeSummary
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/upgrade-summary", nil, &out)
	return out, err
}

// GetPlatformUpgradePolicy calls GET /api/v1/settings/database-upgrades.
func (c *Client) GetPlatformUpgradePolicy(ctx context.Context) (DBUpgradePolicy, error) {
	var out DBUpgradePolicy
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/database-upgrades", nil, &out)
	return out, err
}

// SetPlatformUpgradePolicy calls PUT /api/v1/settings/database-upgrades.
func (c *Client) SetPlatformUpgradePolicy(ctx context.Context, p DBUpgradePolicy) (DBUpgradePolicy, error) {
	var out DBUpgradePolicy
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/database-upgrades", p, &out)
	return out, err
}
