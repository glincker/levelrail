package spec

import (
	"fmt"
	"regexp"
	"strings"
)

var buildArgNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// RunsStaticBuild reports whether the build compiles a static site in a
// build container, as opposed to serving committed files as-is.
func (b Build) RunsStaticBuild() bool {
	return b.Type == BuildStatic && b.Command != ""
}

// ValidateStaticBuild checks command and publish for a build of buildType.
func ValidateStaticBuild(name, buildType, command, publish, path string, args map[string]string) error {
	return Build{Type: buildType, Command: command, Publish: publish, Path: path, Args: args}.validateStaticBuild(name)
}

func (b Build) validateStaticBuild(name string) error {
	if b.Type != BuildStatic {
		if b.Command != "" || b.Publish != "" {
			return fmt.Errorf("spec: service %q: build.command and build.publish are only meaningful for build.type: static", name)
		}
		return nil
	}
	if b.Command == "" {
		if b.Publish != "" {
			return fmt.Errorf("spec: service %q: build.publish needs build.command, or use build.path to serve committed files", name)
		}
		return nil
	}
	if b.Publish == "" {
		return fmt.Errorf("spec: service %q: build.publish is required with build.command", name)
	}
	if b.Path != "" {
		return fmt.Errorf("spec: service %q: build.path and build.command are mutually exclusive, the publish directory replaces path", name)
	}
	if strings.ContainsAny(b.Command, "\r\n\x00") {
		return fmt.Errorf("spec: service %q: build.command must be a single line", name)
	}
	pub, err := NormalizeRepoPath("build.publish", b.Publish)
	if err != nil {
		return fmt.Errorf("spec: service %q: %w", name, err)
	}
	if pub == "" {
		return fmt.Errorf("spec: service %q: build.publish must name a subdirectory, not the project root", name)
	}
	for k := range b.Args {
		if !buildArgNameRe.MatchString(k) {
			return fmt.Errorf("spec: service %q: build.args name %q is not a valid variable name", name, k)
		}
	}
	return nil
}
