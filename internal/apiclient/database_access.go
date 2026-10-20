package apiclient

import (
	"context"
	"net/http"
)

// DatabaseUser mirrors internal/api's databaseUserResource. It never holds a secret.
type DatabaseUser struct {
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Preset          string `json:"preset,omitempty"`
	CanLogin        bool   `json:"can_login"`
	Superuser       bool   `json:"superuser"`
	CreateDB        bool   `json:"create_db"`
	CreateRole      bool   `json:"create_role"`
	ConnectionLimit int    `json:"connection_limit"`
	ValidUntil      string `json:"valid_until,omitempty"`
	Expired         bool   `json:"expired"`
	Connections     int    `json:"connections"`
	Protected       bool   `json:"protected"`
	Managed         bool   `json:"managed"`
	CreatedBy       string `json:"created_by,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
}

// DatabaseCredential is shown once by the server and is never stored.
type DatabaseCredential struct {
	Username     string `json:"username"`
	Password     string `json:"password"`
	Database     string `json:"database"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	SSLMode      string `json:"sslmode"`
	InternalURL  string `json:"internal_url"`
	ExternalURL  string `json:"external_url,omitempty"`
	ExternalNote string `json:"external_note,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

// DatabaseUserCreateRequest is POST /databases/{name}/users.
type DatabaseUserCreateRequest struct {
	Name            string `json:"name"`
	Preset          string `json:"preset,omitempty"`
	ConnectionLimit int    `json:"connection_limit,omitempty"`
	ExpiresAt       string `json:"expires_at,omitempty"`
}

// DatabaseUserCreated is the create response.
type DatabaseUserCreated struct {
	User       DatabaseUser       `json:"user"`
	Credential DatabaseCredential `json:"credential"`
}

// DatabaseTemp is one tracked temporary credential, without its secret.
type DatabaseTemp struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Preset    string `json:"preset"`
	CreatedBy string `json:"created_by"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	State     string `json:"state"`
}

// DatabaseTTLLimits are the allowed temporary credential lifetimes.
type DatabaseTTLLimits struct {
	MinMinutes     int `json:"min_minutes"`
	MaxMinutes     int `json:"max_minutes"`
	DefaultMinutes int `json:"default_minutes"`
}

// DatabaseTempIssued is the issue response.
type DatabaseTempIssued struct {
	Temp       DatabaseTemp       `json:"temp"`
	Credential DatabaseCredential `json:"credential"`
	Clamped    bool               `json:"clamped"`
	Limits     DatabaseTTLLimits  `json:"limits"`
}

// DatabaseTempList is GET /databases/{name}/access/temp.
type DatabaseTempList struct {
	Items  []DatabaseTemp    `json:"items"`
	Limits DatabaseTTLLimits `json:"limits"`
}

// DatabasePrincipal is a user or token with effective abilities on a database.
type DatabasePrincipal struct {
	Type      string   `json:"type"`
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Effective []string `json:"effective"`
	Via       []string `json:"via"`
}

// DatabasePolicyRef is a policy that touches a database.
type DatabasePolicyRef struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Principals  []string `json:"principals"`
	ScopedToOne bool     `json:"scoped_to_database"`
}

// DatabaseWho is GET /databases/{name}/access/principals.
type DatabaseWho struct {
	Database   string              `json:"database"`
	Principals []DatabasePrincipal `json:"principals"`
	Policies   []DatabasePolicyRef `json:"policies"`
	Templates  []string            `json:"grant_templates"`
}

// DatabaseGrantRequest is POST /databases/{name}/access/grants.
type DatabaseGrantRequest struct {
	Template      string `json:"template"`
	PrincipalType string `json:"principal_type"`
	PrincipalID   string `json:"principal_id"`
	Preview       bool   `json:"preview,omitempty"`
}

// DatabaseGrant is the grant response.
type DatabaseGrant struct {
	PolicyName string   `json:"policy_name"`
	Principal  string   `json:"principal"`
	Applied    bool     `json:"applied"`
	Notes      []string `json:"notes"`
}

// DatabaseClient is one app that reaches a database.
type DatabaseClient struct {
	App         string `json:"app"`
	Via         string `json:"via"`
	ProjectName string `json:"project_name,omitempty"`
	Environment string `json:"environment,omitempty"`
	InScope     bool   `json:"in_scope"`
	ScopeReason string `json:"scope_reason,omitempty"`
}

// DatabaseRule is one allowed source.
type DatabaseRule struct {
	Source      string `json:"source"`
	Description string `json:"description,omitempty"`
}

