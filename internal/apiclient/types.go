package apiclient

import (
	"encoding/json"
	"fmt"
	"time"
)

// ServiceResources mirrors internal/api's appResource.Resources field
// (internal/api/apps.go), which is itself store.DesiredService's own
// JSON encoding: bytes and nanoseconds, not app.yaml's human-friendly
// "512Mi"/"5s" strings. Redeclared here, not imported from internal/store
// or internal/api, so this package depends only on the documented wire
// contract, never on the control plane's internal Go types. ServiceProbe
// and ServiceHealth below share that same reasoning.
type ServiceResources struct {
	MemoryBytes     int64  `json:"memory_bytes,omitempty"`
	NanoCPUs        int64  `json:"nano_cpus,omitempty"`
	SwapMemoryBytes int64  `json:"swap_memory_bytes,omitempty"`
	CPUSetCPUs      string `json:"cpuset_cpus,omitempty"`
}

// ServiceProbe mirrors one of AppResource.Health's two probes (readiness
// or liveness).
type ServiceProbe struct {
	Path     string `json:"path"`
	Interval int64  `json:"interval,omitempty"`
	Timeout  int64  `json:"timeout,omitempty"`
	Failures int    `json:"failures,omitempty"`
}

// ServiceHealth mirrors internal/api's appResource.Health field.
type ServiceHealth struct {
	Readiness *ServiceProbe `json:"readiness,omitempty"`
	Liveness  *ServiceProbe `json:"liveness,omitempty"`
}

