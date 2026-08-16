package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// serviceResources and serviceHealth/serviceProbe mirror
// internal/api's appResource wire shape (internal/api/apps.go), which is
// itself store.DesiredService's own JSON encoding: bytes and
// nanoseconds, not app.yaml's human-friendly "512Mi"/"5s" strings.
// Redeclared here, not imported from internal/store or internal/api, so
// this binary depends only on the documented wire contract, never on the
// control plane's internal Go types: the CLI and a future MCP server are
// exactly the external callers that contract exists for.
type serviceResources struct {
	MemoryBytes int64 `json:"memory_bytes,omitempty"`
	NanoCPUs    int64 `json:"nano_cpus,omitempty"`
}

type serviceProbe struct {
	Path     string `json:"path"`
	Interval int64  `json:"interval,omitempty"`
	Timeout  int64  `json:"timeout,omitempty"`
	Failures int    `json:"failures,omitempty"`
}

type serviceHealth struct {
	Readiness *serviceProbe `json:"readiness,omitempty"`
	Liveness  *serviceProbe `json:"liveness,omitempty"`
}

// appResource mirrors internal/api's appResource (apps.go). Field order
// and JSON tags match exactly, so a response decodes cleanly and a
// request encodes into exactly what the server expects.
type appResource struct {
	Name      string            `json:"name"`
	Image     string            `json:"image"`
	Port      int               `json:"port"`
	Domains   []string          `json:"domains,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Resources *serviceResources `json:"resources,omitempty"`
	Health    *serviceHealth    `json:"health,omitempty"`
	NodeID    string            `json:"node_id,omitempty"`
}

// buildTriggerRequest mirrors internal/api's triggerBuildRequest
// (internal/api/builds.go). Ref is required server-side (handleTriggerBuild
// rejects an empty one with 400), so every caller of TriggerBuild must
// resolve a real ref before sending this, never leave it blank.
type buildTriggerRequest struct {
	RepoURL   string                   `json:"repo_url"`
	Ref       string                   `json:"ref"`
	ImageRepo string                   `json:"image_repo,omitempty"`
	Build     buildTriggerRequestBuild `json:"build,omitempty"`
}

// buildTriggerRequestBuild is triggerBuildRequest's nested build.* input
// (internal/api/builds.go's triggerBuildBuildInput), matching the same
// two fields app.yaml's own build: block has. Type left empty defaults
// server-side to a Dockerfile build, the only kind internal/deploy.Pipeline
// actually performs today.
type buildTriggerRequestBuild struct {
	Type string `json:"type,omitempty"`
	Path string `json:"path,omitempty"`
}

// buildTriggerResponse mirrors internal/api's triggerBuildResponse: the
// real built image tag plus the app's full, now-updated resource (its
// desired_services.image row was overwritten by the build, per
// pendingImageTag's own doc comment above).
type buildTriggerResponse struct {
	Image string      `json:"image"`
	App   appResource `json:"app"`
}

// deployTriggerRequest mirrors internal/api's deployTriggerRequest
// (internal/api/deploys.go). Response is a plain appResource (the app's
// now-updated desired state), the same shape CreateApp/GetApp already
// use, so no separate response type is needed here.
type deployTriggerRequest struct {
	Image string `json:"image"`
}

// conditionResource mirrors internal/reconcile.Condition, the wire shape
// both GET /api/v1/apps/{name}/deploys (internal/api/deploys.go's
// handleDeployHistory) and GET /api/v1/databases/{name}/status
// (internal/api/databases.go's handleDatabaseStatus) return. The source
// type carries no json struct tags, so its field names are the wire
// field names verbatim; the tags here just make that explicit rather
// than relying on encoding/json's case-insensitive match.
type conditionResource struct {
	Type               string    `json:"Type"`
	Status             string    `json:"Status"`
	Reason             string    `json:"Reason"`
	Message            string    `json:"Message"`
	LastTransitionTime time.Time `json:"LastTransitionTime"`
}

// logEntryResource mirrors internal/api's logEntryResource
// (internal/api/logs.go).
type logEntryResource struct {
	Timestamp  time.Time       `json:"timestamp"`
	Stream     string          `json:"stream"`
	Message    string          `json:"message"`
	Structured bool            `json:"structured"`
	FieldsJSON json.RawMessage `json:"fields,omitempty"`
}

// logsResponse mirrors internal/api's logsResponse (internal/api/logs.go).
type logsResponse struct {
	Entries []logEntryResource `json:"entries"`
}

// execRequest mirrors internal/api's execRequest (internal/api/exec.go).
// Command is required server-side and is never shell-interpreted: a
// caller who wants shell features (pipes, redirection, env expansion)
// passes Command: "sh", Args: []string{"-c", "..."} explicitly, the same
// contract the server documents.
type execRequest struct {
	Command        string   `json:"command"`
	Args           []string `json:"args,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

// execResponse mirrors internal/api's execResponse. ExitCode is always
// the remote command's real exit code, never a CLI-level sentinel: "apps
// exec" (apps_exec.go) exits the whole process with this value, which is
// the entire point of the command.
type execResponse struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr,omitempty"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated,omitempty"`
}

// domainResource mirrors internal/api's domainResource
// (internal/api/ingress_settings.go).
type domainResource struct {
	Domain      string `json:"domain"`
	ServiceName string `json:"service_name"`
}

// backupHistoryResource mirrors internal/api's backupHistoryResource
// (internal/api/backups.go).
type backupHistoryResource struct {
	ID           string `json:"id"`
	DatabaseName string `json:"database_name"`
	TargetID     string `json:"target_id"`
	ObjectKey    string `json:"object_key"`
	SizeBytes    int64  `json:"size_bytes"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
	StartedAt    string `json:"started_at"`
	FinishedAt   string `json:"finished_at,omitempty"`
}

