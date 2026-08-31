// Package apiclient is the one HTTP client implementation for the
// control plane's versioned API (internal/api, mounted at /api/v1).
// cmd/levelrail-cli and cmd/levelrail-mcp both import this package rather
// than each rolling their own: they are exactly the external callers the
// documented wire contract in internal/api exists for (see
// internal/store/tokens.go's own doc comment on APIToken), so neither
// gets special internal access, only this same bearer-token HTTP client.
// Wire-shape request/response types live in types.go; this file is the
// Client type itself and its methods.
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is a minimal HTTP client for the control plane's versioned API
// (internal/api, mounted at /api/v1). It carries no session/cookie
// state: every request is a single bearer-token call, matching how this
// project's own token scheme (internal/api/abilities.go) is meant to be
// used by a non-interactive caller.
type Client struct {
	baseURL string
	token   string
	hc      *http.Client
}

// NewClient builds a Client. baseURL should not have a trailing slash
// (trimmed defensively if it does). The timeout is long, not the usual
// few seconds a typical REST call would use: POST .../builds is
// synchronous and blocking (internal/api/builds.go's own doc comment),
// and a real Dockerfile build can legitimately take minutes.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		hc:      &http.Client{Timeout: 15 * time.Minute},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody) //nolint:gosec // c.baseURL is the operator-supplied API target this client exists to call, not attacker-controlled input
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.hc.Do(req) //nolint:gosec // same target as above
	if err != nil {
		// A transport-level failure (connection refused, DNS, TLS,
		// timeout): deliberately not wrapped in *APIError, so callers
		// can tell "never reached the server" apart from "the server
		// answered with an error" via errors.As, and pick a different
		// exit code / tool-error shape for each.
		return fmt.Errorf("request %s %s: %w", method, c.baseURL+path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	return decodeResponse(resp, out)
}

// decodeResponse reads resp's body, mapping a non-2xx status to
// *APIError and otherwise JSON-decoding into out. Shared by do() and any
// method (DeployCompose) that must build its own *http.Request because
// its body isn't JSON.
func decodeResponse(resp *http.Response, out any) error {
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Message: ExtractErrorMessage(data)}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response body: %w", err)
		}
	}
	return nil
}

// ExtractErrorMessage parses the control plane's {"error": "..."} body
// shape (internal/api/respond.go). A body that doesn't match that shape
// (a proxy's HTML error page, an empty body) falls back to the raw text
// so the caller still sees something, not a blank message. Exported so
// callers with their own hand-rolled request (outside this Client, e.g.
// the CLI's session-cookie authSessionClient) can reuse the same parsing
// rather than duplicating it.
func ExtractErrorMessage(data []byte) string {
	var body apiErrorBody
	if err := json.Unmarshal(data, &body); err == nil && body.Error != "" {
		return body.Error
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "(empty response body)"
	}
	return trimmed
}

// CreateApp calls POST /api/v1/apps.
func (c *Client) CreateApp(ctx context.Context, req AppResource) (AppResource, error) {
	var out AppResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps", req, &out)
	return out, err
}

// GetApp calls GET /api/v1/apps/{name}.
func (c *Client) GetApp(ctx context.Context, name string) (AppResource, error) {
	var out AppResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name), nil, &out)
	return out, err
}

// ListApps calls GET /api/v1/apps.
func (c *Client) ListApps(ctx context.Context) ([]AppResource, error) {
	var out []AppResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps", nil, &out)
	return out, err
}

// TriggerBuild calls POST /api/v1/apps/{name}/builds.
func (c *Client) TriggerBuild(ctx context.Context, name string, req BuildTriggerRequest) (BuildTriggerResponse, error) {
	var out BuildTriggerResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/builds", req, &out)
	return out, err
}

// DeployApp calls POST /api/v1/apps/{name}/deploys: points the app's
// desired image at image. Asynchronous: this returns as soon as the
// desired state is saved, not once a container is actually running it.
// Also how a rollback is done: deploying an older, already-known tag
// with this same method.
func (c *Client) DeployApp(ctx context.Context, name, image string) (AppResource, error) {
	var out AppResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/deploys", DeployTriggerRequest{Image: image}, &out)
	return out, err
}

// SetAppDatabaseAttachment calls PUT /api/v1/apps/{name}/database:
// attaches an existing managed database to name as a real, persisted
// connection env var source.
func (c *Client) SetAppDatabaseAttachment(ctx context.Context, name string, req SetAppDatabaseRequest) (AppDatabaseResource, error) {
	var out AppDatabaseResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/database", req, &out)
	return out, err
}

// ClearAppDatabaseAttachment calls DELETE /api/v1/apps/{name}/database.
func (c *Client) ClearAppDatabaseAttachment(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"/database", nil, nil)
}

// DeployCompose calls POST /api/v1/apps/{name}/compose with composeYAML
// as the raw request body. Unlike every other Client method, this
// doesn't go through do(): handleDeployCompose (internal/api/apps_compose.go)
// reads the body directly via io.ReadAll and parses it as a compose.yaml
// document, so the body must be sent as-is, not JSON-marshaled, with
// Content-Type: text/yaml instead of do()'s hardcoded application/json.
func (c *Client) DeployCompose(ctx context.Context, name string, composeYAML []byte) (ComposeDeployResult, error) {
	var out ComposeDeployResult

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/apps/"+PathEscape(name)+"/compose", bytes.NewReader(composeYAML)) //nolint:gosec // c.baseURL is the operator-supplied API target, same as do()
	if err != nil {
		return out, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "text/yaml")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.hc.Do(req) //nolint:gosec // same target as do()
	if err != nil {
		return out, fmt.Errorf("request %s %s: %w", http.MethodPost, req.URL.String(), err)
	}
	defer func() { _ = resp.Body.Close() }()

	err = decodeResponse(resp, &out)
	return out, err
}

// RestartApp calls POST /api/v1/apps/{name}/restart: force a running
// container to be recreated with no image change (internal/api/apps.go's
// own handleRestartApp). No request body; the response is the app's
// current desired state, unchanged.
func (c *Client) RestartApp(ctx context.Context, name string) (AppResource, error) {
	var out AppResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/restart", nil, &out)
	return out, err
}