// AppResource mirrors internal/api's appResource (apps.go). Field order
// and JSON tags match exactly, so a response decodes cleanly and a
// request encodes into exactly what the server expects.
type AppResource struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	Port  int    `json:"port"`
	// HostPort mirrors internal/api's appResource.HostPort: nil means
	// "let Docker assign one", a value pins the host-side port. Settable
	// on create and update, like Port.
	HostPort  *int              `json:"host_port,omitempty"`
	Domains   []string          `json:"domains,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Resources *ServiceResources `json:"resources,omitempty"`
	Health    *ServiceHealth    `json:"health,omitempty"`
	NodeID    string            `json:"node_id,omitempty"`
	// EnvironmentID mirrors internal/api's appResource.EnvironmentID:
	// response-only, set via PUT /api/v1/apps/{name}/environment.
	EnvironmentID string `json:"environment_id,omitempty"`
}

// LogDrainResource mirrors internal/api's logDrainResource
// (apps_log_drain.go): GET/PUT /api/v1/apps/{name}/log-drain's wire
// shape. Type is "http" or "syslog", validated server-side.
type LogDrainResource struct {
	AppName string `json:"app_name"`
	Type    string `json:"type"`
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
}

// SetLogDrainRequest mirrors internal/api's setLogDrainRequest.
type SetLogDrainRequest struct {
	Type    string `json:"type"`
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
}

// BuildTriggerRequest mirrors internal/api's triggerBuildRequest
// (internal/api/builds.go). Ref is required server-side (handleTriggerBuild
// rejects an empty one with 400), so every caller of TriggerBuild must
// resolve a real ref before sending this, never leave it blank.
type BuildTriggerRequest struct {
	RepoURL   string                   `json:"repo_url"`
	Ref       string                   `json:"ref"`
	ImageRepo string                   `json:"image_repo,omitempty"`
	Build     BuildTriggerRequestBuild `json:"build,omitempty"`
}

// BuildTriggerRequestBuild is BuildTriggerRequest's nested build.* input
// (internal/api/builds.go's triggerBuildBuildInput), matching the same
// fields app.yaml's own build: block has. Type left empty defaults
// server-side to a Dockerfile build. Image is only meaningful for
// Type == "image": a prebuilt registry reference deployed as-is, no
// repo/ref/build needed.
type BuildTriggerRequestBuild struct {
	Type string `json:"type,omitempty"`
	Path string `json:"path,omitempty"`
	// BaseDirectory scopes the build context to a subdirectory of the
	// repo, for a monorepo. Not meaningful for Type == "image".
	BaseDirectory string `json:"base_directory,omitempty"`
	Image         string `json:"image,omitempty"`
	// Args are Dockerfile build-time ARG values. Only meaningful for
	// Type == "dockerfile".
	Args map[string]string `json:"args,omitempty"`
}

// BuildTriggerResponse mirrors internal/api's triggerBuildResponse: the
// real built image tag plus the app's full, now-updated resource.
type BuildTriggerResponse struct {
	Image string      `json:"image"`
	App   AppResource `json:"app"`
}

// DeployTriggerRequest mirrors internal/api's deployTriggerRequest
// (internal/api/deploys.go). Response is a plain AppResource (the app's
// now-updated desired state), the same shape CreateApp/GetApp already
// use, so no separate response type is needed here.
type DeployTriggerRequest struct {
	Image string `json:"image"`
}

// ComposeDeployResult mirrors internal/api's composeDeployResponse
// (internal/api/apps_compose.go).
type ComposeDeployResult struct {
	AppID    string        `json:"app_id"`
	Services []AppResource `json:"services"`
}

// ConditionResource mirrors internal/reconcile.Condition, the wire shape
// both GET /api/v1/apps/{name}/deploys (internal/api/deploys.go's
// handleDeployHistory) and GET /api/v1/databases/{name}/status
// (internal/api/databases.go's handleDatabaseStatus) return. The source
// type carries no json struct tags, so its field names are the wire
// field names verbatim; the tags here just make that explicit rather
// than relying on encoding/json's case-insensitive match.
type ConditionResource struct {
	Type               string    `json:"Type"`
	Status             string    `json:"Status"`
	Reason             string    `json:"Reason"`
	Message            string    `json:"Message"`
	LastTransitionTime time.Time `json:"LastTransitionTime"`
}

// NetworkResource mirrors internal/api's networkResource
// (internal/api/network.go): the live traffic path, container's declared
// port plus whatever host port Docker currently has bound.
type NetworkResource struct {
	ContainerPort int  `json:"container_port"`
	HostPort      int  `json:"host_port,omitempty"`
	Running       bool `json:"running"`
}

// LogEntryResource mirrors internal/api's logEntryResource
// (internal/api/logs.go).
type LogEntryResource struct {
	Timestamp  time.Time       `json:"timestamp"`
	Stream     string          `json:"stream"`
	Message    string          `json:"message"`
	Structured bool            `json:"structured"`
	FieldsJSON json.RawMessage `json:"fields,omitempty"`
}

// logsResponse mirrors internal/api's logsResponse (internal/api/logs.go).
type logsResponse struct {
	Entries []LogEntryResource `json:"entries"`
}

// ExecRequest mirrors internal/api's execRequest (internal/api/exec.go).
// Command is required server-side and is never shell-interpreted: a
// caller who wants shell features (pipes, redirection, env expansion)
// passes Command: "sh", Args: []string{"-c", "..."} explicitly, the same
// contract the server documents.
type ExecRequest struct {
	Command        string   `json:"command"`
	Args           []string `json:"args,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}

// ExecResponse mirrors internal/api's execResponse. ExitCode is always
// the remote command's real exit code, never a client-level sentinel.
type ExecResponse struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr,omitempty"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated,omitempty"`
}

// DomainResource mirrors internal/api's domainResource
// (internal/api/ingress_settings.go).
type DomainResource struct {
	Domain      string `json:"domain"`
	ServiceName string `json:"service_name"`
}

// CloudflareDNSResource mirrors internal/api's cloudflareDNSResource
// (internal/api/cloudflare_dns.go): GET/PUT/DELETE
// /api/v1/settings/cloudflare-dns's wire shape. The token itself never
// appears here in either direction.
type CloudflareDNSResource struct {
	Enabled  bool `json:"enabled"`
	HasToken bool `json:"has_token"`
}

// UpdateCloudflareDNSRequest mirrors internal/api's
// updateCloudflareDNSRequest. Token empty on an update means "leave the
// currently stored token unchanged".
type UpdateCloudflareDNSRequest struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token,omitempty"`
}

