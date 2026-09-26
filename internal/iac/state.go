package iac

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
)

type liveApp struct {
	wire    wireApp
	raw     map[string]any
	secrets map[string]bool
}

// State is the live control plane data a plan needs, read through the API
// with the caller's own permissions. A read the caller may not perform is
// recorded in denied instead of failing the whole plan.
type State struct {
	projects     map[string]wireProject
	projectsByID map[string]wireProject
	envs         map[string]wireEnvironment
	envsByID     map[string]wireEnvironment
	tags         map[string]wireTag
	databases    map[string]wireDatabase
	apps         map[string]*liveApp
	lbs          map[string]wireLB
	pipelines    map[string]map[string]wirePipeline
	alerts       map[string]map[string]wireAlert
	channels     map[string]string
	projectEnv   map[string]map[string]string
	envEnv       map[string]map[string]string
	managed      map[string]bool
	denied       map[string]error
}

type idNames struct{ st *State }

func (n idNames) project(id string) string {
	if id == "" || n.st == nil {
		return ""
	}
	return n.st.projectsByID[id].Name
}

func (n idNames) environment(id string) string {
	if id == "" || n.st == nil {
		return ""
	}
	return n.st.envsByID[id].Name
}

func envKey(project, name string) string { return project + "/" + name }

func newState() *State {
	return &State{
		projects: map[string]wireProject{}, projectsByID: map[string]wireProject{}, envs: map[string]wireEnvironment{}, envsByID: map[string]wireEnvironment{},
		tags: map[string]wireTag{}, databases: map[string]wireDatabase{}, apps: map[string]*liveApp{}, lbs: map[string]wireLB{},
		pipelines: map[string]map[string]wirePipeline{}, alerts: map[string]map[string]wireAlert{}, channels: map[string]string{},
		projectEnv: map[string]map[string]string{}, envEnv: map[string]map[string]string{}, managed: map[string]bool{}, denied: map[string]error{},
	}
}

// IsUnavailable reports whether err says the feature is not configured.
func IsUnavailable(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Status == http.StatusNotImplemented
}

func (st *State) note(key string, err error) error {
	if IsDenied(err) || IsUnavailable(err) {
		st.denied[key] = err
		return nil
	}
	return err
}

// Load reads the live state the resources and prune scope need.
func Load(ctx context.Context, d Doer, res []*Resource, opts Options) (*State, error) {
	st := newState()
	l := loader{d: d, st: st}
	if err := l.base(ctx, res); err != nil {
		return nil, err
	}
	appNames := referencedApps(res)
	if opts.Prune && opts.Source != "" {
		if err := l.managedApps(ctx, opts.Source, appNames); err != nil {
			return nil, err
		}
	}
	for _, name := range sortedKeys(appNames) {
		if err := l.app(ctx, name); err != nil {
			return nil, err
		}
	}
	if err := l.scoped(ctx, res); err != nil {
		return nil, err
	}
	for _, name := range st.ManagedApps() {
		if err := l.lb(ctx, name); err != nil {
			return nil, err
		}
	}
	return st, nil
}

func referencedApps(res []*Resource) map[string]bool {
	out := map[string]bool{}
	for _, r := range res {
		switch r.Kind {
		case KindApp, KindLoadBalancer:
			out[r.Name] = true
		case KindDomain:
			if a, _ := r.Fields["app"].(string); a != "" {
				out[a] = true
			}
		case KindPipeline, KindAlertRule:
			out[r.Scope] = true
		}
	}
	return out
}

type loader struct {
	d  Doer
	st *State
}

func (l loader) get(ctx context.Context, path string, out any) error {
	return l.d.Do(ctx, http.MethodGet, path, nil, out)
}

// fetch reads path; a permission failure is recorded under key and is not
// fatal, anything else is.
func (l loader) fetch(ctx context.Context, key, what, path string, out any) error {
	err := l.get(ctx, path, out)
	if err == nil {
		return nil
	}
	return l.st.note(key, fmt.Errorf("%s: %w", what, err))
}

func (l loader) base(ctx context.Context, res []*Resource) error {
	var projects []wireProject
	if err := l.fetch(ctx, "projects", "list projects", "/api/v1/projects", &projects); err != nil {
		return err
	}
	for _, p := range projects {
		l.st.projects[p.Name] = p
		l.st.projectsByID[p.ID] = p
	}
	var tags []wireTag
	if err := l.fetch(ctx, "tags", "list tags", "/api/v1/tags", &tags); err != nil {
		return err
	}
	for _, t := range tags {
		l.st.tags[t.Name] = t
	}
	var dbs []wireDatabase
	if err := l.fetch(ctx, "databases", "list databases", "/api/v1/databases", &dbs); err != nil {
		return err
	}
	for _, db := range dbs {
		l.st.databases[db.Name] = db
	}
	for _, r := range res {
		if r.Kind == KindAlertRule {
			return l.channels(ctx)
		}
	}
	return nil
}

func (l loader) channels(ctx context.Context) error {
	var chans []wireChannel
	if err := l.fetch(ctx, "channels", "list notification channels", "/api/v1/notification-channels", &chans); err != nil {
		return err
	}
	for _, c := range chans {
		l.st.channels[c.Name] = c.ID
	}
	return nil
}

func (l loader) managedApps(ctx context.Context, source string, apps map[string]bool) error {
	tag, ok := l.st.tags[tagManagedPre+source]
	if !ok {
		return nil
	}
	var members []struct {
		Name string `json:"name"`
	}
	if err := l.fetch(ctx, "managed", "list apps managed by "+source, "/api/v1/tags/"+esc(tag.ID)+"/apps", &members); err != nil {
		return err
	}
	for _, m := range members {
		l.st.managed[m.Name] = true
		apps[m.Name] = true
	}
	return nil
}

