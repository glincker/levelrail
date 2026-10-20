// Package deploycontext holds the pure rules for deploy contexts: which
// context a deploy runs in and how env and build settings resolve across
// the branch override, context and app default layers.
package deploycontext

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Context names. Production is the app's own settings and has no stored row.
const (
	Production    = "production"
	DeployPreview = "deploy-preview"
	BranchDeploy  = "branch-deploy"
	branchPrefix  = "branch:"
)

// Sources reported for each resolved value, lowest to highest precedence.
const (
	SourceApp            = "app"
	SourceLegacyPreview  = "preview-env"
	SourceContext        = "context"
	SourceBranchContext  = "branch-context"
	SourceBranchOverride = "branch-env"
)

// MaskedValue replaces a secret's value in every effective view.
const MaskedValue = "********"

// BranchContext names the per-branch context for a branch pattern.
func BranchContext(pattern string) string { return branchPrefix + pattern }

// BranchPattern returns the pattern of a "branch:<pattern>" context.
func BranchPattern(name string) (string, bool) {
	p, ok := strings.CutPrefix(name, branchPrefix)
	return p, ok && p != ""
}

// Validate checks that name is a settable context and its pattern is valid.
func Validate(name string) error {
	if name == Production {
		return fmt.Errorf("context %q uses the app's own settings, set them on the app itself", Production)
	}
	if name == DeployPreview || name == BranchDeploy {
		return nil
	}
	p, ok := BranchPattern(name)
	if !ok {
		return fmt.Errorf("unknown context %q: use %s, %s or %s<pattern>", name, DeployPreview, BranchDeploy, branchPrefix)
	}
	if _, err := path.Match(p, "x"); err != nil {
		return fmt.Errorf("context %q: invalid branch pattern: %w", name, err)
	}
	return nil
}

// For picks the context a deploy runs in: a pull request preview is
// deploy-preview, any other non-production deploy is branch-deploy.
func For(prNumber int, branch, productionBranch string) string {
	switch {
	case prNumber > 0:
		return DeployPreview
	case branch != "" && branch != productionBranch:
		return BranchDeploy
	}
	return Production
}

// Config is the settings one context can override. Empty fields inherit.
type Config struct {
	Env          map[string]string
	BuildCommand string
	Publish      string
	BuildArgs    map[string]string
}

// Empty reports whether c overrides nothing.
func (c Config) Empty() bool {
	return len(c.Env) == 0 && c.BuildCommand == "" && c.Publish == "" && len(c.BuildArgs) == 0
}

// EnvValue is one env var as the resolver sees it.
type EnvValue struct {
	Value  string
	Secret bool
}

// BranchOverride is one branch-scoped env override (secrets carry no value).
type BranchOverride struct {
	Pattern string
	Key     string
	Value   string
	Secret  bool
}

// Inputs are every layer the resolver reads for one deploy.
type Inputs struct {
	Context       string
	Branch        string
	AppEnv        map[string]EnvValue
	LegacyPreview map[string]string
	Contexts      map[string]Config
	BranchEnv     []BranchOverride
	AppBuild      Config
}

// Entry is one resolved env var with where it came from.
type Entry struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
	Source string `json:"source"`
}

// Setting is one resolved build setting with where it came from.
type Setting struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

// Effective is the resolved config for one deploy. Secret values are masked.
type Effective struct {
	Context      string             `json:"context"`
	Env          []Entry            `json:"env"`
	BuildCommand Setting            `json:"build_command"`
	Publish      Setting            `json:"publish_dir"`
	BuildArgs    map[string]Setting `json:"build_args,omitempty"`
}

// Match reports whether branch matches a shell-glob pattern.
func Match(branch, pattern string) bool {
	ok, err := path.Match(pattern, branch)
	return err == nil && ok
}

// matchingBranchContexts returns the "branch:" context names matching
// branch, least specific first: globs by pattern, then an exact match last.
func matchingBranchContexts(branch string, contexts map[string]Config) []string {
	var names []string
	for name := range contexts {
		if p, ok := BranchPattern(name); ok && Match(branch, p) {
			names = append(names, name)
		}
	}
	sort.Slice(names, func(i, j int) bool {
		pi, _ := BranchPattern(names[i])
		pj, _ := BranchPattern(names[j])
		ei, ej := pi == branch, pj == branch
		if ei != ej {
			return ej
		}
		return pi < pj
	})
	return names
}

// matchingBranchEnv picks, per key, the winning branch override: an exact
// pattern beats a glob, and between globs the lexicographically last wins.
func matchingBranchEnv(branch string, overrides []BranchOverride) map[string]BranchOverride {
	out := map[string]BranchOverride{}
	for _, o := range overrides {
		if !Match(branch, o.Pattern) {
			continue
		}
		cur, have := out[o.Key]
		switch {
		case !have:
		case cur.Pattern == branch && o.Pattern != branch:
			continue
		case cur.Pattern != branch && o.Pattern != branch && o.Pattern < cur.Pattern:
			continue
		}
		out[o.Key] = o
	}
	return out
}

// Layer is one context config tagged with its source label.
type Layer struct {
	Source string
	Config Config
}

// Layers returns the context configs that apply to a deploy, lowest
// precedence first: the context itself, then matching branch contexts.
func Layers(contextName, branch string, contexts map[string]Config) []Layer {
	var out []Layer
	if c, ok := contexts[contextName]; ok {
		out = append(out, Layer{SourceContext, c})
	}
	for _, name := range matchingBranchContexts(branch, contexts) {
		out = append(out, Layer{SourceBranchContext, contexts[name]})
	}
	return out
}

// Resolve applies branch override, then context, then app default.
func Resolve(in Inputs) Effective {
	env := map[string]Entry{}
	put := func(k, v string, secret bool, source string) {
		if secret {
			v = MaskedValue
		}
		env[k] = Entry{Key: k, Value: v, Secret: secret, Source: source}
	}
	for k, v := range in.AppEnv {
		put(k, v.Value, v.Secret, SourceApp)
	}
	setCmd := Setting{Value: in.AppBuild.BuildCommand, Source: SourceApp}
	setPub := Setting{Value: in.AppBuild.Publish, Source: SourceApp}
	args := map[string]Setting{}
	for k, v := range in.AppBuild.BuildArgs {
		args[k] = Setting{Value: v, Source: SourceApp}
	}
	layer := func(c Config, source string) {
		for k, v := range c.Env {
			put(k, v, false, source)
		}
		if c.BuildCommand != "" {
			setCmd = Setting{Value: c.BuildCommand, Source: source}
		}
		if c.Publish != "" {
			setPub = Setting{Value: c.Publish, Source: source}
		}
		for k, v := range c.BuildArgs {
			args[k] = Setting{Value: v, Source: source}
		}
	}

	if in.Context != Production {
		for k, v := range in.LegacyPreview {
			put(k, v, false, SourceLegacyPreview)
		}
		for _, l := range Layers(in.Context, in.Branch, in.Contexts) {
			layer(l.Config, l.Source)
		}
		for k, o := range matchingBranchEnv(in.Branch, in.BranchEnv) {
			put(k, o.Value, o.Secret, SourceBranchOverride)
		}
	}

	eff := Effective{Context: in.Context, BuildCommand: setCmd, Publish: setPub}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		eff.Env = append(eff.Env, env[k])
	}
	if len(args) > 0 {
		eff.BuildArgs = args
	}
	return eff
}