// StopApp calls POST /api/v1/apps/{name}/stop: marks the service
// suspended, so the reconciler stops (and leaves stopped) its container
// on the next pass, without touching desired state otherwise.
func (c *Client) StopApp(ctx context.Context, name string) (AppResource, error) {
	var out AppResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/stop", nil, &out)
	return out, err
}

// StartApp calls POST /api/v1/apps/{name}/start: clears the suspended
// flag StopApp set, letting the reconciler bring the container back.
func (c *Client) StartApp(ctx context.Context, name string) (AppResource, error) {
	var out AppResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/start", nil, &out)
	return out, err
}

// DeleteApp calls DELETE /api/v1/apps/{name}: removes the app's desired
// state (internal/api/apps.go's own handleDeleteApp doc comment covers
// the known gap that this does not itself stop the running container).
func (c *Client) DeleteApp(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name), nil, nil)
}

// GetDeployStatus calls GET /api/v1/apps/{name}/deploys: the application
// controller's current stored reconcile conditions, not a deploy history
// log (internal/api/deploys.go's own handleDeployHistory doc comment).
func (c *Client) GetDeployStatus(ctx context.Context, name string) ([]ConditionResource, error) {
	var out []ConditionResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/deploys", nil, &out)
	return out, err
}

// GetAppNetwork calls GET /api/v1/apps/{name}/network
// (internal/api/network.go's handleGetAppNetwork).
func (c *Client) GetAppNetwork(ctx context.Context, name string) (NetworkResource, error) {
	var out NetworkResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/network", nil, &out)
	return out, err
}

// QueryLogs calls GET /api/v1/apps/{name}/logs?from=&to=&q=
// (internal/api/logs.go's handleQueryLogs): a real, historical full-text
// search over already-stored log entries. from/to are sent as RFC3339,
// the same format the server requires (internal/api's parseTimeRange);
// q empty means every entry in the window, matching the server's own
// "empty query" contract.
func (c *Client) QueryLogs(ctx context.Context, name string, from, to time.Time, q string) ([]LogEntryResource, error) {
	query := url.Values{}
	query.Set("from", from.UTC().Format(time.RFC3339))
	query.Set("to", to.UTC().Format(time.RFC3339))
	if q != "" {
		query.Set("q", q)
	}
	var out logsResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/logs?"+query.Encode(), nil, &out)
	return out.Entries, err
}

// CreateDatabase calls POST /api/v1/databases.
func (c *Client) CreateDatabase(ctx context.Context, req DatabaseResource) (DatabaseResource, error) {
	var out DatabaseResource
	err := c.do(ctx, http.MethodPost, "/api/v1/databases", req, &out)
	return out, err
}

// GetDatabase calls GET /api/v1/databases/{name}.
func (c *Client) GetDatabase(ctx context.Context, name string) (DatabaseResource, error) {
	var out DatabaseResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+PathEscape(name), nil, &out)
	return out, err
}

// ListDatabases calls GET /api/v1/databases.
func (c *Client) ListDatabases(ctx context.Context) ([]DatabaseResource, error) {
	var out []DatabaseResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases", nil, &out)
	return out, err
}

// DeleteDatabase calls DELETE /api/v1/databases/{name}.
func (c *Client) DeleteDatabase(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/databases/"+PathEscape(name), nil, nil)
}

// ListDatabaseEngines calls GET /api/v1/database-engines: every engine
// this control plane can actually create, backing the creation wizard's
// engine picker instead of a hardcoded list.
func (c *Client) ListDatabaseEngines(ctx context.Context) ([]DatabaseEngineResource, error) {
	var out []DatabaseEngineResource
	err := c.do(ctx, http.MethodGet, "/api/v1/database-engines", nil, &out)
	return out, err
}

// SetDatabaseResources calls PUT /api/v1/databases/{name}/resources:
// applies memory/CPU limits to name, replacing whatever was set before.
func (c *Client) SetDatabaseResources(ctx context.Context, name string, resources *ServiceResources) (DatabaseResource, error) {
	var out DatabaseResource
	err := c.do(ctx, http.MethodPut, "/api/v1/databases/"+PathEscape(name)+"/resources", SetDatabaseResourcesRequest{Resources: resources}, &out)
	return out, err
}

// SetDatabasePublicAccess calls PUT /api/v1/databases/{name}/public-access:
// exposes name on the host at port (0 requests auto-assignment).
func (c *Client) SetDatabasePublicAccess(ctx context.Context, name string, port int) (DatabasePublicAccessResource, error) {
	var out DatabasePublicAccessResource
	err := c.do(ctx, http.MethodPut, "/api/v1/databases/"+PathEscape(name)+"/public-access", SetDatabasePublicAccessRequest{Port: port}, &out)
	return out, err
}

// ExecApp calls POST /api/v1/apps/{name}/exec: runs command (plus args)
// inside the app's currently running container and returns its
// stdout/stderr/exit code once it finishes. AbilityRoot-gated
// server-side (internal/api/exec.go's own doc comment: secrets are
// injected as plaintext env vars, so exec can read them anyway, and
// must sit behind the tier that boundary implies).
func (c *Client) ExecApp(ctx context.Context, name string, req ExecRequest) (ExecResponse, error) {
	var out ExecResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/exec", req, &out)
	return out, err
}

// ListDomains calls GET /api/v1/domains: every service_domains row
// across every app, aggregated in one call.
func (c *Client) ListDomains(ctx context.Context) ([]DomainResource, error) {
	var out []DomainResource
	err := c.do(ctx, http.MethodGet, "/api/v1/domains", nil, &out)
	return out, err
}

// GetCloudflareDNS calls GET /api/v1/settings/cloudflare-dns: the
// Cloudflare DNS-01 credential's enabled/has_token state, needed for
// ACME to issue real wildcard certificates (HTTP-01 cannot).
func (c *Client) GetCloudflareDNS(ctx context.Context) (CloudflareDNSResource, error) {
	var out CloudflareDNSResource
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/cloudflare-dns", nil, &out)
	return out, err
}

// SetCloudflareDNS calls PUT /api/v1/settings/cloudflare-dns.
func (c *Client) SetCloudflareDNS(ctx context.Context, req UpdateCloudflareDNSRequest) (CloudflareDNSResource, error) {
	var out CloudflareDNSResource
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/cloudflare-dns", req, &out)
	return out, err
}