func (l loader) app(ctx context.Context, name string) error {
	var raw map[string]any
	key := resourceKey(KindApp, "", name)
	if err := l.get(ctx, "/api/v1/apps/"+esc(name), &raw); err != nil {
		if IsNotFound(err) {
			return nil
		}
		return l.st.note(key, fmt.Errorf("read app %s: %w", name, err))
	}
	var w wireApp
	if err := remarshal(raw, &w); err != nil {
		return fmt.Errorf("decode app %s: %w", name, err)
	}
	la := &liveApp{wire: w, raw: raw, secrets: map[string]bool{}}
	l.st.apps[name] = la
	var keys []wireSecretKey
	if err := l.get(ctx, "/api/v1/apps/"+esc(name)+"/secrets", &keys); err != nil {
		var se *StatusError
		if !errors.As(err, &se) {
			return fmt.Errorf("list secrets for %s: %w", name, err)
		}
	}
	for _, k := range keys {
		la.secrets[k.Key] = true
	}
	return nil
}

func (l loader) scoped(ctx context.Context, res []*Resource) error {
	loadedProjects := map[string]bool{}
	loadEnvs := func(project string) error {
		p, ok := l.st.projects[project]
		if !ok || loadedProjects[project] {
			return nil
		}
		loadedProjects[project] = true
		var envs []wireEnvironment
		if err := l.fetch(ctx, "environments/"+project, "list environments of "+project, "/api/v1/projects/"+esc(p.ID)+"/environments", &envs); err != nil {
			return err
		}
		for _, e := range envs {
			l.st.envs[envKey(project, e.Name)] = e
			l.st.envsByID[e.ID] = e
		}
		return nil
	}
	for _, r := range res {
		if err := l.scopedOne(ctx, r, loadEnvs); err != nil {
			return err
		}
	}
	for _, a := range l.st.apps {
		if a.wire.ProjectID != "" {
			if p, ok := l.st.projectsByID[a.wire.ProjectID]; ok {
				if err := loadEnvs(p.Name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (l loader) scopedOne(ctx context.Context, r *Resource, loadEnvs func(string) error) error {
	switch r.Kind {
	case KindProject:
		if p, ok := l.st.projects[r.Name]; ok {
			return l.envMap(ctx, "/api/v1/projects/"+esc(p.ID)+"/env", "projects/"+r.Name, l.st.projectEnv, r.Name)
		}
	case KindEnvironment:
		if err := loadEnvs(r.Scope); err != nil {
			return err
		}
		if e, ok := l.st.envs[envKey(r.Scope, r.Name)]; ok {
			return l.envMap(ctx, "/api/v1/environments/"+esc(e.ID)+"/env", r.Key(), l.st.envEnv, envKey(r.Scope, r.Name))
		}
	case KindApp:
		for _, p := range []string{stringField(r.Fields, "project")} {
			if p != "" {
				return loadEnvs(p)
			}
		}
	case KindLoadBalancer:
		return l.lb(ctx, r.Name)
	case KindPipeline:
		return l.pipelinesOf(ctx, r.Scope)
	case KindAlertRule:
		return l.alertsOf(ctx, r.Scope)
	}
	return nil
}

func stringField(f map[string]any, k string) string {
	s, _ := f[k].(string)
	return s
}

func (l loader) envMap(ctx context.Context, path, key string, dst map[string]map[string]string, id string) error {
	var m map[string]string
	if err := l.get(ctx, path, &m); err != nil {
		return l.st.note(key, fmt.Errorf("read env of %s: %w", key, err))
	}
	dst[id] = m
	return nil
}

func (l loader) lb(ctx context.Context, app string) error {
	if _, ok := l.st.apps[app]; !ok {
		return nil
	}
	var lb wireLB
	if err := l.get(ctx, "/api/v1/apps/"+esc(app)+"/loadbalancer", &lb); err != nil {
		return l.st.note(resourceKey(KindLoadBalancer, "", app), fmt.Errorf("read load balancer of %s: %w", app, err))
	}
	l.st.lbs[app] = lb
	return nil
}

func (l loader) pipelinesOf(ctx context.Context, app string) error {
	if _, ok := l.st.apps[app]; !ok || l.st.pipelines[app] != nil {
		return nil
	}
	var list []wirePipeline
	if err := l.get(ctx, "/api/v1/apps/"+esc(app)+"/pipelines", &list); err != nil {
		return l.st.note("pipelines/"+app, fmt.Errorf("list pipelines of %s: %w", app, err))
	}
	m := map[string]wirePipeline{}
	for _, p := range list {
		if p.YAML == "" {
			var full wirePipeline
			if err := l.get(ctx, "/api/v1/apps/"+esc(app)+"/pipelines/"+esc(p.Name), &full); err == nil {
				p = full
			}
		}
		m[p.Name] = p
	}
	l.st.pipelines[app] = m
	return nil
}

func (l loader) alertsOf(ctx context.Context, app string) error {
	if _, ok := l.st.apps[app]; !ok || l.st.alerts[app] != nil {
		return nil
	}
	var list []wireAlert
	if err := l.get(ctx, "/api/v1/apps/"+esc(app)+"/alerts", &list); err != nil {
		return l.st.note("alerts/"+app, fmt.Errorf("list alert rules of %s: %w", app, err))
	}
	m := map[string]wireAlert{}
	for _, a := range list {
		m[a.Name] = a
	}
	l.st.alerts[app] = m
	return nil
}

// ManagedApps lists the apps carrying this source's managed-by tag.
func (st *State) ManagedApps() []string {
	out := make([]string, 0, len(st.managed))
	for n := range st.managed {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
