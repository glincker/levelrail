package iac

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/GLINCKER/levelrail/internal/loadbalancer"
	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// Options carry apply-time inputs that never live in a resource file.
type Options struct {
	// Vars resolve ${{ env.NAME }} placeholders.
	Vars map[string]string
	// Source names this apply source; apps it creates carry a managed-by
	// tag and only those are ever pruned.
	Source string
	// Secrets maps "NAME" or "app/NAME" to a value to store at apply time.
	// Values never appear in a plan, result or export.
	Secrets map[string]string
	// Project limits prune to managed apps of this project, matching the
	// project filter applied to the files.
	Project string
	// Prune deletes managed resources absent from the files.
	Prune bool
	// NoDeploy skips the restart that applies an env change to a running app.
	NoDeploy bool
	// ContinueOnError keeps applying after an item fails.
	ContinueOnError bool
}

var (
	appNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	envKeyRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type issueSink struct {
	d      *Document
	issues []Issue
}

func (s *issueSink) at(path, format string, args ...any) {
	s.issues = append(s.issues, Issue{File: s.d.File, Path: path, Line: lineFor(s.d.Root, strings.Split(path, ".")), Message: fmt.Sprintf(format, args...)})
}

// Build validates every document (schema first, then semantics) and turns
// them into canonical resources. It returns no resources when any issue
// exists, so nothing is planned from a partly valid input.
func Build(docs []*Document, opts Options) ([]*Resource, []Issue) {
	var issues []Issue
	for _, d := range docs {
		if d.Root == nil || d.Root.Kind != yaml.MappingNode {
			continue
		}
		si, err := schemaIssues(d)
		if err != nil {
			issues = append(issues, Issue{File: d.File, Line: d.Line, Message: err.Error()})
			continue
		}
		issues = append(issues, si...)
	}
	if len(issues) > 0 {
		return nil, sortIssues(issues)
	}

	var out []*Resource
	for _, d := range docs {
		sink := &issueSink{d: d}
		r := buildOne(d, opts, sink)
		checkPlaceholders(r, sink)
		if r != nil && len(sink.issues) == 0 {
			out = append(out, r)
		}
		issues = append(issues, sink.issues...)
	}
	issues = append(issues, crossChecks(out)...)
	if len(issues) > 0 {
		return nil, sortIssues(issues)
	}
	return out, nil
}

// checkPlaceholders rejects a ${{ ... }} left in a rendered field, so a typo or
// an unsupported spot never reaches a live app as a literal string.
func checkPlaceholders(r *Resource, s *issueSink) {
	if r == nil || r.Kind == KindPipeline {
		return
	}
	keys := make([]string, 0, len(r.Fields))
	for k := range r.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if m := findPlaceholder(r.Fields[k]); m != "" {
			s.at("spec", "field %q still holds the unresolved placeholder %q; ${{ env.NAME }} is replaced from a var flag and ${{ secrets.NAME }} only works as a whole env var value", k, m)
		}
	}
}

var anyPlaceRe = regexp.MustCompile(`\$\{\{.*?\}\}`)

func findPlaceholder(v any) string {
	switch t := v.(type) {
	case string:
		return anyPlaceRe.FindString(t)
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if m := findPlaceholder(t[k]); m != "" {
				return m
			}
		}
	case map[string]string:
		for _, e := range t {
			if m := anyPlaceRe.FindString(e); m != "" {
				return m
			}
		}
	case []any:
		for _, e := range t {
			if m := findPlaceholder(e); m != "" {
				return m
			}
		}
	}
	return ""
}

func sortIssues(in []Issue) []Issue {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].File != in[j].File {
			return in[i].File < in[j].File
		}
		return in[i].Line < in[j].Line
	})
	return in
}

