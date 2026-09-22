package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Clone environment (this file): given a source environment (a
// project-scoped label, see store.Environment's own doc comment), create
// a new environment in the same project plus an equivalent copy of every
// app tagged with the source, and actually deploy those copies through
// the normal reconcile-driven path (recordInstantDeployAttempt +
// nudgeReconciler), the same real-deploy shape handleCreateApp already
// establishes. Unlike promote.go (one app's image, onto an existing
// sibling), this creates brand-new apps and a brand-new environment in
// one call.
//
// What is copied verbatim: image, port, bind address, env vars, command/
// entrypoint, pull policy, registry credential, secret/vault env
// declarations (names only, see below), resources, health checks, hooks,
// egress policy, bind mounts, labels, strategy/replicas, storage target,
// log drain, auto-rollback flag, exec-enabled flag, and scheduled tasks.
//
// What is regenerated: the app's own name (desired_services.name is
// globally unique, so a straight copy would collide with the source) and
// any Docker-volume name derived from it.
//
// What is dropped, deliberately, not carried over "for free": domains (a
// domain can only ever belong to one service, so the clone starts with
// none, or whatever the caller explicitly assigns), any host port pin
// (same collision reasoning as domains), database attachments and
// app.yaml database env references (this operation clones services, not
// managed databases), node placement, and any git build source (the
// clone deploys the source app's *current* image, the same thing
// promote.go itself moves).
//
// Secret values are the one deliberately opt-in exception to "copy
// everything real": every secret-backed env var, per-app or
// environment-wide, is declared on the clone (same key, same
// required flag) but left with no value, exactly like a brand-new
// required secret, unless the caller sets copy_secret_values: true on
// the POST request. See internal/api/promote.go's own
// promotePreviewUnsnapshottedFields doc comment for the precedent this
// follows: being careful about what crosses a tier boundary unprompted.
const environmentClonePreviewNote = "Domains are never copied: a domain can only ever belong to one service, so every cloned app starts with none unless you assign new ones (either here or afterward). Host port pins are cleared for the same reason. Database attachments, app.yaml database env references, git build source, and node placement are not cloned either. Secret values (per-app and environment-wide alike) are declared on the clone with no value set unless you opt in with copy_secret_values: true on the actual clone request; every declared secret otherwise behaves like a brand-new required secret until you set it."

// environmentCloneUnclonedFields lists what a clone never carries over
// regardless of flags, the read-only-preview counterpart to
// promote.go's own promotePreviewUnsnapshottedFields.
var environmentCloneUnclonedFields = []string{
	"domains", "host_port", "database_attachment", "database_env",
	"git_source", "node_id",
}

// environmentCloneAppPreview is one source app's own view inside GET
// .../clone/preview: enough for an operator to judge what a clone would
// create before committing to it.
type environmentCloneAppPreview struct {
	SourceApp             string   `json:"source_app"`
	SuggestedNewName      string   `json:"suggested_new_name"`
	Image                 string   `json:"image"`
	EnvVarCount           int      `json:"env_var_count"`
	SecretEnvKeys         []string `json:"secret_env_keys"`
	CurrentDomains        []string `json:"current_domains"`
	VolumeCount           int      `json:"volume_count"`
	BindMountCount        int      `json:"bind_mount_count"`
	HasHealthCheck        bool     `json:"has_health_check"`
	HasDatabaseAttachment bool     `json:"has_database_attachment"`
	HasHostPortPin        bool     `json:"has_host_port_pin"`
	ScheduledTaskCount    int      `json:"scheduled_task_count"`
}

// environmentClonePreviewResource is GET .../clone/preview's response
// shape.
type environmentClonePreviewResource struct {
	SourceEnvironment        environmentResource          `json:"source_environment"`
	NewEnvironmentName       string                       `json:"new_environment_name"`
	Apps                     []environmentCloneAppPreview `json:"apps"`
	EnvironmentEnvVarKeys    []string                     `json:"environment_env_var_keys"`
	EnvironmentSecretEnvKeys []string                     `json:"environment_secret_env_keys"`
	UnclonedFields           []string                     `json:"uncloned_fields"`
	Note                     string                       `json:"note"`
}

// handleEnvironmentClonePreview handles
// GET /api/v1/environments/{id}/clone/preview?new_environment_name={name}.
// Read-only: it never mints IDs or reserves the suggested app names,
// only computes what a same-request POST .../clone would most likely
// produce, the same "preview never commits" contract
// handlePromotePreview already keeps for its own resource.
func (rt *Router) handleEnvironmentClonePreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	env, err := rt.environments.GetEnvironment(r.Context(), id)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, "environment not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: preview environment clone: load environment failed", err, slog.String("id", id))
		return
	}

	newName := strings.TrimSpace(r.URL.Query().Get("new_environment_name"))

	source, err := rt.appsInEnvironment(r.Context(), env)
	if err != nil {
		rt.internalError(w, "api: preview environment clone: list apps failed", err, slog.String("id", id))
		return
	}

	apps := make([]environmentCloneAppPreview, 0, len(source))
	for _, svc := range source {
		tasks, err := rt.scheduledTasks.ListScheduledTasksForService(r.Context(), svc.Name)
		if err != nil {
			rt.internalError(w, "api: preview environment clone: list scheduled tasks failed", err, slog.String("app", svc.Name))
			return
		}
		apps = append(apps, environmentCloneAppPreview{
			SourceApp:             svc.Name,
			SuggestedNewName:      suggestClonedAppName(svc.Name, newName),
			Image:                 svc.Image,
			EnvVarCount:           len(svc.Env),
			SecretEnvKeys:         store.SecretEnvNames(svc.SecretEnv),
			CurrentDomains:        svc.Domains,
			VolumeCount:           len(svc.Volumes),
			BindMountCount:        len(svc.BindMounts),
			HasHealthCheck:        svc.Health != nil,
			HasDatabaseAttachment: svc.DatabaseAttachment != nil,
			HasHostPortPin:        svc.HostPort != nil,
			ScheduledTaskCount:    len(tasks),
		})
	}

	envVars, err := rt.environments.ListEnvironmentEnvVars(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: preview environment clone: list shared env vars failed", err, slog.String("id", id))
		return
	}
	envVarKeys := make([]string, 0, len(envVars))
	for k := range envVars {
		envVarKeys = append(envVarKeys, k)
	}
	sort.Strings(envVarKeys)

	secretKeys, err := rt.environments.ListEnvironmentSecretEnvKeys(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: preview environment clone: list shared secret env keys failed", err, slog.String("id", id))
		return
	}

	writeJSON(w, http.StatusOK, environmentClonePreviewResource{
		SourceEnvironment:        toEnvironmentResource(env),
		NewEnvironmentName:       newName,
		Apps:                     apps,
		EnvironmentEnvVarKeys:    envVarKeys,
		EnvironmentSecretEnvKeys: secretKeys,
		UnclonedFields:           environmentCloneUnclonedFields,
		Note:                     environmentClonePreviewNote,
	})
}

