package iac

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// ExportOptions select what to export.
type ExportOptions struct {
	Project string
	App     string
	// IncludeEnvValues writes literal env values; when false every value
	// becomes a ${{ env.NAME }} placeholder.
	IncludeEnvValues bool
}

// ExportFile is one exported document.
type ExportFile struct {
	Name    string `json:"name"`
	Kind    Kind   `json:"kind"`
	Content string `json:"content"`
}

// ExportResult is the exported documents in stable order plus warnings for
// live settings the documents cannot express.
type ExportResult struct {
	Files    []ExportFile `json:"files"`
	Warnings []string     `json:"warnings,omitempty"`
}

type outDoc struct {
	Version  int     `yaml:"version"`
	Kind     Kind    `yaml:"kind"`
	Metadata outMeta `yaml:"metadata"`
	Spec     any     `yaml:"spec,omitempty"`
}

type outMeta struct {
	Name string `yaml:"name"`
}

type outBuild struct {
	Type  string `yaml:"type"`
	Image string `yaml:"image"`
}

type outService struct {
	Build       outBuild          `yaml:"build"`
	Domains     []string          `yaml:"domains,omitempty"`
	Port        int               `yaml:"port,omitempty"`
	HostPort    int               `yaml:"host_port,omitempty"`
	BindAddress string            `yaml:"bind_address,omitempty"`
	Health      *healthFields     `yaml:"health,omitempty"`
	Resources   *resourceFields   `yaml:"resources,omitempty"`
	Env         map[string]any    `yaml:"env,omitempty"`
	Replicas    int               `yaml:"replicas,omitempty"`
	Strategy    string            `yaml:"strategy,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
	Hooks       *hookFields       `yaml:"hooks,omitempty"`
	Command     []string          `yaml:"command,omitempty"`
}

type outAppSpec struct {
	Project     string     `yaml:"project,omitempty"`
	Environment string     `yaml:"environment,omitempty"`
	Tags        []string   `yaml:"tags,omitempty"`
	Service     outService `yaml:"service"`
}

type exporter struct {
	d     Doer
	opts  ExportOptions
	st    *State
	docs  []outDoc
	warns []string
	tags  map[string]bool
}

// Export reads live state and renders it as documents. Output is stable:
// no timestamps or generated ids, sorted keys and a fixed document order.
func Export(ctx context.Context, d Doer, opts ExportOptions) (ExportResult, error) {
	e := &exporter{d: d, opts: opts, st: newState(), tags: map[string]bool{}}
	l := loader{d: d, st: e.st}
	if err := l.base(ctx, nil); err != nil {
		return ExportResult{}, err
	}
	if opts.Project != "" {
		if _, ok := e.st.projects[opts.Project]; !ok {
			return ExportResult{}, fmt.Errorf("project %q was not found", opts.Project)
		}
	}
	if err := e.projects(ctx, l); err != nil {
		return ExportResult{}, err
	}
	apps, err := e.selectApps(ctx)
	if err != nil {
		return ExportResult{}, err
	}
	for _, a := range apps {
		if err := e.app(ctx, l, a); err != nil {
			return ExportResult{}, err
		}
	}
	e.databases()
	for _, t := range sortedKeys(e.tags) {
		e.docs = append(e.docs, outDoc{Version: SchemaVersion, Kind: KindTag, Metadata: outMeta{Name: t}})
	}
	return e.render()
}

func (e *exporter) projects(ctx context.Context, l loader) error {
	if e.opts.App != "" {
		return nil
	}
	for _, name := range sortedKeys(e.st.projects) {
		if e.opts.Project != "" && name != e.opts.Project {
			continue
		}
		p := e.st.projects[name]
		var env map[string]string
		if err := l.get(ctx, "/api/v1/projects/"+esc(p.ID)+"/env", &env); err != nil && !IsDenied(err) {
			return fmt.Errorf("read env of project %s: %w", name, err)
		}
		pd := outDoc{Version: SchemaVersion, Kind: KindProject, Metadata: outMeta{Name: name}}
		if m := e.envSpec(env); m != nil {
			pd.Spec = m
		}
		e.docs = append(e.docs, pd)
		var envs []wireEnvironment
		if err := l.get(ctx, "/api/v1/projects/"+esc(p.ID)+"/environments", &envs); err != nil && !IsDenied(err) {
			return fmt.Errorf("list environments of %s: %w", name, err)
		}
		for _, en := range envs {
			e.st.envs[envKey(name, en.Name)] = en
			e.st.envsByID[en.ID] = en
			var eenv map[string]string
			if err := l.get(ctx, "/api/v1/environments/"+esc(en.ID)+"/env", &eenv); err != nil && !IsDenied(err) {
				return fmt.Errorf("read env of environment %s/%s: %w", name, en.Name, err)
			}
			spec := map[string]any{"project": name}
			if en.Protected {
				spec["protected"] = true
			}
			if m := e.envSpec(eenv); m != nil {
				spec["env"] = m["env"]
			}
			e.docs = append(e.docs, outDoc{Version: SchemaVersion, Kind: KindEnvironment, Metadata: outMeta{Name: en.Name}, Spec: spec})
		}
	}
	return nil
}

func (e *exporter) envSpec(env map[string]string) map[string]any {
	if len(env) == 0 {
		return nil
	}
	return map[string]any{"env": e.envValues(env)}
}

func (e *exporter) envValues(env map[string]string) map[string]any {
	out := make(map[string]any, len(env))
	for k, v := range env {
		if e.opts.IncludeEnvValues && !looksSecret(k, v) {
			out[k] = v
			continue
		}
		if e.opts.IncludeEnvValues {
			e.warns = append(e.warns, fmt.Sprintf("env %s looks like a secret and was written as a placeholder", k))
		}
		out[k] = "${{ env." + k + " }}"
	}
	return out
}

func (e *exporter) selectApps(ctx context.Context) ([]wireApp, error) {
	if e.opts.App != "" {
		var w wireApp
		if err := e.d.Do(ctx, http.MethodGet, "/api/v1/apps/"+esc(e.opts.App), nil, &w); err != nil {
			return nil, fmt.Errorf("read app %s: %w", e.opts.App, err)
		}
		return []wireApp{w}, nil
	}
	var list []wireApp
	if err := e.d.Do(ctx, http.MethodGet, "/api/v1/apps", nil, &list); err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	var out []wireApp
	for _, a := range list {
		if e.opts.Project != "" && e.st.projectsByID[a.ProjectID].Name != e.opts.Project {
			continue
		}
		var full wireApp
		if err := e.d.Do(ctx, http.MethodGet, "/api/v1/apps/"+esc(a.Name), nil, &full); err != nil {
			if IsDenied(err) {
				continue
			}
			return nil, fmt.Errorf("read app %s: %w", a.Name, err)
		}
		out = append(out, full)
	}
	return out, nil
}

func (e *exporter) app(ctx context.Context, l loader, a wireApp) error {
	if err := e.ensureEnvs(ctx, l, a.ProjectID); err != nil {
		return err
	}
	f := appFromWire(a, idNames{e.st})
	svc := outService{Build: outBuild{Type: spec.BuildImage, Image: f.Image}, Domains: f.Domains, Port: f.Port, HostPort: f.HostPort,
		Health: f.Health, Resources: f.Resources, Labels: f.Labels, Hooks: f.Hooks, Command: f.Command}
	if f.BindAddress != bindaddr.Default {
		svc.BindAddress = f.BindAddress
	}
	if f.Replicas != spec.DefaultReplicas {
		svc.Replicas = f.Replicas
	}
	if f.Strategy != spec.StrategyBlueGreen {
		svc.Strategy = f.Strategy
	}
	env := e.envValues(f.Env)
	for _, n := range f.SecretEnv {
		env[n] = map[string]string{"secretRef": n}
	}
	if len(env) > 0 {
		svc.Env = env
	}
	if u := unsupportedApp(a); len(u) > 0 {
		e.warns = append(e.warns, fmt.Sprintf("app %s: %s are not exported and are kept when the export is applied", a.Name, strings.Join(u, ", ")))
	}
	if strings.HasPrefix(f.Image, ":") || f.Image == "" {
		e.warns = append(e.warns, fmt.Sprintf("app %s has no built image yet; its export cannot be applied until it does", a.Name))
	}
	var tags []string
	for _, t := range f.Tags {
		if !strings.HasPrefix(t, tagManagedPre) {
			tags = append(tags, t)
			e.tags[t] = true
		}
	}
	e.docs = append(e.docs, outDoc{Version: SchemaVersion, Kind: KindApp, Metadata: outMeta{Name: a.Name},
		Spec: outAppSpec{Project: f.Project, Environment: f.Environment, Tags: tags, Service: svc}})
	return e.appExtras(ctx, l, a.Name)
}

func (e *exporter) ensureEnvs(ctx context.Context, l loader, projectID string) error {
	p, ok := e.st.projectsByID[projectID]
	if !ok {
		return nil
	}
	for _, en := range e.st.envs {
		if en.ProjectID == projectID {
			return nil
		}
	}
	var envs []wireEnvironment
	if err := l.fetch(ctx, "environments/"+p.Name, "list environments of "+p.Name, "/api/v1/projects/"+esc(projectID)+"/environments", &envs); err != nil {
		return err
	}
	for _, en := range envs {
		e.st.envs[envKey(p.Name, en.Name)] = en
		e.st.envsByID[en.ID] = en
	}
	return nil
}

func (e *exporter) appExtras(ctx context.Context, l loader, name string) error {
	e.st.apps[name] = &liveApp{}
	if err := l.lb(ctx, name); err != nil {
		return err
	}
	if lb := e.st.lbs[name]; lb.Configured {
		var cfg loadbalancer.Config
		if err := remarshal(lb.Config, &cfg); err == nil {
			e.docs = append(e.docs, outDoc{Version: SchemaVersion, Kind: KindLoadBalancer, Metadata: outMeta{Name: name}, Spec: cfg})
		}
	}
	if err := l.pipelinesOf(ctx, name); err != nil {
		return err
	}
	for _, pn := range sortedKeys(e.st.pipelines[name]) {
		p := e.st.pipelines[name][pn]
		if p.Source == "repo" {
			continue
		}
		e.docs = append(e.docs, outDoc{Version: SchemaVersion, Kind: KindPipeline, Metadata: outMeta{Name: pn},
			Spec: map[string]any{"app": name, "yaml": strings.TrimRight(p.YAML, "\n") + "\n", "enabled": p.Enabled}})
	}
	if len(e.st.channels) == 0 {
		if err := l.channels(ctx); err != nil {
			return err
		}
	}
	if err := l.alertsOf(ctx, name); err != nil {
		return err
	}
	for _, an := range sortedKeys(e.st.alerts[name]) {
		f := alertFromWire(e.st, e.st.alerts[name][an])
		f["app"] = name
		e.docs = append(e.docs, outDoc{Version: SchemaVersion, Kind: KindAlertRule, Metadata: outMeta{Name: an}, Spec: f})
	}
	return nil
}

func (e *exporter) databases() {
	if e.opts.App != "" {
		return
	}
	for _, name := range sortedKeys(e.st.databases) {
		db := e.st.databases[name]
		project := e.st.projectsByID[db.ProjectID].Name
		if e.opts.Project != "" && project != e.opts.Project {
			continue
		}
		s := map[string]any{"engine": db.Engine}
		putIf(s, "version", db.Version)
		putIf(s, "project", project)
		e.docs = append(e.docs, outDoc{Version: SchemaVersion, Kind: KindDatabase, Metadata: outMeta{Name: name}, Spec: s})
	}
}

func (e *exporter) render() (ExportResult, error) {
	sort.SliceStable(e.docs, func(i, j int) bool {
		if ri, rj := kindRank(e.docs[i].Kind), kindRank(e.docs[j].Kind); ri != rj {
			return ri < rj
		}
		return docSortKey(e.docs[i]) < docSortKey(e.docs[j])
	})
	res := ExportResult{Warnings: sortedUnique(e.warns)}
	for _, doc := range e.docs {
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(doc); err != nil {
			return ExportResult{}, fmt.Errorf("encode %s %s: %w", doc.Kind, doc.Metadata.Name, err)
		}
		if err := enc.Close(); err != nil {
			return ExportResult{}, fmt.Errorf("encode %s %s: %w", doc.Kind, doc.Metadata.Name, err)
		}
		res.Files = append(res.Files, ExportFile{Name: fileName(doc), Kind: doc.Kind, Content: buf.String()})
	}
	return res, nil
}

func docSortKey(d outDoc) string {
	if m, ok := d.Spec.(map[string]any); ok {
		if a, ok := m["app"].(string); ok {
			return a + "/" + d.Metadata.Name
		}
		if p, ok := m["project"].(string); ok {
			return p + "/" + d.Metadata.Name
		}
	}
	return d.Metadata.Name
}

func fileName(d outDoc) string {
	name := strings.ToLower(string(d.Kind)) + "-" + d.Metadata.Name
	if m, ok := d.Spec.(map[string]any); ok {
		if a, ok := m["app"].(string); ok {
			name = strings.ToLower(string(d.Kind)) + "-" + a + "-" + d.Metadata.Name
		} else if p, ok := m["project"].(string); ok && d.Kind == KindEnvironment {
			name = "environment-" + p + "-" + d.Metadata.Name
		}
	}
	return name + ".yaml"
}

// Join concatenates exported files into one multi-document stream.
func (r ExportResult) Join() string {
	var b strings.Builder
	for i, f := range r.Files {
		if i > 0 {
			b.WriteString("---\n")
		}
		b.WriteString(f.Content)
	}
	return b.String()
}