func buildOne(d *Document, opts Options, s *issueSink) *Resource {
	r := &Resource{Kind: d.Kind, Name: d.Name, Doc: d}
	switch d.Kind {
	case KindProject:
		r.Fields = map[string]any{}
		if env := specEnv(d, s, opts); len(env) > 0 {
			r.Fields["env"] = env
		}
	case KindEnvironment:
		var sp struct {
			Project   string `yaml:"project"`
			Protected bool   `yaml:"protected"`
		}
		decodeSpec(d, &sp, s)
		r.Scope = sp.Project
		r.Fields = map[string]any{}
		if sp.Protected {
			r.Fields["protected"] = true
		}
		if env := specEnv(d, s, opts); len(env) > 0 {
			r.Fields["env"] = env
		}
	case KindTag:
		if len(d.Name) > 64 {
			s.at("metadata.name", "tag names are at most 64 characters")
		}
		r.Fields = map[string]any{}
	case KindDatabase:
		var sp struct {
			Engine  string `yaml:"engine"`
			Version string `yaml:"version"`
			Project string `yaml:"project"`
		}
		decodeSpec(d, &sp, s)
		if !appNameRe.MatchString(d.Name) {
			s.at("metadata.name", "database names are lowercase letters, digits and hyphens, starting with a letter")
		}
		r.Fields = map[string]any{"engine": sp.Engine}
		putIf(r.Fields, "version", sp.Version)
		putIf(r.Fields, "project", sp.Project)
	case KindApp:
		return buildApp(d, opts, s)
	case KindDomain:
		var sp struct {
			App string `yaml:"app"`
		}
		decodeSpec(d, &sp, s)
		r.Fields = map[string]any{"app": sp.App}
	case KindLoadBalancer:
		var cfg loadbalancer.Config
		decodeSpec(d, &cfg, s)
		if err := cfg.Validate(); err != nil {
			s.at("spec", "%v", err)
		}
		r.Fields = toMap(cfg.Defaults())
	case KindPipeline:
		return buildPipeline(d, s)
	case KindAlertRule:
		return buildAlert(d, s)
	}
	return r
}

func putIf(m map[string]any, k, v string) {
	if v != "" {
		m[k] = v
	}
}

func decodeSpec(d *Document, out any, s *issueSink) {
	if d.SpecNode == nil {
		return
	}
	if err := d.SpecNode.Decode(out); err != nil {
		s.at("spec", "%v", err)
	}
}

func specEnv(d *Document, s *issueSink, opts Options) map[string]string {
	envNode := mapValue(d.SpecNode, "env")
	if envNode == nil {
		return nil
	}
	out := map[string]string{}
	for i := 0; i+1 < len(envNode.Content); i += 2 {
		k, vn := envNode.Content[i].Value, envNode.Content[i+1]
		v, ok := resolveLiteral(k, vn.Value, "spec.env."+k, s, opts)
		if ok {
			out[k] = v
		}
	}
	return out
}

// resolveLiteral substitutes variables and refuses literals that look like
// secrets. Values that arrive through a variable are the caller's call.
func resolveLiteral(key, raw, path string, s *issueSink, opts Options) (string, bool) {
	if secretPlaceRe.MatchString(raw) {
		s.at(path, "a secret reference is only valid on an app env var, not here")
		return "", false
	}
	val, missing := substituteVars(raw, opts.Vars)
	for _, m := range missing {
		s.at(path, "variable %s is not provided (pass it with a var flag)", m)
	}
	if len(missing) > 0 {
		return "", false
	}
	if val == raw && looksSecret(key, raw) {
		s.at(path, "%s looks like a secret; documents never hold secret values, use a secretRef and set the value with the secret flag or the secrets API", key)
		return "", false
	}
	return val, true
}

func crossChecks(rs []*Resource) []Issue {
	seen := map[string]*Resource{}
	var issues []Issue
	domainOwner := map[string]string{}
	for _, r := range rs {
		if prev, dup := seen[r.Key()]; dup {
			issues = append(issues, Issue{File: r.Doc.File, Line: r.Doc.Line, Message: fmt.Sprintf("%s is declared twice (first at %s:%d)", r.Key(), prev.Doc.File, prev.Doc.Line)})
			continue
		}
		seen[r.Key()] = r
		hosts := stringList(r.Fields["domains"])
		if r.Kind == KindDomain {
			hosts = []string{r.Name}
		}
		for _, h := range hosts {
			app, _ := r.Fields["app"].(string)
			if r.Kind == KindApp {
				app = r.Name
			}
			if prev, ok := domainOwner[h]; ok && prev != app {
				issues = append(issues, Issue{File: r.Doc.File, Line: r.Doc.Line, Message: fmt.Sprintf("domain %s is claimed by both %s and %s", h, prev, app)})
			}
			domainOwner[h] = app
		}
	}
	return issues
}