// CloudflareTunnelResource mirrors internal/api's cloudflareTunnelResource
// (internal/api/cloudflare_tunnel.go): GET/PUT/DELETE
// /api/v1/settings/cloudflare-tunnel's wire shape. The token itself
// never appears here in either direction. A distinct credential and
// endpoint from CloudflareDNSResource: this one runs the cloudflared
// container, that one configures ACME's DNS-01 challenge.
type CloudflareTunnelResource struct {
	Enabled  bool   `json:"enabled"`
	HasToken bool   `json:"has_token"`
	Status   string `json:"status"`
	Message  string `json:"message,omitempty"`
}

// UpdateCloudflareTunnelRequest mirrors internal/api's
// updateCloudflareTunnelRequest. Token empty on an update means "leave
// the currently stored token unchanged".
type UpdateCloudflareTunnelRequest struct {
	Enabled bool   `json:"enabled"`
	Token   string `json:"token,omitempty"`
}

// DomainBasicAuthResource mirrors internal/api's domainBasicAuthResource
// (internal/api/domain_basic_auth.go): GET/PUT/DELETE
// /api/v1/apps/{name}/domains/{domain}/auth's wire shape. The password
// itself never appears here in either direction.
type DomainBasicAuthResource struct {
	Domain      string `json:"domain"`
	Enabled     bool   `json:"enabled"`
	Username    string `json:"username,omitempty"`
	HasPassword bool   `json:"has_password"`
}

// SetDomainBasicAuthRequest mirrors internal/api's
// setDomainBasicAuthRequest. Password empty on an update means "leave
// the currently stored password unchanged".
type SetDomainBasicAuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

// BackupHistoryResource mirrors internal/api's backupHistoryResource
// (internal/api/backups.go).
type BackupHistoryResource struct {
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

// TriggerBackupRequest mirrors internal/api's triggerBackupRequest.
type TriggerBackupRequest struct {
	TargetID string `json:"target_id"`
}

// BackupScheduleResource mirrors internal/api's backupScheduleResource
// (internal/api/backups.go): PUT/DELETE .../backup-schedule only ever
// touch these three fields, never the full database resource.
type BackupScheduleResource struct {
	DatabaseName string `json:"database_name"`
	TargetID     string `json:"target_id,omitempty"`
	Schedule     string `json:"schedule,omitempty"`
	Retain       int    `json:"retain,omitempty"`
	RetainDays   int    `json:"retain_days,omitempty"`
}

// SetBackupScheduleRequest mirrors internal/api's setBackupScheduleRequest.
type SetBackupScheduleRequest struct {
	TargetID   string `json:"target_id"`
	Schedule   string `json:"schedule"`
	Retain     int    `json:"retain,omitempty"`
	RetainDays int    `json:"retain_days,omitempty"`
}

// ListBackupsOptions is ListBackups' pagination input, mirroring the
// server's ?limit/?before query params. Zero values omit the param
// (Limit <= 0 uses the server default, Before == "" is the first page).
type ListBackupsOptions struct {
	Limit  int
	Before string
}

// RestoreHistoryResource mirrors internal/api's restoreHistoryResource
// (internal/api/restore.go).
type RestoreHistoryResource struct {
	ID              string `json:"id"`
	DatabaseName    string `json:"database_name"`
	BackupHistoryID string `json:"backup_history_id"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at,omitempty"`
}

// TriggerRestoreRequest mirrors internal/api's triggerRestoreRequest.
type TriggerRestoreRequest struct {
	BackupID string `json:"backup_id"`
}

// SessionInfoResource mirrors internal/api's sessionInfoResponse
// (internal/api/account.go)'s wire shape. GetSession below sends this
// over a plain bearer-token request, which handleGetSession's own doc
// comment says it will never honor ("deliberately not requireAbility: a
// bearer token has no session of its own to report on").
type SessionInfoResource struct {
	Username  string `json:"username"`
	ExpiresAt string `json:"expires_at"`
}

// DatabaseResource mirrors internal/api's databaseResource
// (internal/api/databases.go). NodeID is response-only, the same
// "shown but not settable through this endpoint" boundary AppResource's
// own NodeID field already documents.
type DatabaseResource struct {
	Name    string `json:"name"`
	Engine  string `json:"engine"`
	Version string `json:"version"`
	NodeID  string `json:"node_id,omitempty"`
	// Resources, PubliclyAccessible, PublicPort, and the Backup* fields
	// are set through their own dedicated routes (SetDatabaseResources,
	// SetDatabasePublicAccess, SetBackupSchedule), never through this
	// resource's own create/update body; see internal/api's
	// databaseResource for the identical boundary server-side.
	Resources          *ServiceResources `json:"resources,omitempty"`
	PubliclyAccessible bool              `json:"publicly_accessible,omitempty"`
	PublicPort         int               `json:"public_port,omitempty"`
	BackupTargetID     string            `json:"backup_target_id,omitempty"`
	BackupSchedule     string            `json:"backup_schedule,omitempty"`
	BackupRetain       int               `json:"backup_retain,omitempty"`
	BackupRetainDays   int               `json:"backup_retain_days,omitempty"`
}

// DatabaseEngineResource mirrors internal/api's databaseEngineResource
// (internal/api/database_engines.go): one entry from
// GET /api/v1/database-engines, the dynamic registry backing the
// creation wizard's engine picker.
type DatabaseEngineResource struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	DefaultVersion string `json:"default_version"`
}

