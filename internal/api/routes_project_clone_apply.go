package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/store"
)

// suspendedServiceSaver is the atomic save-as-stopped some stores offer, so
// no reconcile pass can start a clone before it is suspended.
type suspendedServiceSaver interface {
	SaveSuspendedDesiredService(ctx context.Context, svc store.DesiredService) error
}

// cloneProjectScope reads the project (and environment) level env: plain
// values travel only when asked, secret keys are always listed, never valued.
func (rt *Router) cloneProjectScope(ctx context.Context, src store.Project, envScoped, includeEnv bool, plan *projectClonePlan) error {
	vars, err := rt.projects.ListProjectEnvVars(ctx, src.ID)
	if err != nil {
		return fmt.Errorf("list project env: %w", err)
	}
	keys, err := rt.projects.ListProjectSecretEnvKeys(ctx, src.ID)
	if err != nil {
		return fmt.Errorf("list project secret keys: %w", err)
	}
	plan.projectEnv, plan.projectSecrets = map[string]string{}, map[string]bool{}
	for _, k := range keys {
		plan.projectSecrets[k] = true
	}
	for k, v := range vars {
		if !includeEnv {
			continue
		}
		if envredact.SensitiveName(k) || envredact.SensitiveValue(k, v) {
			plan.projectSecrets[k] = true
			continue
		}
		plan.projectEnv[k] = v
		plan.EnvVarCount++
	}
	for k := range plan.projectSecrets {
		plan.SecretKeys = append(plan.SecretKeys, projectCloneSecretKey{Scope: "project", Key: k})
	}
	if !envScoped {
		return nil
	}
	envs, err := rt.environments.ListEnvironmentsByProject(ctx, src.ID)
	if err != nil {
		return fmt.Errorf("list environments: %w", err)
	}
	for _, e := range envs {
		if e.Name != plan.Environment {
			continue
		}
		plan.srcKind = e.Kind
		if !includeEnv {
			return nil
		}
		ev, err := rt.environments.ListEnvironmentEnvVars(ctx, e.ID)
		if err != nil {
			return fmt.Errorf("list environment env: %w", err)
		}
		plan.envEnv = map[string]string{}
		for k, v := range ev {
			if envredact.SensitiveName(k) || envredact.SensitiveValue(k, v) {
				plan.SecretKeys = append(plan.SecretKeys, projectCloneSecretKey{Scope: "environment/" + e.Name, Key: k})
				plan.Skipped = append(plan.Skipped, projectCloneSkip{Scope: "environment/" + e.Name, Item: k, Reason: "secret-looking value not copied; set it on the new environment"})
				continue
			}
			plan.envEnv[k] = v
			plan.EnvVarCount++
		}
	}
	return nil
}