// DisconnectCloudflareDNS calls DELETE /api/v1/settings/cloudflare-dns.
func (c *Client) DisconnectCloudflareDNS(ctx context.Context) (CloudflareDNSResource, error) {
	var out CloudflareDNSResource
	err := c.do(ctx, http.MethodDelete, "/api/v1/settings/cloudflare-dns", nil, &out)
	return out, err
}

// GetCloudflareTunnel calls GET /api/v1/settings/cloudflare-tunnel: the
// cloudflared container's configured/observed state, exposing the
// control plane through a Cloudflare Tunnel instead of an inbound port.
func (c *Client) GetCloudflareTunnel(ctx context.Context) (CloudflareTunnelResource, error) {
	var out CloudflareTunnelResource
	err := c.do(ctx, http.MethodGet, "/api/v1/settings/cloudflare-tunnel", nil, &out)
	return out, err
}

// SetCloudflareTunnel calls PUT /api/v1/settings/cloudflare-tunnel.
func (c *Client) SetCloudflareTunnel(ctx context.Context, req UpdateCloudflareTunnelRequest) (CloudflareTunnelResource, error) {
	var out CloudflareTunnelResource
	err := c.do(ctx, http.MethodPut, "/api/v1/settings/cloudflare-tunnel", req, &out)
	return out, err
}

// DisconnectCloudflareTunnel calls DELETE
// /api/v1/settings/cloudflare-tunnel: disables the tunnel and clears the
// stored token in one step.
func (c *Client) DisconnectCloudflareTunnel(ctx context.Context) (CloudflareTunnelResource, error) {
	var out CloudflareTunnelResource
	err := c.do(ctx, http.MethodDelete, "/api/v1/settings/cloudflare-tunnel", nil, &out)
	return out, err
}

// domainAuthPath builds /api/v1/apps/{name}/domains/{domain}/auth,
// shared by all three domain basic auth methods below.
func domainAuthPath(name, domain string) string {
	return "/api/v1/apps/" + PathEscape(name) + "/domains/" + PathEscape(domain) + "/auth"
}

// GetDomainBasicAuth calls GET /api/v1/apps/{name}/domains/{domain}/auth:
// domain's current HTTP Basic Auth state.
func (c *Client) GetDomainBasicAuth(ctx context.Context, name, domain string) (DomainBasicAuthResource, error) {
	var out DomainBasicAuthResource
	err := c.do(ctx, http.MethodGet, domainAuthPath(name, domain), nil, &out)
	return out, err
}

// SetDomainBasicAuth calls PUT /api/v1/apps/{name}/domains/{domain}/auth:
// enables HTTP Basic Auth on domain, enforced by Caddy on the next
// ingress reconcile pass.
func (c *Client) SetDomainBasicAuth(ctx context.Context, name, domain string, req SetDomainBasicAuthRequest) (DomainBasicAuthResource, error) {
	var out DomainBasicAuthResource
	err := c.do(ctx, http.MethodPut, domainAuthPath(name, domain), req, &out)
	return out, err
}

// ClearDomainBasicAuth calls DELETE
// /api/v1/apps/{name}/domains/{domain}/auth: removes basic auth
// protection from domain.
func (c *Client) ClearDomainBasicAuth(ctx context.Context, name, domain string) (DomainBasicAuthResource, error) {
	var out DomainBasicAuthResource
	err := c.do(ctx, http.MethodDelete, domainAuthPath(name, domain), nil, &out)
	return out, err
}

// TriggerBackup calls POST /api/v1/databases/{name}/backups: starts a
// real backup of name to targetID and returns as soon as the attempt is
// recorded and under way, not once the dump and upload actually finish.
// ListBackups is how a caller finds out whether it did.
func (c *Client) TriggerBackup(ctx context.Context, name, targetID string) (BackupHistoryResource, error) {
	var out BackupHistoryResource
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+PathEscape(name)+"/backups", TriggerBackupRequest{TargetID: targetID}, &out)
	return out, err
}

// SetBackupSchedule calls PUT /api/v1/databases/{name}/backup-schedule:
// configures a recurring backup, replacing any previously configured
// schedule for name.
func (c *Client) SetBackupSchedule(ctx context.Context, name string, req SetBackupScheduleRequest) (BackupScheduleResource, error) {
	var out BackupScheduleResource
	err := c.do(ctx, http.MethodPut, "/api/v1/databases/"+PathEscape(name)+"/backup-schedule", req, &out)
	return out, err
}

// ClearBackupSchedule calls DELETE
// /api/v1/databases/{name}/backup-schedule: returns name to its default
// "no scheduled backup configured" state. Past backup_history rows are
// untouched, only the going-forward configuration.
func (c *Client) ClearBackupSchedule(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/databases/"+PathEscape(name)+"/backup-schedule", nil, nil)
}

// ListBackups calls GET /api/v1/databases/{name}/backups: the full
// backup attempt history for one database.
func (c *Client) ListBackups(ctx context.Context, name string, opts ListBackupsOptions) ([]BackupHistoryResource, error) {
	path := "/api/v1/databases/" + PathEscape(name) + "/backups"
	q := url.Values{}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Before != "" {
		q.Set("before", opts.Before)
	}
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out []BackupHistoryResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// TriggerRestore calls POST /api/v1/databases/{name}/restore: overwrites
// name's live data in place from a previously succeeded backup attempt.
// The single most destructive call this Client makes; callers must gate
// this behind their own explicit confirmation before ever reaching it.
func (c *Client) TriggerRestore(ctx context.Context, name, backupID string) (RestoreHistoryResource, error) {
	var out RestoreHistoryResource
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+PathEscape(name)+"/restore", TriggerRestoreRequest{BackupID: backupID}, &out)
	return out, err
}

// GetSession calls GET /api/v1/auth/session using this Client's bearer
// token, exactly the same auth mechanism every other method on this
// type uses. The server gates this route with requireAuth,
// session-cookie-only, by explicit design (internal/api/account.go's
// handleGetSession doc comment), so a call made through this method
// always gets back a real, honest 401 "authentication required" for a
// bearer-token caller, not a client-side bug.
func (c *Client) GetSession(ctx context.Context) (SessionInfoResource, error) {
	var out SessionInfoResource
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/session", nil, &out)
	return out, err
}