func buildPipeline(d *Document, s *issueSink) *Resource {
	var sp struct {
		App     string `yaml:"app"`
		YAML    string `yaml:"yaml"`
		Enabled *bool  `yaml:"enabled"`
	}
	decodeSpec(d, &sp, s)
	_, pissues := pipeline.Validate([]byte(sp.YAML))
	base := 0
	if n := mapValue(d.SpecNode, "yaml"); n != nil {
		base = n.Line
	}
	for _, pi := range pissues {
		s.issues = append(s.issues, Issue{File: d.File, Path: "spec.yaml", Line: base + pi.Line, Message: pi.Message})
	}
	enabled := true
	if sp.Enabled != nil {
		enabled = *sp.Enabled
	}
	return &Resource{Kind: KindPipeline, Name: d.Name, Scope: sp.App, Doc: d,
		Fields: map[string]any{"yaml": strings.TrimRight(sp.YAML, "\n"), "enabled": enabled}}
}

func buildAlert(d *Document, s *issueSink) *Resource {
	var sp struct {
		App                   string   `yaml:"app"`
		Type                  string   `yaml:"type"`
		Metric                string   `yaml:"metric"`
		Comparator            string   `yaml:"comparator"`
		Threshold             *float64 `yaml:"threshold"`
		For                   string   `yaml:"for"`
		RestartCountThreshold int      `yaml:"restartCountThreshold"`
		RestartWindow         string   `yaml:"restartWindow"`
		Channel               string   `yaml:"channel"`
		Enabled               *bool    `yaml:"enabled"`
	}
	decodeSpec(d, &sp, s)
	f := map[string]any{"type": sp.Type, "enabled": sp.Enabled == nil || *sp.Enabled}
	putIf(f, "metric", sp.Metric)
	putIf(f, "comparator", sp.Comparator)
	putIf(f, "channel", sp.Channel)
	for k, v := range map[string]string{"for": sp.For, "restartWindow": sp.RestartWindow} {
		n, err := normDuration(v)
		if err != nil {
			s.at("spec."+k, "%v", err)
		}
		putIf(f, k, n)
	}
	if sp.Threshold != nil && *sp.Threshold != 0 {
		f["threshold"] = *sp.Threshold
	}
	if sp.RestartCountThreshold > 0 {
		f["restartCountThreshold"] = sp.RestartCountThreshold
	}
	return &Resource{Kind: KindAlertRule, Name: d.Name, Scope: sp.App, Doc: d, Fields: f}
}

func buildApp(d *Document, opts Options, s *issueSink) *Resource {
	if !appNameRe.MatchString(d.Name) {
		s.at("metadata.name", "app names are lowercase letters, digits and hyphens, starting with a letter")
	}
	var sp struct {
		Project     string   `yaml:"project"`
		Environment string   `yaml:"environment"`
		Tags        []string `yaml:"tags"`
	}
	decodeSpec(d, &sp, s)
	svcNode := mapValue(d.SpecNode, "service")
	env, secrets, conns := appEnv(svcNode, s, opts)

	trimmed := *svcNode
	trimmed.Content = nil
	for i := 0; i+1 < len(svcNode.Content); i += 2 {
		if svcNode.Content[i].Value != "env" {
			trimmed.Content = append(trimmed.Content, svcNode.Content[i], svcNode.Content[i+1])
		}
	}
	substituteImage(&trimmed, s, opts)
	var svc spec.Service
	if err := trimmed.Decode(&svc); err != nil {
		s.at("spec.service", "%v", err)
		return nil
	}
	checkSupportedService(&svc, s)
	if len(s.issues) > 0 {
		return nil
	}
	if err := (&spec.Spec{Version: 1, Services: map[string]spec.Service{d.Name: svc}}).Validate(); err != nil {
		s.at("spec.service", "%s", strings.TrimPrefix(err.Error(), "spec: "))
		return nil
	}

	f := appFields{Image: svc.Build.Image, Port: svc.Port, HostPort: svc.HostPort, BindAddress: svc.EffectiveBindAddress(),
		Replicas: svc.EffectiveReplicas(), Strategy: svc.EffectiveStrategy(), Domains: sortedUnique(svc.Domains), Env: nonEmptyMap(env),
		SecretEnv: sortedUnique(secrets), Labels: nonEmptyMap(svc.Labels), Command: svc.Command, Project: sp.Project, Environment: sp.Environment,
		Tags: sortedUnique(sp.Tags), Connections: nonEmptyMap(conns)}
	if opts.Source != "" {
		f.Tags = sortedUnique(append(f.Tags, tagManagedPre+opts.Source))
	}
	var err error
	f.Resources = resourcesFromSpec(svc.Resources)
	if f.Health, err = healthFromSpec(svc.Health); err != nil {
		s.at("spec.service.health", "%v", err)
	}
	if svc.Hooks != nil && (svc.Hooks.PreDeploy != "" || svc.Hooks.PostDeploy != "") {
		f.Hooks = &hookFields{PreDeploy: svc.Hooks.PreDeploy, PostDeploy: svc.Hooks.PostDeploy}
	}
	if f.Environment != "" && f.Project == "" {
		s.at("spec.environment", "an environment needs a project")
	}
	f.Jobs = jobsFromSpec(svc.Jobs, s)
	for _, v := range svc.Volumes {
		if v.Name != "" {
			if f.Volumes == nil {
				f.Volumes = map[string]string{}
			}
			f.Volumes[v.Name] = v.Path
		}
	}
	return &Resource{Kind: KindApp, Name: d.Name, Doc: d, Fields: toMap(f), SecretRefs: f.SecretEnv}
}