// DatabaseNetwork is GET /databases/{name}/network.
type DatabaseNetwork struct {
	Database string `json:"database"`
	Running  bool   `json:"running"`
	Internal struct {
		Host    string `json:"host"`
		Address string `json:"address,omitempty"`
		Port    int    `json:"port"`
	} `json:"internal"`
	Networks []struct {
		Name    string `json:"name"`
		Address string `json:"address,omitempty"`
		Kind    string `json:"kind"`
	} `json:"networks"`
	Published *struct {
		HostPort      int      `json:"host_port"`
		ContainerPort int      `json:"container_port"`
		Bind          []string `json:"bind"`
		Class         string   `json:"class"`
	} `json:"published,omitempty"`
	Clients []DatabaseClient `json:"clients"`
	Verdict struct {
		Level string `json:"level"`
		Text  string `json:"text"`
	} `json:"verdict"`
	Rules struct {
		Port         int            `json:"port,omitempty"`
		Active       bool           `json:"active"`
		Allow        []DatabaseRule `json:"allow"`
		Missing      []string       `json:"missing,omitempty"`
		Extra        []string       `json:"extra,omitempty"`
		CanRestrict  bool           `json:"can_restrict"`
		CannotReason string         `json:"cannot_restrict_reason,omitempty"`
	} `json:"rules"`
	Scope struct {
		Current     string `json:"current"`
		ProjectName string `json:"project_name,omitempty"`
		Environment string `json:"environment,omitempty"`
	} `json:"scope"`
	TLS struct {
		Supported bool   `json:"supported"`
		Enabled   bool   `json:"enabled"`
		Required  bool   `json:"required"`
		State     string `json:"state,omitempty"`
		Drift     bool   `json:"drift"`
	} `json:"tls"`
	Caveats []string `json:"caveats"`
}

// DatabaseRulesRequest is the body of the network rules preview and apply calls.
type DatabaseRulesRequest struct {
	Allow           []DatabaseRule `json:"allow"`
	LocalContainers bool           `json:"local_containers,omitempty"`
	Confirm         bool           `json:"confirm,omitempty"`
}

// DatabaseRulesPlan is the rules preview or apply response.
type DatabaseRulesPlan struct {
	Commands []string `json:"commands"`
	Drops    string   `json:"drops"`
	Allow    []string `json:"allow"`
	Warnings []string `json:"warnings"`
	Applied  bool     `json:"applied"`
}

func dbPath(name, suffix string) string {
	return "/api/v1/databases/" + PathEscape(name) + suffix
}

// ListDatabaseUsers calls GET /api/v1/databases/{name}/users.
func (c *Client) ListDatabaseUsers(ctx context.Context, name string) ([]DatabaseUser, error) {
	var out []DatabaseUser
	err := c.do(ctx, http.MethodGet, dbPath(name, "/users"), nil, &out)
	return out, err
}

// CreateDatabaseUser calls POST /api/v1/databases/{name}/users.
func (c *Client) CreateDatabaseUser(ctx context.Context, name string, req DatabaseUserCreateRequest) (DatabaseUserCreated, error) {
	var out DatabaseUserCreated
	err := c.do(ctx, http.MethodPost, dbPath(name, "/users"), req, &out)
	return out, err
}

// RotateDatabaseUser calls POST /api/v1/databases/{name}/users/{role}/rotate.
func (c *Client) RotateDatabaseUser(ctx context.Context, name, role string) (DatabaseCredential, error) {
	var out DatabaseCredential
	err := c.do(ctx, http.MethodPost, dbPath(name, "/users/"+PathEscape(role)+"/rotate"), nil, &out)
	return out, err
}

// SetDatabaseUserLogin calls POST .../users/{role}/disable or /enable.
func (c *Client) SetDatabaseUserLogin(ctx context.Context, name, role string, login bool) error {
	verb := "/disable"
	if login {
		verb = "/enable"
	}
	return c.do(ctx, http.MethodPost, dbPath(name, "/users/"+PathEscape(role)+verb), nil, nil)
}

// DeleteDatabaseUser calls DELETE /api/v1/databases/{name}/users/{role}.
func (c *Client) DeleteDatabaseUser(ctx context.Context, name, role string) error {
	return c.do(ctx, http.MethodDelete, dbPath(name, "/users/"+PathEscape(role)), nil, nil)
}

// IssueDatabaseTemp calls POST /api/v1/databases/{name}/access/temp.
func (c *Client) IssueDatabaseTemp(ctx context.Context, name, preset string, ttlMinutes int) (DatabaseTempIssued, error) {
	var out DatabaseTempIssued
	body := map[string]any{"preset": preset, "ttl_minutes": ttlMinutes}
	err := c.do(ctx, http.MethodPost, dbPath(name, "/access/temp"), body, &out)
	return out, err
}