// triggerBackupRequest mirrors internal/api's triggerBackupRequest.
type triggerBackupRequest struct {
	TargetID string `json:"target_id"`
}

// restoreHistoryResource mirrors internal/api's restoreHistoryResource
// (internal/api/restore.go).
type restoreHistoryResource struct {
	ID              string `json:"id"`
	DatabaseName    string `json:"database_name"`
	BackupHistoryID string `json:"backup_history_id"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at,omitempty"`
}

// triggerRestoreRequest mirrors internal/api's triggerRestoreRequest.
type triggerRestoreRequest struct {
	BackupID string `json:"backup_id"`
}

// sessionInfoResource mirrors internal/api's sessionInfoResponse
// (internal/api/account.go)'s wire shape. GetSession below sends this
// over a plain bearer-token request, which handleGetSession's own doc
// comment says it will never honor ("deliberately not requireAbility: a
// bearer token has no session of its own to report on"); see
// auth_whoami.go's own doc comment for the consequence.
type sessionInfoResource struct {
	Username  string `json:"username"`
	ExpiresAt string `json:"expires_at"`
}

// databaseResource mirrors internal/api's databaseResource
// (internal/api/databases.go). NodeID is response-only, the same
// "shown but not settable through this endpoint" boundary appResource's
// own NodeID field already documents: there is no CLI-facing way to set
// it, matching there being no PUT .../databases/{name}/node command
// here either.
type databaseResource struct {
	Name    string `json:"name"`
	Engine  string `json:"engine"`
	Version string `json:"version"`
	NodeID  string `json:"node_id,omitempty"`
}

// apiErrorBody is the JSON shape every non-2xx response from the
// control plane returns (internal/api/respond.go's own apiError).
type apiErrorBody struct {
	Error string `json:"error"`
}

// apiError is returned by Client methods for a non-2xx HTTP response: a
// real reply from the server, as opposed to a network-level failure
// (connection refused, DNS, timeout), which Client methods return as a
// plain wrapped error instead. Callers distinguish the two with
// errors.As, which is what picks the CLI's exit code.
type apiError struct {
	StatusCode int
	Message    string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("server returned %d: %s", e.StatusCode, e.Message)
}

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

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody) //nolint:gosec // c.baseURL is the operator-supplied --api-url/APP_API_URL target this whole command exists to call, not attacker-controlled input; that's the CLI's entire job, not an SSRF finding
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.hc.Do(req) //nolint:gosec // same target as above, the request this CLI was invoked to make
	if err != nil {
		// A transport-level failure (connection refused, DNS, TLS,
		// timeout): deliberately not wrapped in *apiError, so callers
		// can tell "never reached the server" apart from "the server
		// answered with an error" via errors.As, and pick a different
		// exit code for each.
		return fmt.Errorf("request %s %s: %w", method, c.baseURL+path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &apiError{StatusCode: resp.StatusCode, Message: extractErrorMessage(data)}
	}

	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response body: %w", err)
		}
	}
	return nil
}

