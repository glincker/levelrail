package spec

import (
	"fmt"
	"path"
	"strings"
)

// Deploy context names. Production is the app's own settings and has no stored row.
const (
	ContextProduction    = "production"
	ContextDeployPreview = "deploy-preview"
	ContextBranchDeploy  = "branch-deploy"
	ContextBranchPrefix  = "branch:"
)

// ContextOverrides is the settings one context can override. Empty fields inherit.
type ContextOverrides struct {
	Env          map[string]string
	BuildCommand string
	Publish      string
	BuildArgs    map[string]string
}

// Empty reports whether c overrides nothing.
func (c ContextOverrides) Empty() bool {
	return len(c.Env) == 0 && c.BuildCommand == "" && c.Publish == "" && len(c.BuildArgs) == 0
}

// ContextBranchPattern returns the pattern of a "branch:<pattern>" context.
func ContextBranchPattern(name string) (string, bool) {
	p, ok := strings.CutPrefix(name, ContextBranchPrefix)
	return p, ok && p != ""
}

// ValidateContextName checks that name is a settable context and its pattern is valid.
func ValidateContextName(name string) error {
	if name == ContextProduction {
		return fmt.Errorf("context %q uses the app's own settings, set them on the app itself", ContextProduction)
	}
	if name == ContextDeployPreview || name == ContextBranchDeploy {
		return nil
	}
	p, ok := ContextBranchPattern(name)
	if !ok {
		return fmt.Errorf("unknown context %q: use %s, %s or %s<pattern>", name, ContextDeployPreview, ContextBranchDeploy, ContextBranchPrefix)
	}
	if _, err := path.Match(p, "x"); err != nil {
		return fmt.Errorf("context %q: invalid branch pattern: %w", name, err)
	}
	return nil
}

// ContextConfig overrides env and build settings for one deploy context.
type ContextConfig struct {
	Env   map[string]string `yaml:"env,omitempty"`
	Build ContextBuild      `yaml:"build,omitempty"`
}

// ContextBuild is the build subset a context may override.
type ContextBuild struct {
	Command string            `yaml:"command,omitempty"`
	Publish string            `yaml:"publish,omitempty"`
	Args    map[string]string `yaml:"args,omitempty"`
}

// DeployConfig converts c to the resolver's form.
func (c ContextConfig) DeployConfig() ContextOverrides {
	return ContextOverrides{Env: c.Env, BuildCommand: c.Build.Command, Publish: c.Build.Publish, BuildArgs: c.Build.Args}
}

// ValidateContextConfig checks one context's overrides against the app's build type.
func ValidateContextConfig(label, buildType string, c ContextOverrides) error {
	for k := range c.Env {
		if k == "" || strings.ContainsAny(k, "= \t\r\n\x00") {
			return fmt.Errorf("%s: env name %q is not valid", label, k)
		}
	}
	for k := range c.BuildArgs {
		if !buildArgNameRe.MatchString(k) {
			return fmt.Errorf("%s: build.args name %q is not a valid variable name", label, k)
		}
	}
	if c.BuildCommand == "" && c.Publish == "" {
		return nil
	}
	if buildType != BuildStatic {
		return fmt.Errorf("%s: build.command and build.publish are only meaningful for build.type: static", label)
	}
	if strings.ContainsAny(c.BuildCommand, "\r\n\x00") {
		return fmt.Errorf("%s: build.command must be a single line", label)
	}
	if c.Publish != "" {
		pub, err := NormalizeRepoPath("build.publish", c.Publish)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		if pub == "" {
			return fmt.Errorf("%s: build.publish must name a subdirectory, not the project root", label)
		}
	}
	return nil
}

func (svc Service) validateContexts(name string) error {
	for ctxName, c := range svc.Contexts {
		if err := ValidateContextName(ctxName); err != nil {
			return fmt.Errorf("spec: service %q: contexts: %w", name, err)
		}
		if err := ValidateContextConfig(fmt.Sprintf("spec: service %q: contexts.%s", name, ctxName), svc.Build.Type, c.DeployConfig()); err != nil {
			return err
		}
	}
	return nil
}