// ListDatabaseTemp calls GET /api/v1/databases/{name}/access/temp.
func (c *Client) ListDatabaseTemp(ctx context.Context, name string) (DatabaseTempList, error) {
	var out DatabaseTempList
	err := c.do(ctx, http.MethodGet, dbPath(name, "/access/temp"), nil, &out)
	return out, err
}

// RevokeDatabaseTemp calls DELETE /api/v1/databases/{name}/access/temp/{id}.
func (c *Client) RevokeDatabaseTemp(ctx context.Context, name, id string) error {
	return c.do(ctx, http.MethodDelete, dbPath(name, "/access/temp/"+PathEscape(id)), nil, nil)
}

// GetDatabaseWho calls GET /api/v1/databases/{name}/access/principals.
func (c *Client) GetDatabaseWho(ctx context.Context, name string) (DatabaseWho, error) {
	var out DatabaseWho
	err := c.do(ctx, http.MethodGet, dbPath(name, "/access/principals"), nil, &out)
	return out, err
}

// GrantDatabaseAccess calls POST /api/v1/databases/{name}/access/grants.
func (c *Client) GrantDatabaseAccess(ctx context.Context, name string, req DatabaseGrantRequest) (DatabaseGrant, error) {
	var out DatabaseGrant
	err := c.do(ctx, http.MethodPost, dbPath(name, "/access/grants"), req, &out)
	return out, err
}

// GetDatabaseNetwork calls GET /api/v1/databases/{name}/network.
func (c *Client) GetDatabaseNetwork(ctx context.Context, name string) (DatabaseNetwork, error) {
	var out DatabaseNetwork
	err := c.do(ctx, http.MethodGet, dbPath(name, "/network"), nil, &out)
	return out, err
}

// PreviewDatabaseRules calls POST /api/v1/databases/{name}/network/rules/preview.
func (c *Client) PreviewDatabaseRules(ctx context.Context, name string, req DatabaseRulesRequest) (DatabaseRulesPlan, error) {
	var out DatabaseRulesPlan
	err := c.do(ctx, http.MethodPost, dbPath(name, "/network/rules/preview"), req, &out)
	return out, err
}

// ApplyDatabaseRules calls PUT /api/v1/databases/{name}/network/rules.
func (c *Client) ApplyDatabaseRules(ctx context.Context, name string, req DatabaseRulesRequest) (DatabaseRulesPlan, error) {
	var out DatabaseRulesPlan
	err := c.do(ctx, http.MethodPut, dbPath(name, "/network/rules"), req, &out)
	return out, err
}

// RemoveDatabaseRules calls DELETE /api/v1/databases/{name}/network/rules.
func (c *Client) RemoveDatabaseRules(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, dbPath(name, "/network/rules"), nil, nil)
}

// DatabaseScopeVerdict is one referencing app's fate under a scope.
type DatabaseScopeVerdict struct {
	App struct {
		Name string `json:"name"`
	} `json:"app"`
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// DatabaseScopeResult is the scope dry-run or apply response.
type DatabaseScopeResult struct {
	Scope           string                 `json:"scope"`
	Applied         bool                   `json:"applied"`
	ConfirmRequired bool                   `json:"confirm_required"`
	Verdicts        []DatabaseScopeVerdict `json:"verdicts"`
	Lost            []DatabaseScopeVerdict `json:"lost"`
	Notes           []string               `json:"notes"`
}

// SetDatabaseScope calls PUT /api/v1/databases/{name}/network/scope. With
// dryRun nothing changes. A scope that would cut apps off is not applied
// without confirm; the result then has ConfirmRequired set.
func (c *Client) SetDatabaseScope(ctx context.Context, name, scope string, dryRun, confirm bool) (DatabaseScopeResult, error) {
	var out DatabaseScopeResult
	body := map[string]any{"scope": scope, "dry_run": dryRun, "confirm": confirm}
	err := c.do(ctx, http.MethodPut, dbPath(name, "/network/scope"), body, &out)
	return out, err
}

// SetDatabaseRequireTLS calls PUT /api/v1/databases/{name}/network/tls.
func (c *Client) SetDatabaseRequireTLS(ctx context.Context, name string, require bool) error {
	return c.do(ctx, http.MethodPut, dbPath(name, "/network/tls"), map[string]bool{"require": require}, nil)
}

// MakeDatabasePrivate calls POST /api/v1/databases/{name}/network/make-private.
func (c *Client) MakeDatabasePrivate(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPost, dbPath(name, "/network/make-private"), nil, nil)
}