// extractErrorMessage parses the control plane's {"error": "..."} body
// shape (internal/api/respond.go). A body that doesn't match that shape
// (a proxy's HTML error page, an empty body) falls back to the raw text
// so the caller still sees something, not a blank message.
func extractErrorMessage(data []byte) string {
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
func (c *Client) CreateApp(ctx context.Context, req appResource) (appResource, error) {
	var out appResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps", req, &out)
	return out, err
}

// GetApp calls GET /api/v1/apps/{name}.
func (c *Client) GetApp(ctx context.Context, name string) (appResource, error) {
	var out appResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+pathEscape(name), nil, &out)
	return out, err
}

// ListApps calls GET /api/v1/apps.
func (c *Client) ListApps(ctx context.Context) ([]appResource, error) {
	var out []appResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps", nil, &out)
	return out, err
}

// TriggerBuild calls POST /api/v1/apps/{name}/builds.
func (c *Client) TriggerBuild(ctx context.Context, name string, req buildTriggerRequest) (buildTriggerResponse, error) {
	var out buildTriggerResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+pathEscape(name)+"/builds", req, &out)
	return out, err
}

// DeployApp calls POST /api/v1/apps/{name}/deploys: points the app's
// desired image at image. Asynchronous, same as the server-side
// handler's own doc comment describes: this returns as soon as the
// desired state is saved, not once a container is actually running it.
// Also how a rollback is done: deploying an older, already-known tag
// with this same method. "apps rollback" (apps_rollback.go) is a thin,
// discoverability-only wrapper around this exact method, the same
// framing the web frontend's DeployAttemptsList.tsx "Rollback to this
// build" button already uses over useTriggerDeploy: there is no
// separate rollback endpoint or request shape, only a different CLI
// command name and message for an operator who wants to type "rollback"
// rather than "deploy --image <older-tag>".
func (c *Client) DeployApp(ctx context.Context, name, image string) (appResource, error) {
	var out appResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+pathEscape(name)+"/deploys", deployTriggerRequest{Image: image}, &out)
	return out, err
}

// RestartApp calls POST /api/v1/apps/{name}/restart: force a running
// container to be recreated with no image change (internal/api/apps.go's
// own handleRestartApp). No request body; the response is the app's
// current desired state, unchanged, the same appResource shape every
// other app-mutating endpoint in this file returns.
func (c *Client) RestartApp(ctx context.Context, name string) (appResource, error) {
	var out appResource
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+pathEscape(name)+"/restart", nil, &out)
	return out, err
}

// GetDeployStatus calls GET /api/v1/apps/{name}/deploys: the application
// controller's current stored reconcile conditions, not a deploy history
// log (internal/api/deploys.go's own handleDeployHistory doc comment).
func (c *Client) GetDeployStatus(ctx context.Context, name string) ([]conditionResource, error) {
	var out []conditionResource
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+pathEscape(name)+"/deploys", nil, &out)
	return out, err
}

// QueryLogs calls GET /api/v1/apps/{name}/logs?from=&to=&q=
// (internal/api/logs.go's handleQueryLogs): a real, historical full-text
// search over already-stored log entries. from/to are sent as RFC3339,
// the same format the server requires (internal/api's parseTimeRange);
// q empty means every entry in the window, matching the server's own
// "empty query" contract.
func (c *Client) QueryLogs(ctx context.Context, name string, from, to time.Time, q string) ([]logEntryResource, error) {
	query := url.Values{}
	query.Set("from", from.UTC().Format(time.RFC3339))
	query.Set("to", to.UTC().Format(time.RFC3339))
	if q != "" {
		query.Set("q", q)
	}
	var out logsResponse
	err := c.do(ctx, http.MethodGet, "/api/v1/apps/"+pathEscape(name)+"/logs?"+query.Encode(), nil, &out)
	return out.Entries, err
}

// CreateDatabase calls POST /api/v1/databases.
func (c *Client) CreateDatabase(ctx context.Context, req databaseResource) (databaseResource, error) {
	var out databaseResource
	err := c.do(ctx, http.MethodPost, "/api/v1/databases", req, &out)
	return out, err
}

// GetDatabase calls GET /api/v1/databases/{name}.
func (c *Client) GetDatabase(ctx context.Context, name string) (databaseResource, error) {
	var out databaseResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+pathEscape(name), nil, &out)
	return out, err
}

