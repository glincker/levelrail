package spec

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/deploycontext"
)

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
func (c ContextConfig) DeployConfig() deploycontext.Config {
	return deploycontext.Config{Env: c.Env, BuildCommand: c.Build.Command, Publish: c.Build.Publish, BuildArgs: c.Build.Args}
}

// ValidateContextConfig checks one context's overrides against the app's build type.
func ValidateContextConfig(label, buildType string, c deploycontext.Config) error {
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
		if err := deploycontext.Validate(ctxName); err != nil {
			return fmt.Errorf("spec: service %q: contexts: %w", name, err)
		}
		if err := ValidateContextConfig(fmt.Sprintf("spec: service %q: contexts.%s", name, ctxName), svc.Build.Type, c.DeployConfig()); err != nil {
			return err
		}
	}
	return nil
}