// SetDatabaseResourcesRequest mirrors internal/api's
// setDatabaseResourcesRequest.
type SetDatabaseResourcesRequest struct {
	Resources *ServiceResources `json:"resources"`
}

// DatabasePublicAccessResource mirrors internal/api's
// databasePublicAccessResource (internal/api/database_public_access.go):
// PUT/DELETE .../public-access only ever touch these three fields.
type DatabasePublicAccessResource struct {
	DatabaseName       string `json:"database_name"`
	PubliclyAccessible bool   `json:"publicly_accessible"`
	PublicPort         int    `json:"public_port,omitempty"`
}

// SetDatabasePublicAccessRequest mirrors internal/api's
// setDatabasePublicAccessRequest. Port 0 means auto-assign the next
// free port.
type SetDatabasePublicAccessRequest struct {
	Port int `json:"port,omitempty"`
}

// ServiceTemplateListItem mirrors internal/api's serviceTemplateListItem
// (internal/api/service_templates.go): one catalog entry from
// GET /api/v1/service-templates, without the full Compose body.
type ServiceTemplateListItem struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Slogan           string `json:"slogan"`
	Category         string `json:"category"`
	DocumentationURL string `json:"documentation_url"`
}

// ServiceTemplateDetail mirrors internal/api's serviceTemplateDetail:
// GET /api/v1/service-templates/{id}'s response, including the full
// compose.yaml body.
type ServiceTemplateDetail struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Slogan           string `json:"slogan"`
	Category         string `json:"category"`
	DocumentationURL string `json:"documentation_url"`
	Compose          string `json:"compose"`
}

// SetSecretRequest mirrors internal/api's setSecretRequest
// (internal/api/secrets.go).
type SetSecretRequest struct {
	Value           string `json:"value"`
	OverwriteLocked bool   `json:"overwrite_locked"`
}

// SecretKeyResource mirrors internal/api's secretKeyResource
// (internal/api/secrets.go): a secret's key and locked state, never its
// value.
type SecretKeyResource struct {
	Key    string `json:"key"`
	Locked bool   `json:"locked"`
}

// SetSecretLockRequest mirrors internal/api's setSecretLockRequest
// (internal/api/secrets.go).
type SetSecretLockRequest struct {
	Locked bool `json:"locked"`
}

