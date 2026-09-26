package iac

import (
	"context"
	"fmt"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

func appBody(f appFields) map[string]any {
	body := map[string]any{
		"image": f.Image, "port": f.Port, "bind_address": f.BindAddress, "strategy": f.Strategy, "replicas": f.Replicas,
	}
	setOrClear(body, "domains", f.Domains, len(f.Domains) > 0)
	setOrClear(body, "env", f.Env, len(f.Env) > 0)
	setOrClear(body, "secret_env", f.SecretEnv, len(f.SecretEnv) > 0)
	setOrClear(body, "labels", f.Labels, len(f.Labels) > 0)
	setOrClear(body, "command", f.Command, len(f.Command) > 0)
	if f.HostPort > 0 {
		body["host_port"] = f.HostPort
	} else {
		delete(body, "host_port")
	}
	setOrClear(body, "resources", resourcesToStore(f.Resources), f.Resources != nil)
	setOrClear(body, "health", healthToStore(f.Health), f.Health != nil)
	var hooks *store.ServiceHooks
	if f.Hooks != nil {
		hooks = &store.ServiceHooks{PreDeploy: f.Hooks.PreDeploy, PostDeploy: f.Hooks.PostDeploy}
	}
	setOrClear(body, "hooks", hooks, hooks != nil)
	return body
}

func setOrClear(body map[string]any, key string, v any, present bool) {
	if present {
		body[key] = v
	} else {
		body[key] = nil
	}
}

func (x *executor) createApp(ctx context.Context, r *Resource, f map[string]any, setSecrets []string) error {
	var fields appFields
	if err := remarshal(f, &fields); err != nil {
		return fmt.Errorf("encode app: %w", err)
	}
	body := appBody(fields)
	body["name"] = r.Name
	for k, v := range body {
		if v == nil {
			delete(body, k)
		}
	}
	if fields.Project != "" {
		pid, err := x.projectID(fields.Project)
		if err != nil {
			return err
		}
		body["project_id"] = pid
	}
	if err := x.do(ctx, http.MethodPost, "/api/v1/apps", body, nil); err != nil {
		return err
	}
	return x.afterApp(ctx, r, fields, nil, false, setSecrets)
}

// updateApp saves the merged app through PUT (a full replace) starting from
// the live body, so settings apply does not manage survive the write.
func (x *executor) updateApp(ctx context.Context, r *Resource, f map[string]any, live *liveApp, prune bool, setSecrets []string) error {
	var fields appFields
	if err := remarshal(f, &fields); err != nil {
		return fmt.Errorf("encode app: %w", err)
	}
	merged := fields
	lw := live.wire
	if !prune {
		merged.Env = mergeEnv(lw.Env, fields.Env)
		merged.Domains = sortedUnique(append(append([]string{}, lw.Domains...), fields.Domains...))
		merged.SecretEnv = sortedUnique(append(append([]string{}, lw.SecretEnv...), fields.SecretEnv...))
		merged.Labels = mergeEnv(lw.Labels, fields.Labels)
	}
	body := copyFields(live.raw)
	for k, v := range appBody(merged) {
		body[k] = v
	}
	for k, v := range body {
		if v == nil {
			delete(body, k)
		}
	}
	var out struct {
		EnvDirty bool `json:"env_dirty"`
	}
	if err := x.do(ctx, http.MethodPut, "/api/v1/apps/"+esc(r.Name), body, &out); err != nil {
		return err
	}
	return x.afterApp(ctx, r, fields, &live.wire, prune, setSecrets, out.EnvDirty)
}

func (x *executor) afterApp(ctx context.Context, r *Resource, want appFields, live *wireApp, prune bool, setSecrets []string, envDirty ...bool) error {
	app := "/api/v1/apps/" + esc(r.Name)
	if want.Project != "" && (live == nil || x.st.projectsByID[live.ProjectID].Name != want.Project) {
		pid, err := x.projectID(want.Project)
		if err != nil {
			return err
		}
		if live != nil {
			if err := x.do(ctx, http.MethodPut, app+"/project", map[string]string{"project_id": pid}, nil); err != nil {
				return err
			}
		}
	}
	if want.Environment != "" && (live == nil || x.st.envsByID[live.EnvironmentID].Name != want.Environment) {
		eid, err := x.environmentID(want.Project, want.Environment)
		if err != nil {
			return err
		}
		if err := x.do(ctx, http.MethodPut, app+"/environment", map[string]string{"environment_id": eid}, nil); err != nil {
			return err
		}
	}
	if err := x.syncTags(ctx, app, want.Tags, live, prune); err != nil {
		return err
	}
	for _, name := range setSecrets {
		v, _ := lookupSecret(x.opts.Secrets, r.Name, name)
		if err := x.do(ctx, http.MethodPut, app+"/secrets/"+esc(name), map[string]any{"value": v, "overwrite_locked": false}, nil); err != nil {
			return fmt.Errorf("set secret %s: %w", name, err)
		}
	}
	if !x.opts.NoDeploy && live != nil && (len(envDirty) > 0 && envDirty[0] || len(setSecrets) > 0) {
		return x.do(ctx, http.MethodPost, app+"/restart", nil, nil)
	}
	return nil
}

func lookupSecret(m map[string]string, app, name string) (string, bool) {
	if v, ok := m[app+"/"+name]; ok {
		return v, true
	}
	v, ok := m[name]
	return v, ok
}

func (x *executor) syncTags(ctx context.Context, app string, want []string, live *wireApp, prune bool) error {
	have := map[string]bool{}
	if live != nil {
		for _, t := range live.Tags {
			have[t] = true
		}
	}
	for _, t := range want {
		if have[t] {
			continue
		}
		if err := x.do(ctx, http.MethodPost, app+"/tags", map[string]string{"name": t}, nil); err != nil {
			return fmt.Errorf("attach tag %s: %w", t, err)
		}
	}
	if !prune || live == nil {
		return nil
	}
	wantSet := map[string]bool{}
	for _, t := range want {
		wantSet[t] = true
	}
	for _, t := range live.Tags {
		if wantSet[t] {
			continue
		}
		if tag, ok := x.st.tags[t]; ok {
			if err := x.do(ctx, http.MethodDelete, app+"/tags/"+esc(tag.ID), nil, nil); err != nil {
				return fmt.Errorf("detach tag %s: %w", t, err)
			}
		}
	}
	return nil
}
