package api

import (
	"context"

	"github.com/GLINCKER/levelrail/internal/store"
)

// AppGroupLister is the store surface for GET /api/v1/apps/{name}/group and the app-linking
// write path. *store.DB satisfies this structurally.
type AppGroupLister interface {
	ListServicesByApp(ctx context.Context, appID string) ([]store.DesiredService, error)
	DeleteApp(ctx context.Context, id string) error
	SaveApp(ctx context.Context, a store.App) error
	GetAppByName(ctx context.Context, name string) (store.App, error)
}

// AppComposeStore is the store surface POST /api/v1/apps/{name}/compose
// needs (apps_compose.go): create-or-reuse the owning store.App, then
// save each of its member services.
type AppComposeStore interface {
	SaveApp(ctx context.Context, a store.App) error
	GetAppByName(ctx context.Context, name string) (store.App, error)
	SaveDesiredService(ctx context.Context, svc store.DesiredService) error
}

// CustomTemplateStore is the store surface save-as-template needs
// (service_templates_custom.go): CRUD on operator-defined templates,
// the growth-loop counterpart to internal/catalog's static built-in
// catalog. See store.CustomTemplate's own doc comment for why a
// template row never holds a real secret value.
type CustomTemplateStore interface {
	SaveCustomTemplate(ctx context.Context, t store.CustomTemplate) error
	GetCustomTemplate(ctx context.Context, id string) (store.CustomTemplate, error)
	ListCustomTemplates(ctx context.Context) ([]store.CustomTemplate, error)
	DeleteCustomTemplate(ctx context.Context, id string) error
}

// ComposeSecretStore is the surface POST /api/v1/apps/{name}/compose
// needs to resolve a compose file's SERVICE_ magic vars
// (compose.ResolveMagicVars): a canonical generate-once-per-app value
// (Resolve/SetValue keyed by the app's own name, a synthetic bucket
// distinct from any real service), plus one SetValue per real service
// that references it, since internal/secrets.Manager itself is keyed
// per real service, not per app.
type ComposeSecretStore interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
}

// DatabaseStore is the store surface the databases handlers need, the
// database-kind counterpart to AppStore.
type DatabaseStore interface {
	SaveDesiredDatabase(ctx context.Context, d store.DesiredDatabase) error
	GetDesiredDatabase(ctx context.Context, name string) (*store.DesiredDatabase, error)
	ListDesiredDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
	DeleteDesiredDatabase(ctx context.Context, name string) error
	// UpdateDatabaseNode is AppStore.UpdateServiceNode's counterpart,
	// same separation-from-ordinary-update reasoning. Originally added
	// unexposed by 3.3; handleDrainNode is the first caller with an
	// actual route to reuse it through.
	UpdateDatabaseNode(ctx context.Context, name, nodeID string) error
	// ListDesiredDatabasesByNode is the database-kind counterpart to
	// AppStore.ListDesiredServicesByNode.
	ListDesiredDatabasesByNode(ctx context.Context, nodeID string) ([]store.DesiredDatabase, error)
	// ListDesiredDatabasesByProject is the database-kind counterpart to
	// AppStore.ListDesiredServicesByProject.
	ListDesiredDatabasesByProject(ctx context.Context, projectID string) ([]store.DesiredDatabase, error)
	// UpdateDatabaseSuspended is AppStore.UpdateServiceSuspended's
	// counterpart, backing POST /api/v1/projects/{id}/stop and .../start
	// (project_stop_start.go) and POST /api/v1/databases/{name}/stop and
	// .../start (database_stop_start.go): same separation-from-ordinary-
	// update reasoning as UpdateDatabaseNode/UpdateDatabaseProject, see
	// store.DB.UpdateDatabaseSuspended's own doc comment.
	UpdateDatabaseSuspended(ctx context.Context, name string, suspended bool) error
	// UpdateDatabaseProject is AppStore.UpdateServiceProject's
	// counterpart (projects.go).
	UpdateDatabaseProject(ctx context.Context, name, projectID string) error
	// SetDatabaseBackupSchedule backs
	// PUT/DELETE /api/v1/databases/{name}/backup-schedule (backups.go):
	// wave-2 roadmap item 6, scheduled backups. Same "own endpoint, own
	// store method" separation UpdateDatabaseNode/UpdateDatabaseProject
	// already establish for their own single-purpose updates.
	SetDatabaseBackupSchedule(ctx context.Context, name, targetID, schedule string, retain, retainDays int) error
	// SetDatabasePITR backs POST/DELETE /api/v1/databases/{name}/pitr
	// (pitr.go): same "own endpoint, own store method" separation
	// SetDatabaseBackupSchedule above establishes.
	SetDatabasePITR(ctx context.Context, name string, enabled bool, enabledAt string) error
	// SetDatabasePublicAccess backs
	// PUT/DELETE /api/v1/databases/{name}/public-access
	// (database_public_access.go): the same "own endpoint, own store
	// method" separation SetDatabaseBackupSchedule already establishes,
	// applied to whether this database's container port is bound to a
	// host port. requestedBindAddress empty resolves to
	// store.DefaultBindAddress; ignored when enabled is false. Returns
	// the port actually assigned (0 when disabling).
	SetDatabasePublicAccess(ctx context.Context, name string, enabled bool, requestedPort int, requestedBindAddress string) (int, error)
}

// ProjectStore is the store surface the projects handlers need
// (projects.go). See that file's own package doc comment for why a
// project is deliberately not the start of the deferred Phase 4 teams/
// RBAC work (repo-plan section 6's own scope note): no owner, no
// member list, no per-project ability, just create/list/get/delete.
type ProjectStore interface {
	SaveProject(ctx context.Context, p store.Project) error
	GetProject(ctx context.Context, id string) (store.Project, error)
	ListProjects(ctx context.Context) ([]store.Project, error)
	DeleteProject(ctx context.Context, id string) error
	// SetProjectEnvVars/ListProjectEnvVars back GET/PUT
	// /api/v1/projects/{id}/env (project_env.go): shared env vars every
	// app filed under this project inherits as its resolveEnv base
	// layer (internal/reconcile/application), full-replace on write, the
	// same shape PUT /api/v1/apps/{name}'s own env field already has.
	SetProjectEnvVars(ctx context.Context, projectID string, vars map[string]string) error
	ListProjectEnvVars(ctx context.Context, projectID string) (map[string]string, error)
	// ListProjectEnvVarsDetailed/SetProjectSecretEnvVar/
	// DeleteProjectSecretEnvVar/ListProjectSecretEnvKeys back GET
	// /api/v1/projects/{id}/env/all and the secret-var sub-resource
	// routes (project_env.go): the secret-capable extension of the
	// plain-only pair above, reusing internal/secrets.Manager (via
	// Router.secrets) for the actual encryption rather than a second
	// path.
	ListProjectEnvVarsDetailed(ctx context.Context, projectID string) ([]store.SharedEnvVar, error)
	SetProjectSecretEnvVar(ctx context.Context, projectID, key string) error
	DeleteProjectSecretEnvVar(ctx context.Context, projectID, key string) error
	ListProjectSecretEnvKeys(ctx context.Context, projectID string) ([]string, error)
}