// environmentCloneAppInput overrides one source app's own new name and/or
// domains inside a clone request; omitted entirely for a source app
// means "use the auto-suggested name, assign no domains".
type environmentCloneAppInput struct {
	SourceApp string   `json:"source_app"`
	NewName   string   `json:"new_name,omitempty"`
	Domains   []string `json:"domains,omitempty"`
}

// environmentCloneRequest is POST .../clone's body.
type environmentCloneRequest struct {
	NewEnvironmentName string `json:"new_environment_name"`
	// CopySecretValues opts into carrying real secret plaintext across
	// the tier boundary (see this file's own package doc comment above);
	// false (the default a bare JSON body without the field already
	// gives) declares every secret on the clone with no value, the safe
	// default.
	CopySecretValues bool                       `json:"copy_secret_values,omitempty"`
	Apps             []environmentCloneAppInput `json:"apps,omitempty"`
}

// environmentCloneAppResult is one cloned app's own entry inside POST
// .../clone's response.
type environmentCloneAppResult struct {
	SourceApp string `json:"source_app"`
	NewApp    string `json:"new_app"`
	Image     string `json:"image"`
}

// environmentCloneResultResource is POST .../clone's response shape.
type environmentCloneResultResource struct {
	Environment        environmentResource         `json:"environment"`
	Apps               []environmentCloneAppResult `json:"apps"`
	CopiedSecretValues bool                        `json:"copied_secret_values"`
	Note               string                      `json:"note"`
}