// ListDatabases calls GET /api/v1/databases.
func (c *Client) ListDatabases(ctx context.Context) ([]databaseResource, error) {
	var out []databaseResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases", nil, &out)
	return out, err
}

// ExecApp calls POST /api/v1/apps/{name}/exec: runs command (plus args)
// inside the app's currently running container and returns its
// stdout/stderr/exit code once it finishes. AbilityRoot-gated
// server-side (internal/api/exec.go's own doc comment: secrets are
// injected as plaintext env vars, so exec can read them anyway, and
// must sit behind the tier that boundary implies).
func (c *Client) ExecApp(ctx context.Context, name string, req execRequest) (execResponse, error) {
	var out execResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/apps/"+pathEscape(name)+"/exec", req, &out)
	return out, err
}

// ListDomains calls GET /api/v1/domains: every service_domains row
// across every app, aggregated in one call.
func (c *Client) ListDomains(ctx context.Context) ([]domainResource, error) {
	var out []domainResource
	err := c.do(ctx, http.MethodGet, "/api/v1/domains", nil, &out)
	return out, err
}

// TriggerBackup calls POST /api/v1/databases/{name}/backups: starts a
// real backup of name to targetID and returns as soon as the attempt is
// recorded and under way (StatusAccepted server-side), not once the
// dump and upload actually finish. ListBackups is how a caller finds
// out whether it did.
func (c *Client) TriggerBackup(ctx context.Context, name, targetID string) (backupHistoryResource, error) {
	var out backupHistoryResource
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+pathEscape(name)+"/backups", triggerBackupRequest{TargetID: targetID}, &out)
	return out, err
}

// ListBackups calls GET /api/v1/databases/{name}/backups: the full
// backup attempt history for one database.
func (c *Client) ListBackups(ctx context.Context, name string) ([]backupHistoryResource, error) {
	var out []backupHistoryResource
	err := c.do(ctx, http.MethodGet, "/api/v1/databases/"+pathEscape(name)+"/backups", nil, &out)
	return out, err
}

// TriggerRestore calls POST /api/v1/databases/{name}/restore: overwrites
// name's live data in place from a previously succeeded backup attempt.
// The single most destructive call this Client makes; "backups restore"
// (backups_restore.go) is the only caller and requires its own explicit
// confirmation before ever reaching this method.
func (c *Client) TriggerRestore(ctx context.Context, name, backupID string) (restoreHistoryResource, error) {
	var out restoreHistoryResource
	err := c.do(ctx, http.MethodPost, "/api/v1/databases/"+pathEscape(name)+"/restore", triggerRestoreRequest{BackupID: backupID}, &out)
	return out, err
}

// GetSession calls GET /api/v1/auth/session using this Client's bearer
// token, exactly the same auth mechanism every other method on this
// type uses. See auth_whoami.go's own doc comment: the server gates
// this route with requireAuth, session-cookie-only, by explicit design
// (internal/api/account.go's handleGetSession doc comment), so a call
// made through this method will reach the server and get back a real,
// honest 401 "authentication required" for any caller authenticated the
// way every other command in this CLI is, not a client-side bug.
func (c *Client) GetSession(ctx context.Context) (sessionInfoResource, error) {
	var out sessionInfoResource
	err := c.do(ctx, http.MethodGet, "/api/v1/auth/session", nil, &out)
	return out, err
}

// setSecretRequest mirrors internal/api's setSecretRequest
// (internal/api/secrets.go).
type setSecretRequest struct {
	Value string `json:"value"`
}

// SetSecret calls PUT /api/v1/apps/{name}/secrets/{key}. No response
// body beyond the status (internal/api/secrets.go's handleSetSecret
// returns 204 on success), matching that handler's own doc comment on
// why a secret's value is never echoed back.
func (c *Client) SetSecret(ctx context.Context, name, key, value string) error {
	return c.do(ctx, http.MethodPut, "/api/v1/apps/"+pathEscape(name)+"/secrets/"+pathEscape(key), setSecretRequest{Value: value}, nil)
}

// pathEscape guards against a name containing characters that would
// otherwise change the request's URL shape (a "/" turning one path
// segment into two, for instance). Server-side validation is the real
// authority on what a valid app name is; this only protects the CLI's
// own request construction from building a request to the wrong path.
func pathEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "%", "%25"), "/", "%2F")
}