// SetSecret calls PUT /api/v1/apps/{name}/secrets/{key}. No response
// body beyond the status (internal/api/secrets.go's handleSetSecret
// returns 204 on success), matching that handler's own doc comment on
// why a secret's value is never echoed back. overwriteLocked bypasses
// the server's 409-if-locked guard, the same escape hatch the dashboard's
// SecretsEditor exposes.
func (c *Client) SetSecret(ctx context.Context, name, key, value string, overwriteLocked bool) error {
	return c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/secrets/"+PathEscape(key), SetSecretRequest{Value: value, OverwriteLocked: overwriteLocked}, nil)
}

// ListSecrets calls GET /api/v1/apps/{name}/secrets: every known secret
// key for the app with its locked state, never a value.
func (c *Client) ListSecrets(ctx context.Context, name string) ([]SecretKeyResource, error) {
	var out []SecretKeyResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/secrets", nil, &out)
	return out, err
}

// SetSecretLock calls POST /api/v1/apps/{name}/secrets/{key}/lock:
// toggles the accidental-overwrite guard SetSecret's overwriteLocked
// bypasses. Reversible either direction, not a permanent write-once
// marker.
func (c *Client) SetSecretLock(ctx context.Context, name, key string, locked bool) error {
	return c.do(ctx, http.MethodPost, "/api/v1/apps/"+PathEscape(name)+"/secrets/"+PathEscape(key)+"/lock", SetSecretLockRequest{Locked: locked}, nil)
}

// GetGitSource calls GET /api/v1/apps/{name}/git-source.
func (c *Client) GetGitSource(ctx context.Context, name string) (GitSourceResource, error) {
	var out GitSourceResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/git-source", nil, &out)
	return out, err
}

// SetGitSource calls PUT /api/v1/apps/{name}/git-source: connects a repo
// (creating a new webhook secret) the first time it's called for an
// app, or edits the connection on every call after.
func (c *Client) SetGitSource(ctx context.Context, name string, req SetGitSourceRequest) (GitSourceResource, error) {
	var out GitSourceResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/git-source", req, &out)
	return out, err
}

// DeleteGitSource calls DELETE /api/v1/apps/{name}/git-source.
func (c *Client) DeleteGitSource(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"/git-source", nil, nil)
}

// ListServiceTemplates calls GET /api/v1/service-templates: the full
// catalog, without each entry's Compose body (see ServiceTemplateListItem).
func (c *Client) ListServiceTemplates(ctx context.Context) ([]ServiceTemplateListItem, error) {
	var out []ServiceTemplateListItem
	err := c.do(ctx, http.MethodGet, "/api/v1/service-templates", nil, &out)
	return out, err
}

// GetServiceTemplate calls GET /api/v1/service-templates/{id}: one
// catalog entry, including its full compose.yaml body.
func (c *Client) GetServiceTemplate(ctx context.Context, id string) (ServiceTemplateDetail, error) {
	var out ServiceTemplateDetail
	err := c.do(ctx, http.MethodGet, "/api/v1/service-templates/"+PathEscape(id), nil, &out)
	return out, err
}

// backupTargetsCollectionPath builds /api/v1/backup-targets, and
// backupTargetPath builds that same path plus /{id}: shared by every
// backup-target method below, the same "one place the URL segment lives"
// reasoning scheduledTasksCollectionPath's own doc comment gives.
func backupTargetsCollectionPath() string {
	return "/api/v1/backup-targets"
}

func backupTargetPath(id string) string {
	return backupTargetsCollectionPath() + "/" + PathEscape(id)
}

// CreateBackupTarget calls POST /api/v1/backup-targets.
func (c *Client) CreateBackupTarget(ctx context.Context, req CreateBackupTargetRequest) (BackupTargetResource, error) {
	var out BackupTargetResource
	err := c.do(ctx, http.MethodPost, backupTargetsCollectionPath(), req, &out)
	return out, err
}

// ListBackupTargets calls GET /api/v1/backup-targets.
func (c *Client) ListBackupTargets(ctx context.Context) ([]BackupTargetResource, error) {
	var out []BackupTargetResource
	err := c.do(ctx, http.MethodGet, backupTargetsCollectionPath(), nil, &out)
	return out, err
}

// GetBackupTarget calls GET /api/v1/backup-targets/{id}.
func (c *Client) GetBackupTarget(ctx context.Context, id string) (BackupTargetResource, error) {
	var out BackupTargetResource
	err := c.do(ctx, http.MethodGet, backupTargetPath(id), nil, &out)
	return out, err
}

// UpdateBackupTarget calls PUT /api/v1/backup-targets/{id}: a full
// replace of name/provider/endpoint/region/bucket, matching
// handleUpdateBackupTarget's own contract. Credentials rotate only when
// req carries both AccessKeyID and SecretAccessKey.
func (c *Client) UpdateBackupTarget(ctx context.Context, id string, req UpdateBackupTargetRequest) (BackupTargetResource, error) {
	var out BackupTargetResource
	err := c.do(ctx, http.MethodPut, backupTargetPath(id), req, &out)
	return out, err
}

// DeleteBackupTarget calls DELETE /api/v1/backup-targets/{id}.
func (c *Client) DeleteBackupTarget(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, backupTargetPath(id), nil, nil)
}

// registryCredentialsCollectionPath builds /api/v1/registry-credentials,
// and registryCredentialPath builds that same path plus /{id}.
func registryCredentialsCollectionPath() string {
	return "/api/v1/registry-credentials"
}

func registryCredentialPath(id string) string {
	return registryCredentialsCollectionPath() + "/" + PathEscape(id)
}

// CreateRegistryCredential calls POST /api/v1/registry-credentials.
func (c *Client) CreateRegistryCredential(ctx context.Context, req CreateRegistryCredentialRequest) (RegistryCredentialResource, error) {
	var out RegistryCredentialResource
	err := c.do(ctx, http.MethodPost, registryCredentialsCollectionPath(), req, &out)
	return out, err
}

// ListRegistryCredentials calls GET /api/v1/registry-credentials.
func (c *Client) ListRegistryCredentials(ctx context.Context) ([]RegistryCredentialResource, error) {
	var out []RegistryCredentialResource
	err := c.do(ctx, http.MethodGet, registryCredentialsCollectionPath(), nil, &out)
	return out, err
}