// handleEnvironmentClone handles POST /api/v1/environments/{id}/clone:
// creates a new environment in the same project as id, then an
// equivalent copy of every app tagged with id, each one actually
// deployed (recordInstantDeployAttempt + nudgeReconciler, the same real
// path handleCreateApp uses), not just written to the database. Either
// every app clones successfully or the request fails with the new
// environment left in place but partially populated; a caller that hits
// a mid-request failure can inspect what exists via GET
// .../projects/{id}/environments and either finish manually or delete
// the partial environment and retry, the same "no automatic rollback of
// already-committed writes" shape most of this codebase's own multi-step
// handlers already accept (see e.g. handleDeploySpec's own per-service
// loop).
func (rt *Router) handleEnvironmentClone(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	env, err := rt.environments.GetEnvironment(r.Context(), id)
	if errors.Is(err, store.ErrEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, "environment not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: clone environment: load environment failed", err, slog.String("id", id))
		return
	}

	var req environmentCloneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.NewEnvironmentName = strings.TrimSpace(req.NewEnvironmentName)
	if req.NewEnvironmentName == "" {
		writeError(w, http.StatusBadRequest, "new_environment_name is required")
		return
	}
	if req.CopySecretValues && rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set); omit copy_secret_values or set a master key first")
		return
	}

	source, err := rt.appsInEnvironment(r.Context(), env)
	if err != nil {
		rt.internalError(w, "api: clone environment: list apps failed", err, slog.String("id", id))
		return
	}
	if len(source) == 0 {
		writeError(w, http.StatusBadRequest, "no apps tagged with environment "+id+" found in its project; nothing to clone")
		return
	}

	overrides := make(map[string]environmentCloneAppInput, len(req.Apps))
	for _, a := range req.Apps {
		overrides[a.SourceApp] = a
	}

	newNames, conflicts, err := rt.resolveClonedAppNames(r.Context(), source, req.NewEnvironmentName, overrides)
	if err != nil {
		rt.internalError(w, "api: clone environment: resolve app names failed", err, slog.String("id", id))
		return
	}
	if len(conflicts) > 0 {
		writeError(w, http.StatusConflict, "app name conflicts: "+strings.Join(conflicts, "; ")+"; specify apps[].new_name to resolve")
		return
	}

	newEnvID, err := randomEnvironmentID()
	if err != nil {
		rt.internalError(w, "api: clone environment: generate id failed", err)
		return
	}
	newEnv := store.Environment{ID: newEnvID, ProjectID: env.ProjectID, Name: req.NewEnvironmentName, Protected: false, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.environments.SaveEnvironment(r.Context(), newEnv); err != nil {
		rt.internalError(w, "api: clone environment: save new environment failed", err)
		return
	}

	if err := rt.cloneEnvironmentSharedEnv(r.Context(), id, newEnvID, req.CopySecretValues); err != nil {
		rt.internalError(w, "api: clone environment: copy shared env failed", err, slog.String("id", id))
		return
	}

	results := make([]environmentCloneAppResult, 0, len(source))
	for _, svc := range source {
		newName := newNames[svc.Name]
		final, err := rt.cloneOneApp(r.Context(), svc, newName, newEnvID, overrides[svc.Name].Domains, req.CopySecretValues)
		if err != nil {
			var domainTaken *store.ErrDomainTaken
			if errors.As(err, &domainTaken) {
				writeError(w, http.StatusConflict, domainTaken.Error())
				return
			}
			rt.internalError(w, "api: clone environment: clone app failed", err, slog.String("source", svc.Name), slog.String("new_name", newName))
			return
		}
		results = append(results, environmentCloneAppResult{SourceApp: svc.Name, NewApp: newName, Image: final.Image})
	}

	rt.nudgeReconciler()
	writeJSON(w, http.StatusCreated, environmentCloneResultResource{
		Environment:        toEnvironmentResource(newEnv),
		Apps:               results,
		CopiedSecretValues: req.CopySecretValues,
		Note:               environmentClonePreviewNote,
	})
}

// appsInEnvironment lists every app tagged with env, defensively
// filtered to env's own project the same way promote.go's own
// findPromotionCandidates already is: SetServiceEnvironment never
// validates that an app's project matches the environment it's being
// tagged with, so this is the one place that mismatch could otherwise
// leak into a clone.
func (rt *Router) appsInEnvironment(ctx context.Context, env store.Environment) ([]store.DesiredService, error) {
	all, err := rt.apps.ListDesiredServicesByEnvironment(ctx, env.ID)
	if err != nil {
		return nil, err
	}
	out := make([]store.DesiredService, 0, len(all))
	for _, s := range all {
		if s.ProjectID == env.ProjectID {
			out = append(out, s)
		}
	}
	return out, nil
}

