package spec

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// EnvStaticBuildCommandMax caps the byte length of a per-site build command override.
const EnvStaticBuildCommandMax = "APP_STATIC_BUILD_COMMAND_MAX_BYTES"

const defaultStaticBuildCommandMax = 1024

// StaticBuildCommandMaxFromEnv returns the build command length cap.
func StaticBuildCommandMaxFromEnv(lookup func(string) (string, bool)) int {
	if v, ok := lookup(EnvStaticBuildCommandMax); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	}
	return defaultStaticBuildCommandMax
}

// ValidateStaticOverrides checks a per-site build command and output directory;
// either may be empty. The message is user facing.
func ValidateStaticOverrides(command, outputDir string, maxCommand int) error {
	if len(command) > maxCommand || strings.ContainsAny(command, "\r\n\x00") {
		return fmt.Errorf("build_command must be one line of at most %d bytes", maxCommand)
	}
	if outputDir != "" && (len(outputDir) > 255 || !filepath.IsLocal(outputDir) || strings.ContainsRune(outputDir, 0)) {
		return fmt.Errorf("output_dir must be a relative path inside the repository")
	}
	return nil
}

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