// GetRegistryCredential calls GET /api/v1/registry-credentials/{id}.
func (c *Client) GetRegistryCredential(ctx context.Context, id string) (RegistryCredentialResource, error) {
	var out RegistryCredentialResource
	err := c.do(ctx, http.MethodGet, registryCredentialPath(id), nil, &out)
	return out, err
}

// UpdateRegistryCredential calls PUT /api/v1/registry-credentials/{id}: a
// full replace of name/registry_host/username, matching
// handleUpdateRegistryCredential's own contract. The password rotates
// only when req carries one.
func (c *Client) UpdateRegistryCredential(ctx context.Context, id string, req UpdateRegistryCredentialRequest) (RegistryCredentialResource, error) {
	var out RegistryCredentialResource
	err := c.do(ctx, http.MethodPut, registryCredentialPath(id), req, &out)
	return out, err
}

// DeleteRegistryCredential calls DELETE /api/v1/registry-credentials/{id}.
func (c *Client) DeleteRegistryCredential(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, registryCredentialPath(id), nil, nil)
}

// CreateNotificationChannel calls POST /api/v1/notification-channels.
func (c *Client) CreateNotificationChannel(ctx context.Context, req CreateNotificationChannelRequest) (NotificationChannelResource, error) {
	var out NotificationChannelResource
	err := c.do(ctx, http.MethodPost, "/api/v1/notification-channels", req, &out)
	return out, err
}

// ListNotificationChannels calls GET /api/v1/notification-channels.
func (c *Client) ListNotificationChannels(ctx context.Context) ([]NotificationChannelResource, error) {
	var out []NotificationChannelResource
	err := c.do(ctx, http.MethodGet, "/api/v1/notification-channels", nil, &out)
	return out, err
}

// DeleteNotificationChannel calls DELETE
// /api/v1/notification-channels/{id}.
func (c *Client) DeleteNotificationChannel(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/notification-channels/"+PathEscape(id), nil, nil)
}

// TestNotificationChannel calls POST
// /api/v1/notification-channels/test: fires a real test message via
// kind/notifyURL without requiring the channel to exist yet.
func (c *Client) TestNotificationChannel(ctx context.Context, kind, notifyURL string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/notification-channels/test", TestNotificationChannelRequest{Kind: kind, NotifyURL: notifyURL}, nil)
}

// TestExistingNotificationChannel calls POST
// /api/v1/notification-channels/{id}/test: the same real send, against
// an already-saved channel's own kind/notify_url.
func (c *Client) TestExistingNotificationChannel(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodPost, "/api/v1/notification-channels/"+PathEscape(id)+"/test", nil, nil)
}

// GetLogDrain calls GET /api/v1/apps/{name}/log-drain: the app's
// currently configured external log-forwarding sink. Returns *APIError
// with StatusCode 404 (via errors.As) if the app doesn't exist or has no
// drain configured, the same two-404-cases-in-one shape that route's own
// doc comment describes.
func (c *Client) GetLogDrain(ctx context.Context, name string) (LogDrainResource, error) {
	var out LogDrainResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/log-drain", nil, &out)
	return out, err
}

// SetLogDrain calls PUT /api/v1/apps/{name}/log-drain.
func (c *Client) SetLogDrain(ctx context.Context, name string, req SetLogDrainRequest) (LogDrainResource, error) {
	var out LogDrainResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/log-drain", req, &out)
	return out, err
}

// ClearLogDrain calls DELETE /api/v1/apps/{name}/log-drain. No response
// body (204 on success), matching SetSecret's own "no body beyond the
// status" shape above.
func (c *Client) ClearLogDrain(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/apps/"+PathEscape(name)+"/log-drain", nil, nil)
}

// scheduledTasksCollectionPath builds
// /api/v1/apps/{name}/scheduled-tasks, and scheduledTaskPath builds that
// same path plus /{id}: shared by every scheduled-task method below so
// the "/scheduled-tasks" segment exists in exactly one place.
func scheduledTasksCollectionPath(name string) string {
	return "/api/v1/apps/" + PathEscape(name) + "/scheduled-tasks"
}

func scheduledTaskPath(name, id string) string {
	return scheduledTasksCollectionPath(name) + "/" + PathEscape(id)
}

// CreateScheduledTask calls POST /api/v1/apps/{name}/scheduled-tasks.
func (c *Client) CreateScheduledTask(ctx context.Context, name string, req ScheduledTaskRequest) (ScheduledTaskResource, error) {
	var out ScheduledTaskResource
	err := c.do(ctx, http.MethodPost, scheduledTasksCollectionPath(name), req, &out)
	return out, err
}

// ListScheduledTasks calls GET /api/v1/apps/{name}/scheduled-tasks.
func (c *Client) ListScheduledTasks(ctx context.Context, name string) ([]ScheduledTaskResource, error) {
	var out []ScheduledTaskResource
	err := c.do(ctx, http.MethodGet, scheduledTasksCollectionPath(name), nil, &out)
	return out, err
}

// GetScheduledTask calls GET /api/v1/apps/{name}/scheduled-tasks/{id}.
func (c *Client) GetScheduledTask(ctx context.Context, name, id string) (ScheduledTaskResource, error) {
	var out ScheduledTaskResource
	err := c.do(ctx, http.MethodGet, scheduledTaskPath(name, id), nil, &out)
	return out, err
}

// UpdateScheduledTask calls PUT /api/v1/apps/{name}/scheduled-tasks/{id}:
// a full replace of Command/Schedule/Enabled, not a partial patch,
// matching handleUpdateScheduledTask's own contract.
func (c *Client) UpdateScheduledTask(ctx context.Context, name, id string, req ScheduledTaskRequest) (ScheduledTaskResource, error) {
	var out ScheduledTaskResource
	err := c.do(ctx, http.MethodPut, scheduledTaskPath(name, id), req, &out)
	return out, err
}

// DeleteScheduledTask calls DELETE
// /api/v1/apps/{name}/scheduled-tasks/{id}.
func (c *Client) DeleteScheduledTask(ctx context.Context, name, id string) error {
	return c.do(ctx, http.MethodDelete, scheduledTaskPath(name, id), nil, nil)
}

