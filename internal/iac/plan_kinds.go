package iac

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
)

var additiveEnv = map[string]bool{"env": true}

func envOf(r *Resource) map[string]string {
	switch t := r.Fields["env"].(type) {
	case map[string]string:
		return t
	case map[string]any:
		out := make(map[string]string, len(t))
		for k, v := range t {
			out[k] = fmt.Sprint(v)
		}
		return out
	}
	return nil
}

func (p *planner) envDiff(r *Resource, live map[string]string, extra, liveExtra map[string]any) (changes, kept []FieldChange) {
	want := map[string]any{}
	liveMap := map[string]any{}
	for k, v := range liveExtra {
		liveMap[k] = v
	}
	if e := envOf(r); len(e) > 0 {
		want["env"] = toMap(e)
	}
	if len(live) > 0 {
		liveMap["env"] = toMap(live)
	}
	for k, v := range extra {
		want[k] = v
	}
	return diffFields(want, liveMap, additiveEnv, false)
}

func mergeEnv(live, desired map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range live {
		out[k] = v
	}
	for k, v := range desired {
		out[k] = v
	}
	return out
}

func (p *planner) planProject(r *Resource) {
	if p.deniedItem(r, "projects") || p.deniedItem(r, "projects/"+r.Name) {
		return
	}
	live, ok := p.st.projects[r.Name]
	env := envOf(r)
	if !ok {
		p.add(r, ActionCreate, addAll(r.Fields, additiveEnv), nil, func(ctx context.Context, x *executor) error {
			var out wireProject
			if err := x.do(ctx, http.MethodPost, "/api/v1/projects", map[string]string{"name": r.Name}, &out); err != nil {
				return err
			}
			x.projectIDs[r.Name] = out.ID
			return x.putEnv(ctx, "/api/v1/projects/"+esc(out.ID)+"/env", nil, env)
		})
		return
	}
	changes, kept := p.envDiff(r, p.st.projectEnv[r.Name], nil, nil)
	if len(changes) == 0 {
		p.add(r, ActionNoop, nil, kept, nil)
		return
	}
	p.add(r, ActionUpdate, changes, kept, func(ctx context.Context, x *executor) error {
		return x.putEnv(ctx, "/api/v1/projects/"+esc(live.ID)+"/env", p.st.projectEnv[r.Name], env)
	})
}

func (p *planner) planEnvironment(r *Resource) {
	if p.deniedItem(r, "environments/"+r.Scope) || p.deniedItem(r, r.Key()) {
		return
	}
	if !p.projectKnown(r.Scope) {
		p.fail(r, "project %q does not exist and is not declared in the files", r.Scope)
		return
	}
	protected, _ := r.Fields["protected"].(bool)
	live, ok := p.st.envs[envKey(r.Scope, r.Name)]
	env := envOf(r)
	if !ok {
		p.add(r, ActionCreate, addAll(r.Fields, additiveEnv), nil, func(ctx context.Context, x *executor) error {
			pid, err := x.projectID(r.Scope)
			if err != nil {
				return err
			}
			var out wireEnvironment
			body := map[string]any{"name": r.Name, "protected": protected}
			if err := x.do(ctx, http.MethodPost, "/api/v1/projects/"+esc(pid)+"/environments", body, &out); err != nil {
				return err
			}
			x.envIDs[envKey(r.Scope, r.Name)] = out.ID
			return x.putEnv(ctx, "/api/v1/environments/"+esc(out.ID)+"/env", nil, env)
		})
		return
	}
	extra := map[string]any{}
	if protected {
		extra["protected"] = true
	}
	liveExtra := map[string]any{}
	if live.Protected {
		liveExtra["protected"] = true
	}
	changes, kept := p.envDiff(r, p.st.envEnv[envKey(r.Scope, r.Name)], extra, liveExtra)
	if len(changes) == 0 {
		p.add(r, ActionNoop, nil, kept, nil)
		return
	}
	p.add(r, ActionUpdate, changes, kept, func(ctx context.Context, x *executor) error {
		if live.Protected != protected {
			if err := x.do(ctx, http.MethodPatch, "/api/v1/environments/"+esc(live.ID), map[string]bool{"protected": protected}, nil); err != nil {
				return err
			}
		}
		return x.putEnv(ctx, "/api/v1/environments/"+esc(live.ID)+"/env", p.st.envEnv[envKey(r.Scope, r.Name)], env)
	})
}

func (p *planner) planTag(r *Resource) {
	if p.deniedItem(r, "tags") {
		return
	}
	if _, ok := p.st.tags[r.Name]; ok {
		p.add(r, ActionNoop, nil, nil, nil)
		return
	}
	p.add(r, ActionCreate, nil, nil, func(ctx context.Context, x *executor) error {
		_, err := x.ensureTag(ctx, r.Name)
		return err
	})
}