// resolveClonedAppNames computes every source app's new name up front,
// so every collision is reported together in one 409 rather than the
// caller discovering them one failed app at a time: the same
// "every problem in one response" reasoning resolvePromotion's own
// multi-candidate 400 already follows.
func (rt *Router) resolveClonedAppNames(ctx context.Context, source []store.DesiredService, newEnvironmentName string, overrides map[string]environmentCloneAppInput) (map[string]string, []string, error) {
	newNames := make(map[string]string, len(source))
	seen := make(map[string]string, len(source))
	var conflicts []string

	for _, svc := range source {
		newName := suggestClonedAppName(svc.Name, newEnvironmentName)
		if o, ok := overrides[svc.Name]; ok && o.NewName != "" {
			newName = o.NewName
		}

		if owner, dup := seen[newName]; dup {
			conflicts = append(conflicts, fmt.Sprintf("%q and %q would both become %q", owner, svc.Name, newName))
			continue
		}
		seen[newName] = svc.Name

		_, err := rt.apps.GetDesiredService(ctx, newName)
		if err == nil {
			conflicts = append(conflicts, fmt.Sprintf("an app named %q already exists (source %q)", newName, svc.Name))
			continue
		}
		if !errors.Is(err, store.ErrServiceNotFound) {
			return nil, nil, err
		}
		newNames[svc.Name] = newName
	}
	return newNames, conflicts, nil
}

// cloneEnvironmentSharedEnv copies sourceEnvID's own shared env vars
// (environment_env.go) onto newEnvID: plain values always, secret-marked
// keys always declared but their values only copied when
// copySecretValues is set (this file's own package doc comment explains
// why).
func (rt *Router) cloneEnvironmentSharedEnv(ctx context.Context, sourceEnvID, newEnvID string, copySecretValues bool) error {
	vars, err := rt.environments.ListEnvironmentEnvVars(ctx, sourceEnvID)
	if err != nil {
		return fmt.Errorf("list source shared env vars: %w", err)
	}
	if len(vars) > 0 {
		if err := rt.environments.SetEnvironmentEnvVars(ctx, newEnvID, vars); err != nil {
			return fmt.Errorf("set shared env vars: %w", err)
		}
	}

	keys, err := rt.environments.ListEnvironmentSecretEnvKeys(ctx, sourceEnvID)
	if err != nil {
		return fmt.Errorf("list source shared secret env keys: %w", err)
	}
	for _, key := range keys {
		if err := rt.environments.SetEnvironmentSecretEnvVar(ctx, newEnvID, key); err != nil {
			return fmt.Errorf("declare shared secret env var %q: %w", key, err)
		}
		if !copySecretValues || rt.secrets == nil {
			continue
		}
		value, err := rt.secrets.Resolve(ctx, store.EnvironmentEnvSecretsKey(sourceEnvID), key)
		if errors.Is(err, secrets.ErrValueNotFound) {
			continue
		}
		if err != nil {
			return fmt.Errorf("resolve shared secret env var %q: %w", key, err)
		}
		if err := rt.secrets.SetValueGuarded(ctx, store.EnvironmentEnvSecretsKey(newEnvID), key, value, false); err != nil {
			return fmt.Errorf("copy shared secret env var %q value: %w", key, err)
		}
	}
	return nil
}

