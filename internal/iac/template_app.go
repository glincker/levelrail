package iac

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

// templateAppName mirrors the install endpoint's app name rule so a bad name
// fails at build time instead of at apply.
var templateAppName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// templateAppInputs is what the executor sends to the install endpoint. Env
// values stay here and never enter Fields, so a plan or export cannot print them.
type templateAppInputs struct {
	template string
	env      map[string]string
	domains  map[string]string
}

// wireTemplate is the part of GET /api/v1/service-templates/{id} the planner reads.
type wireTemplate struct {
	ID       string `json:"id"`
	Manifest *struct {
		Env []struct {
			Key      string `json:"key"`
			Default  string `json:"default"`
			Generate string `json:"generate"`
			Required bool   `json:"required"`
		} `json:"env"`
		Domains []struct {
			Service string `json:"service"`
		} `json:"domains"`
	} `json:"manifest"`
	missing bool
}

// liveTemplateApp records which templates a live compose app was installed from.
type liveTemplateApp struct{ latest string }

// templateInputsDigest keys env and domains together, so a changed value shows without being stored.
func templateInputsDigest(in *templateAppInputs) string {
	m := map[string]string{}
	for k, v := range in.env {
		m["env."+k] = v
	}
	for k, v := range in.domains {
		m["domains."+k] = v
	}
	return composeEnvDigest(m)
}

func buildTemplateApp(d *Document, opts Options, s *issueSink) *Resource {
	var sp struct {
		Template string            `yaml:"template"`
		Name     string            `yaml:"name"`
		Domains  map[string]string `yaml:"domains"`
	}
	decodeSpec(d, &sp, s)
	name := d.Name
	if sp.Name != "" {
		name = sp.Name
	}
	if !templateAppName.MatchString(name) {
		s.atCode(CodeInvalidName, "spec.name", "app name %q must start with a letter and use only lowercase letters, digits and dashes (40 characters at most)", name)
	}
	if strings.TrimSpace(sp.Template) == "" {
		s.atCode(CodeInvalidDocument, "spec.template", "template is required: a catalog template id")
	}
	if opts.Project != "" {
		s.atCode(CodeInvalidDocument, "kind", "TemplateApp has no project and cannot be applied with a project scope")
	}
	if opts.NoTemplateEnvVars {
		if n := mapValue(d.SpecNode, "env"); n != nil {
			for i := 1; i < len(n.Content); i += 2 {
				if envPlaceRe.MatchString(n.Content[i].Value) {
					s.atCode(CodeInvalidDocument, "spec.env."+n.Content[i-1].Value, "a stack keeps its rendered files, so TemplateApp env cannot use ${{ env.NAME }} there; apply it with the CLI instead")
				}
			}
		}
	}
	env := specEnv(d, s, opts)
	in := &templateAppInputs{template: sp.Template, env: env, domains: sp.Domains}
	f := map[string]any{"template": sp.Template}
	if len(sp.Domains) > 0 {
		f["domains"] = sp.Domains
	}
	if len(env) > 0 {
		// Only the key names are part of the resource, values are keyed-hashed in the plan.
		f["env"] = sortedKeys(env)
	}
	return &Resource{Kind: KindTemplateApp, Name: name, Doc: d, Fields: f, tmpl: in}
}

func (l loader) templateOf(ctx context.Context, id string) error {
	if _, done := l.st.templates[id]; done {
		return nil
	}
	var t wireTemplate
	if err := l.get(ctx, "/api/v1/service-templates/"+esc(id), &t); err != nil {
		if IsNotFound(err) {
			l.st.templates[id] = &wireTemplate{missing: true}
			return nil
		}
		return l.st.note("template/"+id, fmt.Errorf("read template %s: %w", id, err))
	}
	l.st.templates[id] = &t
	return nil
}

func (l loader) templateAppOf(ctx context.Context, name string) error {
	var list struct {
		Revisions []struct {
			Source     string `json:"source"`
			TemplateID string `json:"template_id"`
		} `json:"revisions"`
	}
	if err := l.get(ctx, "/api/v1/apps/"+esc(name)+"/compose/revisions", &list); err != nil {
		if IsNotFound(err) {
			return nil
		}
		return l.st.note("templateapp/"+name, fmt.Errorf("read compose history of app %s: %w", name, err))
	}
	if len(list.Revisions) == 0 {
		return nil
	}
	// The list is newest first, only the latest revision says what runs now.
	live := &liveTemplateApp{}
	if rv := list.Revisions[0]; rv.Source == "template" {
		live.latest = rv.TemplateID
	}
	l.st.templateApps[name] = live
	return nil
}