// applyProjectClone writes the plan. Apps are saved suspended so a clone
// never starts, or binds a port, until its owner reviews it.
func (rt *Router) applyProjectClone(r *http.Request, plan *projectClonePlan) (err error) {
	ctx := r.Context()
	src, err := rt.projects.GetProject(ctx, plan.Source.ID)
	if err != nil {
		return fmt.Errorf("reload source project: %w", err)
	}
	id, err := randomProjectID()
	if err != nil {
		return fmt.Errorf("generate project id: %w", err)
	}
	p := store.Project{ID: id, Name: plan.Project.Name, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.projects.SaveProject(ctx, p); err != nil {
		return fmt.Errorf("save project: %w", err)
	}
	plan.Project.ID = id
	var created []string
	defer func() {
		if err == nil {
			return
		}
		for _, n := range created {
			if derr := rt.apps.DeleteDesiredService(context.WithoutCancel(ctx), n); derr != nil {
				rt.logger.Warn("api: clone project: rollback app failed", slog.String("app", n), slog.String("error", derr.Error()))
			}
		}
		if derr := rt.projects.DeleteProject(context.WithoutCancel(ctx), id); derr != nil {
			rt.logger.Warn("api: clone project: rollback failed", slog.String("project_id", id), slog.String("error", derr.Error()))
		}
	}()
	if src.OrgID != "" {
		if err := rt.organizations.SetProjectOrganization(ctx, id, src.OrgID); err != nil {
			return fmt.Errorf("assign team: %w", err)
		}
	}
	if err := rt.createProjectDefaultEnvironment(r, id); err != nil {
		return fmt.Errorf("default environment: %w", err)
	}
	envID, err := rt.cloneTargetEnvironment(ctx, id, plan)
	if err != nil {
		return err
	}
	if len(plan.projectEnv) > 0 {
		if err := rt.projects.SetProjectEnvVars(ctx, id, plan.projectEnv); err != nil {
			return fmt.Errorf("set project env: %w", err)
		}
	}
	for k := range plan.projectSecrets {
		if err := rt.projects.SetProjectSecretEnvVar(ctx, id, k); err != nil {
			return fmt.Errorf("register project secret key: %w", err)
		}
	}
	if envID != "" && len(plan.envEnv) > 0 {
		if err := rt.environments.SetEnvironmentEnvVars(ctx, envID, plan.envEnv); err != nil {
			return fmt.Errorf("set environment env: %w", err)
		}
	}
	for i := range plan.services {
		svc := plan.services[i]
		if err := rt.saveClonedService(ctx, svc); err != nil {
			return fmt.Errorf("save app %q: %w", svc.Name, err)
		}
		created = append(created, svc.Name)
		if err := rt.apps.UpdateServiceProject(ctx, svc.Name, id); err != nil {
			return fmt.Errorf("file app %q: %w", svc.Name, err)
		}
		if envID != "" {
			if err := rt.environments.SetServiceEnvironment(ctx, svc.Name, envID); err != nil {
				return fmt.Errorf("tag app %q: %w", svc.Name, err)
			}
		}
	}
	return nil
}

// cloneTargetEnvironment returns the environment the cloned apps join: the
// new project's default one, or a fresh one named like the source environment.
func (rt *Router) cloneTargetEnvironment(ctx context.Context, projectID string, plan *projectClonePlan) (string, error) {
	if s, ok := rt.defaultEnvStore(); ok {
		if id, err := s.DefaultEnvironmentID(ctx, projectID); err == nil && id != "" {
			return id, nil
		}
	}
	if plan.Environment == "" {
		return "", nil
	}
	envID, err := randomEnvironmentID()
	if err != nil {
		return "", fmt.Errorf("generate environment id: %w", err)
	}
	kind := plan.srcKind
	if kind == "" {
		kind = store.EnvironmentKindCustom
	}
	e := store.Environment{ID: envID, ProjectID: projectID, Name: plan.Environment, Kind: kind, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.environments.SaveEnvironment(ctx, e); err != nil {
		return "", fmt.Errorf("save environment: %w", err)
	}
	return envID, nil
}

func (rt *Router) saveClonedService(ctx context.Context, svc store.DesiredService) error {
	// The plan picked free names earlier; re-check under the lock a create also takes.
	defer lockAppName(svc.Name)()
	if err := rt.appNameFree(ctx, svc.Name); err != nil {
		return err
	}
	if s, ok := rt.apps.(suspendedServiceSaver); ok {
		return s.SaveSuspendedDesiredService(ctx, svc)
	}
	if err := rt.apps.SaveDesiredService(ctx, svc); err != nil {
		return err
	}
	return rt.apps.UpdateServiceSuspended(ctx, svc.Name, true)
}

func (rt *Router) auditProjectClone(r *http.Request, sourceID string, plan *projectClonePlan) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: project clone audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWrite, Method: http.MethodPost, Path: "/api/v1/projects/" + sourceID + "/clone", StatusCode: http.StatusCreated,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionProjectClone,
		Detail:     fmt.Sprintf(`{"new_project":%q,"apps":%d,"secret_keys":%d}`, plan.Project.ID, len(plan.Apps), len(plan.SecretKeys)),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: project clone audit save failed", slog.String("error", err.Error()), slog.String("project", sourceID))
	}
}