// cloneOneApp creates newName as a full copy of source's own desired
// state (see cloneDesiredService), tags it with newEnvID, links it to
// its own store.App row (ensureAppLinked, the same call handleCreateApp
// makes for an ordinary single-service app), optionally copies its
// secret values, copies its scheduled tasks, then records and triggers a
// real deploy the same way handleCreateApp's own trailing
// recordPlainDeployAttempt/nudgeReconciler does (nudgeReconciler itself
// is called once by the caller after every app in the batch is done, not
// once per app).
func (rt *Router) cloneOneApp(ctx context.Context, source store.DesiredService, newName, newEnvID string, domains []string, copySecretValues bool) (store.DesiredService, error) {
	desired := cloneDesiredService(source, newName, domains)

	if err := rt.apps.SaveDesiredService(ctx, desired); err != nil {
		return store.DesiredService{}, fmt.Errorf("save desired service: %w", err)
	}
	if err := rt.apps.UpdateServiceProject(ctx, newName, source.ProjectID); err != nil {
		return store.DesiredService{}, fmt.Errorf("assign project: %w", err)
	}
	if err := rt.environments.SetServiceEnvironment(ctx, newName, newEnvID); err != nil {
		return store.DesiredService{}, fmt.Errorf("tag environment: %w", err)
	}
	if source.StorageTargetID != "" {
		if err := rt.apps.UpdateServiceStorageTarget(ctx, newName, source.StorageTargetID); err != nil {
			return store.DesiredService{}, fmt.Errorf("assign storage target: %w", err)
		}
	}
	if source.LogDrain != nil {
		drain := *source.LogDrain
		if err := rt.apps.UpdateServiceLogDrain(ctx, newName, &drain); err != nil {
			return store.DesiredService{}, fmt.Errorf("assign log drain: %w", err)
		}
	}
	if source.AutoRollbackOnCrashloop {
		if err := rt.apps.SetServiceAutoRollbackOnCrashloop(ctx, newName, true); err != nil {
			return store.DesiredService{}, fmt.Errorf("set auto rollback on crashloop: %w", err)
		}
	}
	if err := rt.apps.SetServiceExecEnabled(ctx, newName, source.ExecEnabled); err != nil {
		return store.DesiredService{}, fmt.Errorf("set exec enabled: %w", err)
	}
	if _, err := rt.ensureAppLinked(ctx, newName, newName); err != nil {
		return store.DesiredService{}, fmt.Errorf("link app: %w", err)
	}

	if copySecretValues && rt.secrets != nil {
		for _, ref := range source.SecretEnv {
			value, err := rt.secrets.Resolve(ctx, source.Name, ref.Name)
			if errors.Is(err, secrets.ErrValueNotFound) {
				continue
			}
			if err != nil {
				return store.DesiredService{}, fmt.Errorf("resolve secret %q: %w", ref.Name, err)
			}
			if err := rt.secrets.SetValueGuarded(ctx, newName, ref.Name, value, false); err != nil {
				return store.DesiredService{}, fmt.Errorf("copy secret %q value: %w", ref.Name, err)
			}
		}
	}

	if err := rt.copyScheduledTasks(ctx, source.Name, newName); err != nil {
		return store.DesiredService{}, fmt.Errorf("copy scheduled tasks: %w", err)
	}

	final, err := rt.apps.GetDesiredService(ctx, newName)
	if err != nil {
		return store.DesiredService{}, fmt.Errorf("reload cloned app: %w", err)
	}
	rt.recordInstantDeployAttempt(ctx, *final, final.Image, store.DeployAttemptSourceClone)
	return *final, nil
}

// copyScheduledTasks copies every scheduled task attached to sourceApp
// onto newApp, minting a fresh store.ScheduledTask.ID for each (its own
// run history starts empty, the same "a clone is a new resource with no
// history of its own" shape a fresh deploy_attempts row already has for
// the app itself).
func (rt *Router) copyScheduledTasks(ctx context.Context, sourceApp, newApp string) error {
	tasks, err := rt.scheduledTasks.ListScheduledTasksForService(ctx, sourceApp)
	if err != nil {
		return fmt.Errorf("list source scheduled tasks: %w", err)
	}
	now := time.Now().UTC()
	for _, t := range tasks {
		id, err := randomScheduledTaskID()
		if err != nil {
			return fmt.Errorf("generate scheduled task id: %w", err)
		}
		if err := rt.scheduledTasks.SaveScheduledTask(ctx, store.ScheduledTask{
			ID:                id,
			ServiceName:       newApp,
			Command:           append([]string(nil), t.Command...),
			Schedule:          t.Schedule,
			Enabled:           t.Enabled,
			ConcurrencyPolicy: t.ConcurrencyPolicy,
			CreatedAt:         now,
			UpdatedAt:         now,
		}); err != nil {
			return fmt.Errorf("save scheduled task %q: %w", t.ID, err)
		}
	}
	return nil
}