func (p *planner) planTemplateApp(r *Resource) {
	in := r.tmpl
	if p.opts.NoDeploy {
		p.failCode(r, CodeComposeNoDeployUnsupported, "TemplateApp %s deploys on install, so no_deploy cannot be honored; apply without it", r.Name)
		return
	}
	if _, clash := p.byKey[resourceKey(KindApp, "", r.Name)]; clash {
		p.failCode(r, CodeOwnershipConflict, "app %s is declared both as an App and as a TemplateApp; declare it once", r.Name)
		return
	}
	if p.deniedItem(r, "template/"+in.template) || p.deniedItem(r, "templateapp/"+r.Name) {
		return
	}
	if _, plain := p.st.apps[r.Name]; plain {
		p.failCode(r, CodeOwnershipConflict, "an app named %s already exists and was not installed from template %s", r.Name, in.template)
		return
	}
	if live := p.st.templateApps[r.Name]; live != nil {
		if live.latest != in.template {
			p.failCode(r, CodeOwnershipConflict, "app %s exists and was not installed from template %s; pick another name", r.Name, in.template)
			return
		}
		// An installed app keeps its pinned images, so install time inputs are not reapplied.
		now := templateInputsDigest(in)
		if applied, known := p.opts.ComposeEnvApplied[r.Name]; known && now != applied {
			p.failCode(r, CodeTemplateAppInputsChanged, "app %s was installed with other env or domains; they apply at install only, so remove the app and apply again to change them", r.Name)
			return
		}
		it := p.add(r, ActionNoop, nil, nil, nil)
		if now != "" {
			it.Warnings = append(it.Warnings, "already installed: env and domains are install time inputs and are not changed by a later apply")
		}
		return
	}
	t := p.st.templates[in.template]
	if t == nil || t.missing {
		p.failCode(r, CodeInvalidDocument, "template %q is not in the catalog of this control plane", in.template)
		return
	}
	if t.Manifest == nil {
		p.failCode(r, CodeInvalidDocument, "template %s has no install manifest, so it cannot be configured here; deploy it with 'templates deploy %s'", in.template, in.template)
		return
	}
	if msg := checkTemplateInputs(t, in); msg != "" {
		p.failCode(r, CodeInvalidDocument, "%s", msg)
		return
	}
	changes := []FieldChange{{Path: "template", Op: OpAdd, New: in.template}}
	if len(in.domains) > 0 {
		for _, svc := range sortedKeys(in.domains) {
			changes = append(changes, FieldChange{Path: "domains." + svc, Op: OpAdd, New: in.domains[svc]})
		}
	}
	if len(in.env) > 0 {
		changes = append(changes, FieldChange{Path: "env", Op: OpAdd, New: shortEnvDigest(composeEnvDigest(in.env))})
	}
	p.add(r, ActionCreate, changes, nil, func(ctx context.Context, x *executor) error {
		body := map[string]any{"name": r.Name}
		if len(in.env) > 0 {
			body["env"] = in.env
		}
		if len(in.domains) > 0 {
			body["domains"] = in.domains
		}
		return x.do(ctx, http.MethodPost, "/api/v1/service-templates/"+esc(in.template)+"/install", body, nil)
	})
}

// checkTemplateInputs compares the declared env and domains with the
// template's prompts, naming every problem in one message.
func checkTemplateInputs(t *wireTemplate, in *templateAppInputs) string {
	known := map[string]bool{}
	var missing []string
	for _, e := range t.Manifest.Env {
		known[e.Key] = true
		if e.Required && e.Default == "" && e.Generate == "" && in.env[e.Key] == "" {
			missing = append(missing, e.Key)
		}
	}
	var unknown []string
	for _, k := range sortedKeys(in.env) {
		if !known[k] {
			unknown = append(unknown, k)
		}
	}
	offered := map[string]bool{}
	for _, d := range t.Manifest.Domains {
		offered[d.Service] = true
	}
	var badDomains []string
	for _, k := range sortedKeys(in.domains) {
		if !offered[k] {
			badDomains = append(badDomains, k)
		}
	}
	sort.Strings(missing)
	var parts []string
	if len(unknown) > 0 {
		parts = append(parts, "template "+in.template+" has no env setting "+strings.Join(unknown, ", "))
	}
	if len(missing) > 0 {
		parts = append(parts, "template "+in.template+" needs a value for env: "+strings.Join(missing, ", "))
	}
	if len(badDomains) > 0 {
		parts = append(parts, "template "+in.template+" does not offer a domain for: "+strings.Join(badDomains, ", "))
	}
	return strings.Join(parts, "; ")
}