// substituteImage resolves ${{ env.NAME }} in build.image like env values.
func substituteImage(svc *yaml.Node, s *issueSink, opts Options) {
	img := mapValue(mapValue(svc, "build"), "image")
	if img == nil || img.Kind != yaml.ScalarNode {
		return
	}
	val, missing := substituteVars(img.Value, opts.Vars)
	for _, m := range missing {
		s.at("spec.service.build.image", "variable %s is not provided (pass it with a var flag)", m)
	}
	if len(missing) == 0 {
		img.Value = val
	}
}

func checkSupportedService(svc *spec.Service, s *issueSink) {
	const p = "spec.service."
	if svc.Build.Type != spec.BuildImage {
		s.at(p+"build.type", "apply manages apps that run a prebuilt image; use build.type image (git builds are configured with a git source)")
	}
	for i, v := range svc.Volumes {
		if v.Name == "" {
			s.at(fmt.Sprintf("%svolumes[%d]", p, i), "apply supports named volumes only; host paths are never declared in the files")
		}
	}
	if svc.Egress != nil {
		s.at(p+"egress", "egress policies are not supported by apply yet")
	}
	if svc.LoadBalancer != nil {
		s.at(p+"loadbalancer", "declare a LoadBalancer document instead")
	}
	if svc.Resources != nil && svc.Resources.GPU != nil {
		s.at(p+"resources.gpu", "GPU requests are not supported by apply yet")
	}
	if svc.Build.RegistryCredential != "" {
		s.at(p+"build.registryCredential", "registry credentials are not supported by apply yet")
	}
	if svc.Restart != "" {
		s.at(p+"restart", "restart is not supported by apply yet; set it in app.yaml or the compose file")
	}
}

func appEnv(svcNode *yaml.Node, s *issueSink, opts Options) (map[string]string, []string, map[string]string) {
	envNode := mapValue(svcNode, "env")
	if envNode == nil {
		return nil, nil, nil
	}
	literal := map[string]string{}
	conns := map[string]string{}
	var secrets []string
	for i := 0; i+1 < len(envNode.Content); i += 2 {
		k, vn := envNode.Content[i].Value, envNode.Content[i+1]
		path := "spec.service.env." + k
		if !envKeyRe.MatchString(k) {
			s.at(path, "env names are letters, digits and underscores, not starting with a digit")
			continue
		}
		switch vn.Kind {
		case yaml.ScalarNode:
			if m := secretPlaceRe.FindStringSubmatch(vn.Value); m != nil {
				secrets = appendSecret(secrets, k, m[1], path, s)
				continue
			}
			if v, ok := resolveLiteral(k, vn.Value, path, s, opts); ok {
				literal[k] = v
			}
		case yaml.MappingNode:
			if from := mapValue(vn, "from"); from != nil {
				if ref, ok := parseFromRef(from, path+".from", s); ok {
					conns[k] = ref
				}
				continue
			}
			secrets = mappingEnv(k, vn, path, secrets, s)
		}
	}
	return literal, secrets, conns
}

func mappingEnv(k string, vn *yaml.Node, path string, secrets []string, s *issueSink) []string {
	if ref := mapValue(vn, "secretRef"); ref != nil {
		return appendSecret(secrets, k, ref.Value, path, s)
	}
	if n := mapValue(vn, "secret"); n != nil && n.Value == "true" {
		return append(secrets, k)
	}
	if mapValue(vn, "vault") != nil {
		s.at(path, "vault references are not supported by apply yet")
	}
	return secrets
}

func appendSecret(secrets []string, key, ref, path string, s *issueSink) []string {
	if ref != key {
		s.at(path, "secretRef %q must match the env var name %q: secrets are stored per app under the env var name", ref, key)
		return secrets
	}
	return append(secrets, key)
}