// cloneDesiredService builds newName's own desired state from source:
// see this file's own package doc comment for exactly what is copied,
// regenerated, or dropped.
func cloneDesiredService(source store.DesiredService, newName string, domains []string) store.DesiredService {
	env := make(map[string]string, len(source.Env))
	for k, v := range source.Env {
		env[k] = v
	}
	labels := make(map[string]string, len(source.Labels))
	for k, v := range source.Labels {
		labels[k] = v
	}
	vaultEnv := make(map[string]store.VaultEnvRef, len(source.VaultEnv))
	for k, v := range source.VaultEnv {
		vaultEnv[k] = v
	}
	secretEnv := make([]store.SecretEnvRef, len(source.SecretEnv))
	copy(secretEnv, source.SecretEnv)

	var resources *store.ServiceResources
	if source.Resources != nil {
		v := *source.Resources
		resources = &v
	}
	var health *store.ServiceHealth
	if source.Health != nil {
		v := *source.Health
		if v.Readiness != nil {
			r := *v.Readiness
			v.Readiness = &r
		}
		if v.Liveness != nil {
			l := *v.Liveness
			v.Liveness = &l
		}
		health = &v
	}
	var hooks *store.ServiceHooks
	if source.Hooks != nil {
		v := *source.Hooks
		hooks = &v
	}
	var egress *store.ServiceEgressPolicy
	if source.Egress != nil {
		v := *source.Egress
		v.Allow = append([]store.ServiceEgressAllow(nil), source.Egress.Allow...)
		egress = &v
	}

	return store.DesiredService{
		Name:                 newName,
		Image:                source.Image,
		Port:                 source.Port,
		HostPort:             nil, // cleared: two environments must never fight over one host port pin
		BindAddress:          source.BindAddress,
		Domains:              append([]string(nil), domains...),
		Env:                  env,
		Command:              append([]string(nil), source.Command...),
		Entrypoint:           append([]string(nil), source.Entrypoint...),
		PullPolicy:           source.PullPolicy,
		RegistryCredentialID: source.RegistryCredentialID,
		SecretEnv:            secretEnv,
		VaultEnv:             vaultEnv,
		Resources:            resources,
		Health:               health,
		Hooks:                hooks,
		Egress:               egress,
		Volumes:              cloneServiceVolumes(source, newName),
		BindMounts:           append([]store.ServiceBindMount(nil), source.BindMounts...),
		Labels:               labels,
		Strategy:             source.Strategy,
		Replicas:             source.Replicas,
	}
}

// cloneServiceVolumes rewrites source's own Docker volume names for
// newName: ServiceVolume.Name is already resolved/prefixed
// ("app-"+serviceName+"-"+logicalName, see store.ServiceVolumeDockerName's
// own doc comment), so a straight copy would silently point the clone at
// the source's own volumes instead of new, empty ones. This produces
// fresh (empty) volume names; it never copies volume *data* -- that's
// deliberately out of scope, matching this whole operation cloning
// config, not state.
func cloneServiceVolumes(source store.DesiredService, newName string) []store.ServiceVolume {
	if len(source.Volumes) == 0 {
		return nil
	}
	oldPrefix := "app-" + source.Name + "-"
	newPrefix := "app-" + newName + "-"
	out := make([]store.ServiceVolume, len(source.Volumes))
	for i, v := range source.Volumes {
		name := v.Name
		if strings.HasPrefix(name, oldPrefix) {
			name = newPrefix + strings.TrimPrefix(name, oldPrefix)
		}
		out[i] = store.ServiceVolume{Name: name, ContainerPath: v.ContainerPath}
	}
	return out
}

var slugInvalidChars = regexp.MustCompile(`[^a-z0-9-]+`)

// slugify lowercases s and collapses every run of characters outside
// a-z0-9- into a single '-', trimming leading/trailing '-': the same
// DNS-label-safe shape a cloned app's own name needs, since it feeds
// both desired_services.name and, indirectly, its Docker volume names
// (cloneServiceVolumes).
func slugify(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	slug := slugInvalidChars.ReplaceAllString(lower, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "env"
	}
	return slug
}

// suggestClonedAppName is the default new name for sourceApp when a
// clone request doesn't override it: sourceApp with the new
// environment's own slugified name appended. desired_services.name is
// globally unique (unlike an environment tag), so a straight copy of
// sourceApp would always collide with the source app itself.
func suggestClonedAppName(sourceApp, newEnvironmentName string) string {
	return sourceApp + "-" + slugify(newEnvironmentName)
}
