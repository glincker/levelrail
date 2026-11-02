package api

import (
	"sort"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/store"
)

// appResource is the wire shape for an app: store.DesiredService plus its
// name, marshaled directly rather than through a parallel domain type.
type appResource struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	// ImageDigest is the content Image is pinned to, or a build's local
	// image ID; empty for a legacy unpinned tag.
	ImageDigest string `json:"image_digest,omitempty"`
	Port        int    `json:"port"`
	// HostPort pins the host-side port Docker binds Port to
	// (store.DesiredService.HostPort, migrations/0056). nil means "let
	// Docker assign one", the ordinary case; a real conflict at deploy
	// time (this host port already bound) surfaces through the existing
	// reconcile-condition/deploy-log mechanism, since Docker itself is
	// the only thing that can authoritatively know at container-create
	// time. Settable on create and update, like Port itself, not
	// response-only.
	HostPort *int `json:"host_port,omitempty"`
	// PublishedPorts are ports bound straight on the host beside Port (UDP
	// today). On update, omitted keeps the stored list and [] clears it.
	PublishedPorts []store.PublishedPort `json:"published_ports,omitempty"`
	// BindAddress picks which network interface Port (and HostPort, if
	// pinned) binds to on the host (store.DesiredService.BindAddress,
	// migrations/0098): "private" (loopback only, the default), "public"
	// (every interface), or a literal IP, see internal/bindaddr.Resolve.
	// No omitempty: a real store.DesiredService read back always carries
	// the resolved value, never "", the same always-resolved treatment
	// Strategy/Replicas get above. An empty value on the way in is not an
	// error, it falls through to store.DefaultBindAddress.
	BindAddress string            `json:"bind_address"`
	Domains     []string          `json:"domains,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	// EnvMeta is response-only per-variable sensitivity. A sensitive value
	// reads back as envredact.Placeholder, and sending that back keeps it.
	EnvMeta map[string]envredact.Meta `json:"env_meta,omitempty"`
	// SecretsWriteOnly is response-only: the owner's per-app reveal lock.
	SecretsWriteOnly bool `json:"secrets_write_only,omitempty"`
	// SecretEnv names which env vars are backed by encrypted secret
	// storage (store.DesiredService.SecretEnv): values live in
	// internal/secrets, never here. Response-mirrors the store; settable
	// at create time, see handleCreateApp and Secrets below.
	SecretEnv []string `json:"secret_env,omitempty"`
	// Secrets carries plaintext values for SecretEnv-named vars,
	// encrypted immediately at create time via the same envelope
	// encryption PUT /api/v1/apps/{name}/secrets/{key} uses. Write-only:
	// toAppResource never sets this, matching that endpoint's own
	// "never echo a value back" rule.
	Secrets map[string]string `json:"secrets,omitempty"`
	// VaultEnv names env vars whose values resolve live from an external
	// HashiCorp Vault instance (store.DesiredService.VaultEnv), the
	// alternative to SecretEnv above: never a value stored by this
	// platform, only a { path, key } reference resolved fresh by the
	// application controller immediately before container creation.
	// Settable at create time, the same "declare it, no value round-
	// trips" shape SecretEnv/Secrets establish for local secrets.
	VaultEnv  map[string]appVaultEnvRef `json:"vault_env,omitempty"`
	Resources *store.ServiceResources   `json:"resources,omitempty"`
	Health    *store.ServiceHealth      `json:"health,omitempty"`
	// Hooks are this service's pre/post-deploy commands
	// (store.DesiredService.Hooks), settable on create and update like
	// Resources/Health above. See internal/reconcile/application.Controller's
	// own doc comment for when each one runs and what a failure does; see
	// GET /api/v1/apps/{name}/hook-runs (apps_hooks.go) for the most
	// recent outcome of each.
	Hooks *store.ServiceHooks `json:"hooks,omitempty"`
	// Egress is display-only here and excluded from toDesiredService so a plain PUT
	// never clears a configured policy; set it via app.yaml or the egress-policy route.
	Egress *store.ServiceEgressPolicy `json:"egress,omitempty"`
	// Strategy and Replicas have no omitempty: reads always carry the resolved
	// value, and an empty value on input falls to the store defaults.
	Strategy string `json:"strategy"`
	Replicas int    `json:"replicas"`
	// Labels are arbitrary operator-supplied Docker labels applied to
	// the app's container at create time (internal/spec.Service.Labels'
	// wire counterpart), exposed on the general create/update endpoints
	// rather than a dedicated one: this is fairly static, low-frequency-
	// change config, not something needing its own PUT the way a
	// restart nonce or backup schedule does. validateAppResource runs
	// internal/spec.ValidateLabels against it, the same reserved-prefix
	// and sanity-limit rules app.yaml-sourced labels get.
	Labels map[string]string `json:"labels,omitempty"`
	// NodeID is placement (empty means the local node). Response-only on PUT; create
	// may set it, and PUT /api/v1/apps/{name}/node is how placement changes.
	NodeID string `json:"node_id,omitempty"`
	// NodeName and NodeIsLocal are response-only display fields resolved
	// from NodeID: an empty NodeID means this server. NodeName is empty
	// when NodeID points at a node that no longer exists.
	NodeName    string `json:"node_name,omitempty"`
	NodeIsLocal bool   `json:"node_is_local,omitempty"`
	// NodeOffline is response-only: true while the hosting node is offline, so
	// clients need not join against the nodes list.
	NodeOffline bool `json:"node_offline,omitempty"`
	// AutoPlaced is response-only, set only by handleCreateApp: true when
	// node_id was omitted from the create request and simple spread
	// scheduling (autoPlaceNode, scheduling.go) picked a non-local node
	// for it. Every other handler returning an appResource leaves this
	// false.
	AutoPlaced bool `json:"auto_placed,omitempty"`
	// AutoDomain and DNS are response-only, set by handleCreateApp when the
	// apps base domain attached <app>.<base> and created its record.
	AutoDomain string           `json:"auto_domain,omitempty"`
	DNS        *domainDNSResult `json:"dns,omitempty"`
	// ProjectID is response-only on PUT so an ordinary edit never moves an app
	// between projects; create accepts it, PUT /api/v1/apps/{name}/project changes it.
	ProjectID string `json:"project_id,omitempty"`
	// EnvironmentID is which environment (environments.go) this app is
	// tagged with, empty meaning none. Response-only, same boundary as
	// ProjectID: set via PUT /api/v1/apps/{name}/environment instead.
	EnvironmentID string `json:"environment_id,omitempty"`
	// StorageTargetID is which connected backup target (backup_targets.go)
	// this app's object-storage credentials resolve from, empty meaning
	// no storage attached. Response-only, the same "shown but not
	// settable through this endpoint" boundary NodeID/ProjectID already
	// establish above, for identical reasoning: an ordinary edit must
	// never silently attach or detach live bucket credentials. Set it via
	// PUT/DELETE /api/v1/apps/{name}/storage (apps_storage.go's
	// handleSetAppStorage/handleClearAppStorage) instead.
	StorageTargetID string `json:"storage_target_id,omitempty"`
	// DatabaseEnv names every env var whose value resolves from a managed
	// database's own connection details (store.DesiredService.DatabaseEnv,
	// internal/spec's { from: "<database>.<field>" } syntax), so the UI
	// can show which databases an app.yaml-deployed service is wired to
	// without hand-parsing app.yaml itself. Response-only: this comes from
	// the deploy pipeline (internal/deploy), never from this endpoint.
	DatabaseEnv map[string]appDatabaseEnvRef `json:"database_env,omitempty"`
	// DatabaseAttachment is the UI/CLI-facing attachment
	// (store.DesiredService.DatabaseAttachment), distinct from DatabaseEnv
	// above: see apps_database.go's own doc comment. Response-only, the
	// same "shown but not settable through this endpoint" boundary
	// StorageTargetID already establishes: set it via PUT/DELETE
	// /api/v1/apps/{name}/database instead.
	DatabaseAttachment *appDatabaseResource `json:"database_attachment,omitempty"`
	// Suspended is whether the app is stopped (POST .../stop), the
	// reconciler's own containers-only teardown, distinct from delete.
	// Response-only: set it via POST/DELETE-shaped
	// /api/v1/apps/{name}/stop and /start (handleStopApp/handleStartApp)
	// instead, the same boundary NodeID/ProjectID/StorageTargetID
	// already establish above.
	Suspended bool `json:"suspended"`
	// AppID is which store.App (migrations/0039_apps.sql) this service
	// belongs to; empty when none. toDesiredService never reads it back
	// in: it's assigned server-side, via ensureAppLinked (apps_multi.go),
	// from handleCreateApp here and from POST /api/v1/apps/{name}/deploy-spec
	// for a fanned-out service. See GET /api/v1/apps/{name}/group
	// (apps_group.go) for the sibling-services read this enables.
	AppID string `json:"app_id,omitempty"`
	// IsTrial marks a service deployed via the one-click template
	// "Deploy now" path (store.DesiredService.IsTrial,
	// migrations/0264_service_is_trial.sql). Response-only: set only by
	// handleDeployServiceTemplateNow at create time, never through this
	// endpoint.
	IsTrial bool `json:"is_trial,omitempty"`
	// TemplateID is the catalog template the app was deployed from; response-only, set at deploy time.
	TemplateID string `json:"template_id,omitempty"`
	// LogDrain is response-only, the same boundary NodeID/ProjectID/
	// StorageTargetID already establish above: set it via PUT/DELETE
	// /api/v1/apps/{name}/log-drain (apps_log_drain.go) instead.
	LogDrain *store.LogDrain `json:"log_drain,omitempty"`
	// PreviewEnvOverrides names, for a subset of this app's own env vars,
	// a preview-specific value applied on top of this app's own env only
	// when a preview environment is next created from it
	// (store.DesiredService.PreviewEnvOverrides). Response-only, the same
	// boundary NodeID/ProjectID/StorageTargetID/LogDrain already establish
	// above: set it via PUT/DELETE /api/v1/apps/{name}/preview-env/{key}
	// (apps_preview_env.go) instead.
	PreviewEnvOverrides map[string]string `json:"preview_env_overrides,omitempty"`
	// ResourcesAppliedLive is response-only, set by handleUpdateApp:
	// whether the new Resources value was already pushed onto a
	// currently running container via the Engine API's live
	// ContainerUpdate (applyResourcesLiveToReplicas), rather than
	// waiting for the next deploy or restart to take effect. False
	// still means the value is saved and will apply next time the
	// container is recreated; it never means the save itself failed.
	ResourcesAppliedLive bool `json:"resources_applied_live,omitempty"`
	// EnvDirty is response-only (store.DesiredService.EnvDirty's own doc
	// comment): true means an env save is not live on the running
	// container yet. No omitempty: like Suspended, always a definite
	// state.
	EnvDirty bool `json:"env_dirty"`
	// Volumes are this app's named Docker volumes (store.DesiredService.
	// Volumes), response-only like NodeID/ProjectID above: declared
	// through app.yaml's volumes: field, not settable through this
	// endpoint. See app_volume_backups.go/app_volume_restore.go for the
	// backup/restore/schedule endpoints each one supports.
	Volumes []appVolumeResource `json:"volumes,omitempty"`
	// BindMounts are this app's bind-mounted host directories
	// (store.DesiredService.BindMounts), response-only for the same
	// reason Volumes above is: see appBindMountResource's own doc
	// comment for where these actually get set.
	BindMounts []appBindMountResource `json:"bind_mounts,omitempty"`
	// Command overrides the image's own default CMD
	// (store.DesiredService.Command): settable here the same as Env or
	// Image, in addition to app.yaml's command: field or a compose
	// import (internal/deploy/translate.go, internal/compose/translate.go).
	Command []string `json:"command,omitempty"`
	// PullPolicy is store.PullPolicyAlways to force a fresh image pull on
	// every deploy, or empty for the default pull-if-absent behavior.
	// Response-only, same reason Command above is: set through a compose
	// import's pull_policy:, never through this endpoint.
	PullPolicy string `json:"pull_policy,omitempty"`
	// DependsOn names sibling service keys (store.DesiredService.
	// DependsOn) this service waits on before the reconciler starts its
	// own containers, set through app.yaml's dependsOn: or a compose
	// file's depends_on:, response-only here, same reason Command is.
	DependsOn []string `json:"depends_on,omitempty"`
	// Tags names every store.Tag (internal/store/tags.go) attached to
	// this app, response-only like NodeID/ProjectID above: toAppResource
	// never sets this (tags live in their own join table, not
	// store.DesiredService), only handleGetApp and handleListApps
	// populate it, via a dedicated attach/detach endpoint
	// (tags.go's handleAttachAppTag/handleDetachAppTag).
	Tags []string `json:"tags,omitempty"`
}

// appVaultEnvRef is store.VaultEnvRef's wire shape, used both inside
// appResource.VaultEnv (a map) and nowhere else, the same "no name of
// its own" shape appDatabaseEnvRef already establishes.
type appVaultEnvRef struct {
	Path string `json:"path"`
	Key  string `json:"key"`
}

// toAppResource builds the wire resource with sensitive env values masked
// by automatic classification; rt.appOut also applies per-app overrides.
func toAppResource(svc store.DesiredService) appResource {
	res := toAppResourceRaw(svc)
	redactAppEnvFields(&res, svc, nil)
	return res
}

func toAppResourceRaw(svc store.DesiredService) appResource {
	var databaseEnv map[string]appDatabaseEnvRef
	if len(svc.DatabaseEnv) > 0 {
		databaseEnv = make(map[string]appDatabaseEnvRef, len(svc.DatabaseEnv))
		for k, v := range svc.DatabaseEnv {
			databaseEnv[k] = appDatabaseEnvRef{Database: v.Database, Field: v.Field}
		}
	}
	var vaultEnv map[string]appVaultEnvRef
	if len(svc.VaultEnv) > 0 {
		vaultEnv = make(map[string]appVaultEnvRef, len(svc.VaultEnv))
		for k, v := range svc.VaultEnv {
			vaultEnv[k] = appVaultEnvRef{Path: v.Path, Key: v.Key}
		}
	}
	var attachment *appDatabaseResource
	if svc.DatabaseAttachment != nil {
		attachment = &appDatabaseResource{
			AppName:      svc.Name,
			DatabaseName: svc.DatabaseAttachment.DatabaseName,
			EnvVar:       svc.DatabaseAttachment.EnvVar,
			Field:        svc.DatabaseAttachment.Field,
		}
	}

	return appResource{
		ImageDigest:         appImageDigest(svc),
		Name:                svc.Name,
		Image:               svc.Image,
		Port:                svc.Port,
		HostPort:            svc.HostPort,
		PublishedPorts:      svc.PublishedPorts,
		BindAddress:         svc.BindAddress,
		Domains:             svc.Domains,
		Env:                 svc.Env,
		SecretEnv:           store.SecretEnvNames(svc.SecretEnv),
		VaultEnv:            vaultEnv,
		Resources:           svc.Resources,
		Health:              svc.Health,
		Hooks:               svc.Hooks,
		Egress:              svc.Egress,
		Strategy:            svc.Strategy,
		Replicas:            svc.Replicas,
		Labels:              svc.Labels,
		NodeID:              svc.NodeID,
		ProjectID:           svc.ProjectID,
		EnvironmentID:       svc.EnvironmentID,
		StorageTargetID:     svc.StorageTargetID,
		DatabaseEnv:         databaseEnv,
		DatabaseAttachment:  attachment,
		Suspended:           svc.Suspended,
		AppID:               svc.AppID,
		IsTrial:             svc.IsTrial,
		TemplateID:          svc.TemplateID,
		LogDrain:            svc.LogDrain,
		PreviewEnvOverrides: svc.PreviewEnvOverrides,
		EnvDirty:            svc.EnvDirty,
		Volumes:             toAppVolumeResources(svc),
		BindMounts:          toAppBindMountResources(svc),
		Command:             svc.Command,
		PullPolicy:          svc.PullPolicy,
		DependsOn:           svc.DependsOn,
	}
}

func (a appResource) toDesiredService() store.DesiredService {
	return store.DesiredService{
		Name:           a.Name,
		Image:          a.Image,
		Port:           a.Port,
		HostPort:       a.HostPort,
		BindAddress:    a.BindAddress,
		Domains:        a.Domains,
		Env:            a.Env,
		Resources:      a.Resources,
		Health:         a.Health,
		Hooks:          a.Hooks,
		Strategy:       a.Strategy,
		Replicas:       a.Replicas,
		Labels:         a.Labels,
		Command:        a.Command,
		PublishedPorts: a.PublishedPorts,
	}
}

// unionSecretEnvNames merges declared secret env var names with the
// names of any inline value provided: a key present in values is
// secret-backed even if the caller didn't also list it in declared.
func unionSecretEnvNames(declared []string, values map[string]string) []string {
	seen := make(map[string]bool, len(declared)+len(values))
	var out []string
	for _, k := range declared {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}