// GitSourceResource mirrors internal/api's gitSourceResource
// (internal/api/git_sources.go). AdditionalServices/Services (the
// multi-service fan-out fields) are deliberately not carried here: this
// client's SetGitSourceRequest only covers the single-service connect
// flow "apps deploy-spec" already handles for the multi-service case.
type GitSourceResource struct {
	ServiceName    string `json:"service_name"`
	RepoURL        string `json:"repo_url"`
	Branch         string `json:"branch"`
	BuildType      string `json:"build_type"`
	BuildPath      string `json:"build_path,omitempty"`
	HasToken       bool   `json:"has_token"`
	WebhookURL     string `json:"webhook_url"`
	WebhookSecret  string `json:"webhook_secret,omitempty"`
	PreviewEnabled bool   `json:"preview_enabled"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// SetGitSourceRequest mirrors internal/api's setGitSourceRequest
// (internal/api/git_sources.go), minus the multi-service Services/
// AdditionalServices fields (see GitSourceResource's own doc comment).
type SetGitSourceRequest struct {
	RepoURL   string `json:"repo_url"`
	Branch    string `json:"branch,omitempty"`
	BuildType string `json:"build_type,omitempty"`
	BuildPath string `json:"build_path,omitempty"`
	Token     string `json:"token,omitempty"`
}

// NotificationChannelResource mirrors internal/api's
// notificationChannelResource (internal/api/notification_channels.go).
type NotificationChannelResource struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	NotifyURL string `json:"notify_url"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// CreateNotificationChannelRequest mirrors internal/api's
// createNotificationChannelRequest.
type CreateNotificationChannelRequest struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	NotifyURL string `json:"notify_url"`
	Enabled   *bool  `json:"enabled,omitempty"`
}

// TestNotificationChannelRequest mirrors internal/api's
// testNotificationChannelRequest.
type TestNotificationChannelRequest struct {
	Kind      string `json:"kind"`
	NotifyURL string `json:"notify_url"`
}

// ScheduledTaskResource mirrors internal/api's scheduledTaskResource
// (internal/api/scheduled_tasks.go). Command is a real argv (no shell),
// and LastRun* describe only the single most recent run in place: there
// is no separate run-history endpoint.
type ScheduledTaskResource struct {
	ID          string   `json:"id,omitempty"`
	ServiceName string   `json:"service_name,omitempty"`
	Command     []string `json:"command"`
	Schedule    string   `json:"schedule"`
	Enabled     bool     `json:"enabled"`

	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	LastRunStatus string     `json:"last_run_status,omitempty"`
	LastRunOutput string     `json:"last_run_output,omitempty"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// ScheduledTaskRequest mirrors the fields internal/api's
// scheduledTaskResource actually reads from a create/update request
// body (Command, Schedule, Enabled); ID and ServiceName always come
// from the URL, never the body.
type ScheduledTaskRequest struct {
	Command  []string `json:"command"`
	Schedule string   `json:"schedule"`
	Enabled  bool     `json:"enabled"`
}

// OrganizationResource mirrors internal/api's organizationResource
// (internal/api/organizations.go).
type OrganizationResource struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// CreateOrganizationRequest mirrors internal/api's
// createOrganizationRequest.
type CreateOrganizationRequest struct {
	Name string `json:"name"`
}

// ProjectResource mirrors internal/api's projectResource
// (internal/api/projects.go). OrgID is empty when the project isn't
// filed under any organization.
type ProjectResource struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	OrgID     string `json:"org_id,omitempty"`
}

// SetProjectOrganizationRequest mirrors internal/api's
// setProjectOrganizationRequest. An empty OrgID clears the assignment.
type SetProjectOrganizationRequest struct {
	OrgID string `json:"org_id"`
}

// EnvironmentResource mirrors internal/api's environmentResource
// (internal/api/environments.go).
type EnvironmentResource struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
}

// CreateEnvironmentRequest mirrors internal/api's
// createEnvironmentRequest.
type CreateEnvironmentRequest struct {
	Name string `json:"name"`
}

// SetAppEnvironmentRequest mirrors internal/api's
// setAppEnvironmentRequest. An empty EnvironmentID clears the
// assignment.
type SetAppEnvironmentRequest struct {
	EnvironmentID string `json:"environment_id"`
}

// PreviewEnvironmentResource mirrors internal/api's
// previewEnvironmentResource (internal/api/preview_environments_handlers.go).
type PreviewEnvironmentResource struct {
	PRNumber     int    `json:"pr_number"`
	PreviewAppID string `json:"preview_app_id"`
	Branch       string `json:"branch"`
	HeadSHA      string `json:"head_sha"`
	Domain       string `json:"domain,omitempty"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// SetPreviewEnabledRequest mirrors internal/api's
// setPreviewEnabledRequest.
type SetPreviewEnabledRequest struct {
	Enabled bool `json:"enabled"`
}

// SetAppDatabaseRequest mirrors internal/api's setAppDatabaseRequest
// (apps_database.go). EnvVar and Field are both optional: the server
// defaults them ("DATABASE_URL"/"url") when left blank.
type SetAppDatabaseRequest struct {
	DatabaseName string `json:"database_name"`
	EnvVar       string `json:"env_var,omitempty"`
	Field        string `json:"field,omitempty"`
}

// AppDatabaseResource mirrors internal/api's appDatabaseResource:
// PUT /api/v1/apps/{name}/database's response body.
type AppDatabaseResource struct {
	AppName      string `json:"app_name,omitempty"`
	DatabaseName string `json:"database_name"`
	EnvVar       string `json:"env_var"`
	Field        string `json:"field"`
}

// NodeResource mirrors internal/api's nodeResource (internal/api/nodes.go).
type NodeResource struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Address         string     `json:"address,omitempty"`
	Status          string     `json:"status"`
	CertFingerprint string     `json:"cert_fingerprint,omitempty"`
	JoinedAt        *time.Time `json:"joined_at,omitempty"`
	LastSeenAt      *time.Time `json:"last_seen_at,omitempty"`
	// Schedulable is the cordon state: false means the node refuses new
	// placements while whatever's already running there keeps running.
	Schedulable           bool      `json:"schedulable"`
	AcceptsAppWorkloads   bool      `json:"accepts_app_workloads"`
	AcceptsBuildWorkloads bool      `json:"accepts_build_workloads"`
	CreatedAt             time.Time `json:"created_at"`
}