// RunScheduledTask calls POST
// /api/v1/apps/{name}/scheduled-tasks/{id}/run: triggers an immediate
// run and returns as soon as it starts, not once the command finishes.
func (c *Client) RunScheduledTask(ctx context.Context, name, id string) (ScheduledTaskResource, error) {
	var out ScheduledTaskResource
	err := c.do(ctx, http.MethodPost, scheduledTaskPath(name, id)+"/run", nil, &out)
	return out, err
}

// featureFlagsCollectionPath builds /api/v1/apps/{name}/flags, and
// featureFlagPath builds that same path plus /{id}: shared by every
// feature-flag CRUD method below, the same "one place for the segment"
// reasoning scheduledTasksCollectionPath's own doc comment gives.
func featureFlagsCollectionPath(name string) string {
	return "/api/v1/apps/" + PathEscape(name) + "/flags"
}

func featureFlagPath(name, id string) string {
	return featureFlagsCollectionPath(name) + "/" + PathEscape(id)
}

// CreateFeatureFlag calls POST /api/v1/apps/{name}/flags.
func (c *Client) CreateFeatureFlag(ctx context.Context, name string, req FeatureFlagRequest) (FeatureFlagResource, error) {
	var out FeatureFlagResource
	err := c.do(ctx, http.MethodPost, featureFlagsCollectionPath(name), req, &out)
	return out, err
}

// ListFeatureFlags calls GET /api/v1/apps/{name}/flags.
func (c *Client) ListFeatureFlags(ctx context.Context, name string) ([]FeatureFlagResource, error) {
	var out []FeatureFlagResource
	err := c.do(ctx, http.MethodGet, featureFlagsCollectionPath(name), nil, &out)
	return out, err
}

// GetFeatureFlag calls GET /api/v1/apps/{name}/flags/{id}.
func (c *Client) GetFeatureFlag(ctx context.Context, name, id string) (FeatureFlagResource, error) {
	var out FeatureFlagResource
	err := c.do(ctx, http.MethodGet, featureFlagPath(name, id), nil, &out)
	return out, err
}

// UpdateFeatureFlag calls PUT /api/v1/apps/{name}/flags/{id}: a full
// replace of Name/Description/Enabled/RolloutPercentage, not a partial
// patch, matching handleUpdateFeatureFlag's own contract. Key is never
// updated once created.
func (c *Client) UpdateFeatureFlag(ctx context.Context, name, id string, req FeatureFlagRequest) (FeatureFlagResource, error) {
	var out FeatureFlagResource
	err := c.do(ctx, http.MethodPut, featureFlagPath(name, id), req, &out)
	return out, err
}

// DeleteFeatureFlag calls DELETE /api/v1/apps/{name}/flags/{id}.
func (c *Client) DeleteFeatureFlag(ctx context.Context, name, id string) error {
	return c.do(ctx, http.MethodDelete, featureFlagPath(name, id), nil, nil)
}

// EvaluateFeatureFlag calls GET /api/v1/flags/evaluate/{key}, optionally
// with an identifier query param for consistent percentage-rollout
// bucketing. Flat, not nested under an app: see
// internal/api/feature_flags.go's own handleEvaluateFeatureFlag doc
// comment for why.
func (c *Client) EvaluateFeatureFlag(ctx context.Context, key, identifier string) (EvaluateFlagResource, error) {
	path := "/api/v1/flags/evaluate/" + PathEscape(key)
	if identifier != "" {
		path += "?identifier=" + url.QueryEscape(identifier)
	}
	var out EvaluateFlagResource
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

func organizationsCollectionPath() string {
	return "/api/v1/organizations"
}

func organizationPath(id string) string {
	return organizationsCollectionPath() + "/" + PathEscape(id)
}

// CreateOrganization calls POST /api/v1/organizations.
func (c *Client) CreateOrganization(ctx context.Context, req CreateOrganizationRequest) (OrganizationResource, error) {
	var out OrganizationResource
	err := c.do(ctx, http.MethodPost, organizationsCollectionPath(), req, &out)
	return out, err
}

// ListOrganizations calls GET /api/v1/organizations.
func (c *Client) ListOrganizations(ctx context.Context) ([]OrganizationResource, error) {
	var out []OrganizationResource
	err := c.do(ctx, http.MethodGet, organizationsCollectionPath(), nil, &out)
	return out, err
}

// GetOrganization calls GET /api/v1/organizations/{id}.
func (c *Client) GetOrganization(ctx context.Context, id string) (OrganizationResource, error) {
	var out OrganizationResource
	err := c.do(ctx, http.MethodGet, organizationPath(id), nil, &out)
	return out, err
}

// DeleteOrganization calls DELETE /api/v1/organizations/{id}.
func (c *Client) DeleteOrganization(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, organizationPath(id), nil, nil)
}

func projectsCollectionPath() string {
	return "/api/v1/projects"
}

func projectPath(id string) string {
	return projectsCollectionPath() + "/" + PathEscape(id)
}

// CreateProject calls POST /api/v1/projects.
func (c *Client) CreateProject(ctx context.Context, req CreateProjectRequest) (ProjectResource, error) {
	var out ProjectResource
	err := c.do(ctx, http.MethodPost, projectsCollectionPath(), req, &out)
	return out, err
}

// ListProjects calls GET /api/v1/projects.
func (c *Client) ListProjects(ctx context.Context) ([]ProjectResource, error) {
	var out []ProjectResource
	err := c.do(ctx, http.MethodGet, projectsCollectionPath(), nil, &out)
	return out, err
}

// GetProject calls GET /api/v1/projects/{id}.
func (c *Client) GetProject(ctx context.Context, id string) (ProjectResource, error) {
	var out ProjectResource
	err := c.do(ctx, http.MethodGet, projectPath(id), nil, &out)
	return out, err
}

// DeleteProject calls DELETE /api/v1/projects/{id}.
func (c *Client) DeleteProject(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, projectPath(id), nil, nil)
}

// GetProjectEnv calls GET /api/v1/projects/{id}/env.
func (c *Client) GetProjectEnv(ctx context.Context, id string) (map[string]string, error) {
	var out map[string]string
	err := c.do(ctx, http.MethodGet, projectPath(id)+"/env", nil, &out)
	return out, err
}

