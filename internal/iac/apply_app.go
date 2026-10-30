package iac

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"net/url"

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
	if f.PublishedPorts != nil {
		body["published_ports"] = *f.PublishedPorts
	}
	setOrClear(body, "resources", resourcesToStore(f.Resources), f.Resources != nil)
	health := healthToStore(f.Health)
	if f.Restart != "" {
		if health == nil {
			health = &store.ServiceHealth{}
		}
		health.Restart = f.Restart
	}
	// An unsent health keeps the stored restart: the API merges it back on PUT.
	setOrClear(body, "health", health, health != nil)
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

// updateApp saves the merged app through PUT starting from the live body,
// so settings apply does not manage survive the write.
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
	// An unset field goes as an explicit null: PUT keeps omitted fields, and apply must clear them.
	for k, v := range appBody(merged) {
		body[k] = v
	}
	if prune && len(merged.SecretEnv) == 0 && len(lw.SecretEnv) > 0 {
		body["secret_env"] = []string{}
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
	if err := x.syncRollout(ctx, r.Name, want.Rollout); err != nil {
		return err
	}
	filesChanged, err := x.syncFiles(ctx, r.Name, want.Files)
	if err != nil {
		return err
	}
	connsChanged, err := x.syncConnections(ctx, r.Name, want.Connections, live, prune)
	if err != nil {
		return err
	}
	volsChanged, err := x.syncVolumes(ctx, r.Name, want.Volumes, live, prune)
	if err != nil {
		return err
	}
	if !x.opts.NoDeploy && live != nil && (len(envDirty) > 0 && envDirty[0] || len(setSecrets) > 0 || connsChanged || volsChanged || filesChanged) {
		return x.do(ctx, http.MethodPost, app+"/restart", nil, nil)
	}
	return nil
}

// syncRollout saves a declared rollout policy; an undeclared one is left alone.
func (x *executor) syncRollout(ctx context.Context, app string, want *rolloutFields) error {
	if want == nil {
		return nil
	}
	if la, ok := x.st.apps[app]; ok && la.rollout != nil && *la.rollout == *want {
		return nil
	}
	if err := x.do(ctx, http.MethodPut, "/api/v1/apps/"+esc(app)+"/rollout", want, nil); err != nil {
		return fmt.Errorf("set rollout: %w", err)
	}
	return nil
}

// syncFiles makes the live non-secret files equal the declared list; an
// undeclared list is left alone. Files are fixed at container create, so a
// change reports true for a restart.
func (x *executor) syncFiles(ctx context.Context, app string, want []fileFields) (bool, error) {
	if len(want) == 0 {
		return false, nil
	}
	var have []fileFields
	if la, ok := x.st.apps[app]; ok {
		have = la.files
	}
	byPath := make(map[string]fileFields, len(have))
	for _, f := range have {
		byPath[f.Path] = f
	}
	base := "/api/v1/apps/" + esc(app) + "/files"
	changed := false
	for _, f := range want {
		same := byPath[f.Path] == f
		delete(byPath, f.Path)
		if same {
			continue
		}
		changed = true
		if err := x.do(ctx, http.MethodPut, base, map[string]any{"path": f.Path, "content": f.Content, "mode": f.Mode}, nil); err != nil {
			return false, fmt.Errorf("set file %s: %w", f.Path, err)
		}
	}
	for _, p := range sortedKeys(byPath) {
		changed = true
		if err := x.do(ctx, http.MethodDelete, base+"?path="+url.QueryEscape(p), nil, nil); err != nil {
			return false, fmt.Errorf("remove file %s: %w", p, err)
		}
	}
	return changed, nil
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

// syncVolumes attaches declared volumes by name through the app volumes
// endpoint, which replaces the whole mount list, so unlisted live volumes
// are re-sent unless prune detaches them. A document without volumes never
// touches them. Detaching never deletes the
// Docker volume or its data.
func (x *executor) syncVolumes(ctx context.Context, app string, want map[string]string, live *wireApp, prune bool) (bool, error) {
	have := map[string]string{}
	if live != nil {
		have = appFromWire(*live, idNames{x.st}).Volumes
	}
	merged := map[string]string{}
	if !prune {
		for n, p := range have {
			merged[n] = p
		}
	}
	for n, p := range want {
		merged[n] = p
	}
	if len(want) == 0 || maps.Equal(have, merged) {
		return false, nil
	}
	list := make([]map[string]string, 0, len(merged))
	for _, n := range sortedKeys(merged) {
		list = append(list, map[string]string{"name": n, "container_path": merged[n]})
	}
	if err := x.do(ctx, http.MethodPut, "/api/v1/apps/"+esc(app)+"/volumes", map[string]any{"volumes": list}, nil); err != nil {
		return false, fmt.Errorf("set volumes: %w", err)
	}
	return true, nil
}