// NodePatchStatusResource mirrors internal/api's nodePatchStatusResponse
// (internal/api/node_patch_status.go). Checked distinguishes "never
// checked" (Total/Security both zero-valued and meaningless) from
// checked-and-genuinely-up-to-date (Checked true, Total 0): callers must
// branch on Checked first.
type NodePatchStatusResource struct {
	Checked   bool       `json:"checked"`
	Total     int        `json:"total"`
	Security  int        `json:"security"`
	CheckedAt *time.Time `json:"checked_at,omitempty"`
}

// SetNodeWorkloadsRequest mirrors internal/api's setNodeWorkloadsRequest:
// a full replace of both fields, not a partial patch.
type SetNodeWorkloadsRequest struct {
	AcceptsAppWorkloads   bool `json:"accepts_app_workloads"`
	AcceptsBuildWorkloads bool `json:"accepts_build_workloads"`
}

// CreateNodeJoinTokenResponse mirrors internal/api's
// createNodeJoinTokenResponse: a join token's one and only plaintext
// appearance, the same "shown once, never recoverable again" shape a
// created API token uses.
type CreateNodeJoinTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// DrainNodeResponse mirrors internal/api's drainNodeResponse. A partial
// failure is not itself a Go error from DrainNode: it comes back as a
// successful response with Errors populated, naming exactly which
// resource didn't move so the caller can retry just that one.
type DrainNodeResponse struct {
	TargetNodeID   string   `json:"target_node_id"`
	MovedServices  []string `json:"moved_services"`
	MovedDatabases []string `json:"moved_databases"`
	Errors         []string `json:"errors,omitempty"`
}

// apiErrorBody is the JSON shape every non-2xx response from the
// control plane returns (internal/api/respond.go's own apiError).
type apiErrorBody struct {
	Error string `json:"error"`
}

// APIError is returned by Client methods for a non-2xx HTTP response: a
// real reply from the server, as opposed to a network-level failure
// (connection refused, DNS, timeout), which Client methods return as a
// plain wrapped error instead. Callers distinguish the two with
// errors.As, which is what picks the CLI's exit code and lets the MCP
// server surface a specific tool error (e.g. a 403's "token lacks the
// required ability" message) rather than a generic failure.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("server returned %d: %s", e.StatusCode, e.Message)
}