// SetProjectEnv calls PUT /api/v1/projects/{id}/env: a full replace,
// mirroring PUT /apps/{name}'s own env field semantics.
func (c *Client) SetProjectEnv(ctx context.Context, id string, vars map[string]string) (map[string]string, error) {
	var out map[string]string
	err := c.do(ctx, http.MethodPut, projectPath(id)+"/env", vars, &out)
	return out, err
}

// SetProjectOrganization calls PUT /api/v1/projects/{id}/organization.
// An empty orgID clears the assignment. Returns the updated project.
func (c *Client) SetProjectOrganization(ctx context.Context, projectID, orgID string) (ProjectResource, error) {
	var out ProjectResource
	err := c.do(ctx, http.MethodPut, projectPath(projectID)+"/organization", SetProjectOrganizationRequest{OrgID: orgID}, &out)
	return out, err
}

// GetOrganizationEnv calls GET /api/v1/organizations/{id}/env.
func (c *Client) GetOrganizationEnv(ctx context.Context, id string) (map[string]string, error) {
	var out map[string]string
	err := c.do(ctx, http.MethodGet, organizationPath(id)+"/env", nil, &out)
	return out, err
}

// SetOrganizationEnv calls PUT /api/v1/organizations/{id}/env: a full
// replace, mirroring PUT /apps/{name}'s own env field semantics.
func (c *Client) SetOrganizationEnv(ctx context.Context, id string, vars map[string]string) (map[string]string, error) {
	var out map[string]string
	err := c.do(ctx, http.MethodPut, organizationPath(id)+"/env", vars, &out)
	return out, err
}

func environmentsCollectionPath(projectID string) string {
	return "/api/v1/projects/" + PathEscape(projectID) + "/environments"
}

func environmentPath(id string) string {
	return "/api/v1/environments/" + PathEscape(id)
}

// CreateEnvironment calls POST /api/v1/projects/{id}/environments.
func (c *Client) CreateEnvironment(ctx context.Context, projectID string, req CreateEnvironmentRequest) (EnvironmentResource, error) {
	var out EnvironmentResource
	err := c.do(ctx, http.MethodPost, environmentsCollectionPath(projectID), req, &out)
	return out, err
}

// ListEnvironments calls GET /api/v1/projects/{id}/environments.
func (c *Client) ListEnvironments(ctx context.Context, projectID string) ([]EnvironmentResource, error) {
	var out []EnvironmentResource
	err := c.do(ctx, http.MethodGet, environmentsCollectionPath(projectID), nil, &out)
	return out, err
}

// DeleteEnvironment calls DELETE /api/v1/environments/{id}.
func (c *Client) DeleteEnvironment(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, environmentPath(id), nil, nil)
}

// GetEnvironmentEnv calls GET /api/v1/environments/{id}/env.
func (c *Client) GetEnvironmentEnv(ctx context.Context, id string) (map[string]string, error) {
	var out map[string]string
	err := c.do(ctx, http.MethodGet, environmentPath(id)+"/env", nil, &out)
	return out, err
}

// ListPreviewEnvironments calls GET /api/v1/apps/{name}/previews: every
// active pull-request preview for an app.
func (c *Client) ListPreviewEnvironments(ctx context.Context, appName string) ([]PreviewEnvironmentResource, error) {
	var out []PreviewEnvironmentResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(appName)+"/previews", nil, &out)
	return out, err
}

// TeardownPreviewEnvironment calls POST
// /api/v1/apps/{name}/previews/{number}/teardown: the manual safety net
// alongside a pull request's own automatic close-triggered teardown.
func (c *Client) TeardownPreviewEnvironment(ctx context.Context, appName string, prNumber int) error {
	path := fmt.Sprintf("/api/v1/apps/%s/previews/%d/teardown", PathEscape(appName), prNumber)
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

// SetPreviewEnabled calls PUT /api/v1/apps/{name}/preview-settings: the
// opt-in toggle for preview environments per pull request.
func (c *Client) SetPreviewEnabled(ctx context.Context, appName string, enabled bool) (SetPreviewEnabledRequest, error) {
	var out SetPreviewEnabledRequest
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(appName)+"/preview-settings", SetPreviewEnabledRequest{Enabled: enabled}, &out)
	return out, err
}

// SetEnvironmentEnv calls PUT /api/v1/environments/{id}/env: a full
// replace, mirroring PUT /apps/{name}'s own env field semantics.
func (c *Client) SetEnvironmentEnv(ctx context.Context, id string, vars map[string]string) (map[string]string, error) {
	var out map[string]string
	err := c.do(ctx, http.MethodPut, environmentPath(id)+"/env", vars, &out)
	return out, err
}

// SetAppEnvironment calls PUT /api/v1/apps/{name}/environment. An empty
// environmentID clears the assignment. Returns the updated app.
func (c *Client) SetAppEnvironment(ctx context.Context, name, environmentID string) (AppResource, error) {
	var out AppResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/environment", SetAppEnvironmentRequest{EnvironmentID: environmentID}, &out)
	return out, err
}

// SetAppProject calls PUT /api/v1/apps/{name}/project. An empty
// projectID clears the assignment. Returns the updated app.
func (c *Client) SetAppProject(ctx context.Context, name, projectID string) (AppResource, error) {
	var out AppResource
	err := c.do(ctx, http.MethodPut, "/api/v1/apps/"+PathEscape(name)+"/project", SetAppProjectRequest{ProjectID: projectID}, &out)
	return out, err
}

// SetDatabaseProject calls PUT /api/v1/databases/{name}/project. An
// empty projectID clears the assignment. Returns the updated database.
func (c *Client) SetDatabaseProject(ctx context.Context, name, projectID string) (DatabaseResource, error) {
	var out DatabaseResource
	err := c.do(ctx, http.MethodPut, "/api/v1/databases/"+PathEscape(name)+"/project", SetDatabaseProjectRequest{ProjectID: projectID}, &out)
	return out, err
}

func nodesCollectionPath() string {
	return "/api/v1/nodes"
}

func nodePath(id string) string {
	return nodesCollectionPath() + "/" + PathEscape(id)
}