func (p *planner) planDatabase(r *Resource) {
	if p.deniedItem(r, "databases") {
		return
	}
	project := stringField(r.Fields, "project")
	if project != "" && !p.projectKnown(project) {
		p.fail(r, "project %q does not exist and is not declared in the files", project)
		return
	}
	live, ok := p.st.databases[r.Name]
	if !ok {
		p.add(r, ActionCreate, addAll(r.Fields, nil), nil, func(ctx context.Context, x *executor) error {
			body := map[string]string{"name": r.Name, "engine": stringField(r.Fields, "engine"), "version": stringField(r.Fields, "version")}
			if err := x.do(ctx, http.MethodPost, "/api/v1/databases", body, nil); err != nil {
				return err
			}
			return x.setDatabaseProject(ctx, r.Name, project)
		})
		return
	}
	if e := stringField(r.Fields, "engine"); e != live.Engine {
		p.fail(r, "engine cannot change (live %s, declared %s); databases hold data, recreate it deliberately", live.Engine, e)
		return
	}
	if v := stringField(r.Fields, "version"); v != "" && v != live.Version {
		p.fail(r, "version cannot change through apply (live %s, declared %s)", live.Version, v)
		return
	}
	if project == "" || p.st.projectsByID[live.ProjectID].Name == project {
		p.add(r, ActionNoop, nil, nil, nil)
		return
	}
	ch := []FieldChange{{Path: "project", Op: OpChange, Old: p.st.projectsByID[live.ProjectID].Name, New: project}}
	p.add(r, ActionUpdate, ch, nil, func(ctx context.Context, x *executor) error {
		return x.setDatabaseProject(ctx, r.Name, project)
	})
}

func (p *planner) planDomain(r *Resource) {
	app := stringField(r.Fields, "app")
	if p.deniedItem(r, resourceKey(KindApp, "", app)) {
		return
	}
	if !p.appKnown(app) {
		p.fail(r, "app %q does not exist and is not declared in the files", app)
		return
	}
	if live, ok := p.st.apps[app]; ok {
		for _, d := range live.wire.Domains {
			if d == r.Name {
				p.add(r, ActionNoop, nil, nil, nil)
				return
			}
		}
	}
	fc := []FieldChange{{Path: "app", Op: OpAdd, New: app}}
	p.add(r, ActionCreate, fc, nil, func(ctx context.Context, x *executor) error {
		if x.appsFailed[app] {
			return fmt.Errorf("app %s was not applied", app)
		}
		if x.appsDone[app] {
			return nil
		}
		return x.addDomain(ctx, app, r.Name)
	})
}

func (p *planner) planLB(r *Resource) {
	if p.deniedItem(r, r.Key()) || p.deniedItem(r, resourceKey(KindApp, "", r.Name)) {
		return
	}
	if !p.appKnown(r.Name) {
		p.fail(r, "app %q does not exist and is not declared in the files", r.Name)
		return
	}
	put := func(ctx context.Context, x *executor) error {
		return x.do(ctx, http.MethodPut, "/api/v1/apps/"+esc(r.Name)+"/loadbalancer", r.Fields, nil)
	}
	lb := p.st.lbs[r.Name]
	if !lb.Configured {
		p.add(r, ActionCreate, addAll(r.Fields, nil), nil, put)
		return
	}
	var cfg loadbalancer.Config
	if err := remarshal(lb.Config, &cfg); err != nil {
		p.fail(r, "read live load balancer: %v", err)
		return
	}
	changes, _ := diffFields(r.Fields, toMap(cfg.Defaults()), nil, false)
	if len(changes) == 0 {
		p.add(r, ActionNoop, nil, nil, nil)
		return
	}
	p.add(r, ActionUpdate, changes, nil, put)
}

func (p *planner) planPipeline(r *Resource) {
	if p.deniedItem(r, "pipelines/"+r.Scope) {
		return
	}
	if !p.appKnown(r.Scope) {
		p.fail(r, "app %q does not exist and is not declared in the files", r.Scope)
		return
	}
	base := "/api/v1/apps/" + esc(r.Scope) + "/pipelines"
	body := map[string]any{"name": r.Name, "yaml": r.Fields["yaml"], "enabled": r.Fields["enabled"]}
	live, ok := p.st.pipelines[r.Scope][r.Name]
	if !ok {
		p.add(r, ActionCreate, []FieldChange{{Path: "yaml", Op: OpAdd, New: firstLine(r)}, {Path: "enabled", Op: OpAdd, New: fmt.Sprint(r.Fields["enabled"])}}, nil, func(ctx context.Context, x *executor) error {
			return x.do(ctx, http.MethodPost, base, body, nil)
		})
		return
	}
	if live.Source == "repo" {
		p.fail(r, "this pipeline is synced from the repository; change it there")
		return
	}
	var changes []FieldChange
	if strings.TrimSpace(live.YAML) != strings.TrimSpace(r.Fields["yaml"].(string)) {
		changes = append(changes, FieldChange{Path: "yaml", Op: OpChange, Old: "(current definition)", New: firstLine(r)})
	}
	if live.Enabled != r.Fields["enabled"].(bool) {
		changes = append(changes, FieldChange{Path: "enabled", Op: OpChange, Old: fmt.Sprint(live.Enabled), New: fmt.Sprint(r.Fields["enabled"])})
	}
	if len(changes) == 0 {
		p.add(r, ActionNoop, nil, nil, nil)
		return
	}
	p.add(r, ActionUpdate, changes, nil, func(ctx context.Context, x *executor) error {
		return x.do(ctx, http.MethodPut, base+"/"+esc(r.Name), body, nil)
	})
}

func firstLine(r *Resource) string {
	y, _ := r.Fields["yaml"].(string)
	line, _, _ := strings.Cut(strings.TrimSpace(y), "\n")
	return line + " ..."
}