// ListNodes calls GET /api/v1/nodes.
func (c *Client) ListNodes(ctx context.Context) ([]NodeResource, error) {
	var out []NodeResource
	err := c.do(ctx, http.MethodGet, nodesCollectionPath(), nil, &out)
	return out, err
}

// GetNode calls GET /api/v1/nodes/{id}.
func (c *Client) GetNode(ctx context.Context, id string) (NodeResource, error) {
	var out NodeResource
	err := c.do(ctx, http.MethodGet, nodePath(id), nil, &out)
	return out, err
}

// DeleteNode calls DELETE /api/v1/nodes/{id}. Refused with a 409
// (*APIError) while any service or database is still placed on id,
// telling the caller to drain first (handleDeleteNode's own doc comment).
func (c *Client) DeleteNode(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, nodePath(id), nil, nil)
}

// SetNodeWorkloads calls PUT /api/v1/nodes/{id}/workloads: a full
// replace of both workload flags, not a partial patch, matching
// handleSetNodeWorkloads' own contract.
func (c *Client) SetNodeWorkloads(ctx context.Context, id string, req SetNodeWorkloadsRequest) (NodeResource, error) {
	var out NodeResource
	err := c.do(ctx, http.MethodPut, nodePath(id)+"/workloads", req, &out)
	return out, err
}

// CreateNodeJoinToken calls POST /api/v1/nodes/join-tokens: mints a
// one-time enrollment token, returned in plaintext exactly once.
func (c *Client) CreateNodeJoinToken(ctx context.Context) (CreateNodeJoinTokenResponse, error) {
	var out CreateNodeJoinTokenResponse
	err := c.do(ctx, http.MethodPost, nodesCollectionPath()+"/join-tokens", nil, &out)
	return out, err
}

// CordonNode calls POST /api/v1/nodes/{id}/cordon: marks id
// unschedulable for new placements without evacuating anything already
// running there.
func (c *Client) CordonNode(ctx context.Context, id string) (NodeResource, error) {
	var out NodeResource
	err := c.do(ctx, http.MethodPost, nodePath(id)+"/cordon", nil, &out)
	return out, err
}

// UncordonNode calls POST /api/v1/nodes/{id}/uncordon: the inverse of
// CordonNode.
func (c *Client) UncordonNode(ctx context.Context, id string) (NodeResource, error) {
	var out NodeResource
	err := c.do(ctx, http.MethodPost, nodePath(id)+"/uncordon", nil, &out)
	return out, err
}

// DrainNode calls POST /api/v1/nodes/{id}/drain?target_node_id=: moves
// every service and database placed on id to targetNodeID (empty: the
// local-node sentinel, the server's own default).
func (c *Client) DrainNode(ctx context.Context, id, targetNodeID string) (DrainNodeResponse, error) {
	path := nodePath(id) + "/drain"
	if targetNodeID != "" {
		q := url.Values{}
		q.Set("target_node_id", targetNodeID)
		path += "?" + q.Encode()
	}
	var out DrainNodeResponse
	err := c.do(ctx, http.MethodPost, path, nil, &out)
	return out, err
}

// GetNodeHealth calls GET /api/v1/nodes/{id}/health: the node health
// controller's stored reconcile conditions, the same ConditionResource
// shape GetDeployStatus returns for an app.
func (c *Client) GetNodeHealth(ctx context.Context, id string) ([]ConditionResource, error) {
	var out []ConditionResource
	err := c.do(ctx, http.MethodGet, nodePath(id)+"/health", nil, &out)
	return out, err
}

// GetNodePatchStatus calls GET /api/v1/nodes/{id}/patch-status: the
// latest OS-patch reading HostPatchCollector wrote for id, or
// Checked == false if it has never checked (unsupported package manager,
// or the collector hasn't run yet).
func (c *Client) GetNodePatchStatus(ctx context.Context, id string) (NodePatchStatusResource, error) {
	var out NodePatchStatusResource
	err := c.do(ctx, http.MethodGet, nodePath(id)+"/patch-status", nil, &out)
	return out, err
}

// GetSystemStatus calls GET /api/v1/system/status: this control plane's
// own configured/not-configured signals, including local Docker daemon
// reachability (DockerConnected/DockerError).
func (c *Client) GetSystemStatus(ctx context.Context) (SystemStatusResource, error) {
	var out SystemStatusResource
	err := c.do(ctx, http.MethodGet, "/api/v1/system/status", nil, &out)
	return out, err
}

// GetSystemDoctor calls GET /api/v1/system/doctor: the "levelrail-cli
// doctor" preflight bundle, a superset of GetSystemStatus above.
func (c *Client) GetSystemDoctor(ctx context.Context) (SystemDoctorResource, error) {
	var out SystemDoctorResource
	err := c.do(ctx, http.MethodGet, "/api/v1/system/doctor", nil, &out)
	return out, err
}

// ListAlertRules calls GET /api/v1/apps/{name}/alerts: every alert rule
// scoped to name, including disabled ones.
func (c *Client) ListAlertRules(ctx context.Context, name string) ([]AlertRuleResource, error) {
	var out []AlertRuleResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/alerts", nil, &out)
	return out, err
}

// QueryAppMetrics calls GET /api/v1/apps/{name}/metrics?metric=&from=&to=&step=
// (internal/api/metrics.go's handleQueryMetrics). from/to are sent as
// RFC3339; step of zero omits the query param, matching the server's own
// "step<=0 means raw unaggregated samples" contract (telemetry.Aggregate).
func (c *Client) QueryAppMetrics(ctx context.Context, name, metric string, from, to time.Time, step time.Duration) (AppMetricsResource, error) {
	query := url.Values{}
	query.Set("metric", metric)
	query.Set("from", from.UTC().Format(time.RFC3339))
	query.Set("to", to.UTC().Format(time.RFC3339))
	if step > 0 {
		query.Set("step", step.String())
	}
	var out AppMetricsResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+PathEscape(name)+"/metrics?"+query.Encode(), nil, &out)
	return out, err
}

// PathEscape guards against a name containing characters that would
// otherwise change the request's URL shape (a "/" turning one path
// segment into two, for instance). Server-side validation is the real
// authority on what a valid app name is; this only protects request
// construction from building a request to the wrong path. Exported for
// the same reuse reason as ExtractErrorMessage.
func PathEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "%", "%25"), "/", "%2F")
}
