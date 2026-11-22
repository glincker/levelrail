package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// The link lives inside the .levelrail directory the repo pointer already uses.
const appLinkFile = ".levelrail/app"

var linkedAppNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// linkedAppName finds the app for the current directory: a .levelrail/app link
// in this or any parent directory first, then the name in ./app.yaml.
func linkedAppName() (name, source string) {
	dir, err := os.Getwd()
	if err != nil {
		return "", ""
	}
	for d := dir; ; d = filepath.Dir(d) {
		if b, err := os.ReadFile(filepath.Join(d, filepath.FromSlash(appLinkFile))); err == nil { //nolint:gosec // fixed relative name under a parent directory
			if n := strings.TrimSpace(string(b)); linkedAppNameRE.MatchString(n) {
				return n, filepath.Join(d, filepath.FromSlash(appLinkFile))
			}
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	if n := appNameFromSpec(filepath.Join(dir, "app.yaml")); n != "" {
		return n, "app.yaml"
	}
	return "", ""
}

// appNameFromSpec reads a top-level name, or the only service key, from an app.yaml.
func appNameFromSpec(path string) string {
	b, err := os.ReadFile(path) //nolint:gosec // fixed file name in the working directory
	if err != nil {
		return ""
	}
	var doc struct {
		Name     string               `yaml:"name"`
		Services map[string]yaml.Node `yaml:"services"`
	}
	if yaml.Unmarshal(b, &doc) != nil {
		return ""
	}
	if linkedAppNameRE.MatchString(doc.Name) {
		return doc.Name
	}
	if len(doc.Services) == 1 {
		for k := range doc.Services {
			if linkedAppNameRE.MatchString(k) {
				return k
			}
		}
	}
	return ""
}

// wantsAppArg is true for the argument labels that name an app.
func wantsAppArg(argLabel string) bool { return argLabel == "app name" || argLabel == "app" }

func linkUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s link <app>        remember <app> for this directory
  %[1]s link              show the app this directory resolves to
  %[1]s link --remove     forget the link

Writes .levelrail/app. Inside that directory or any below it, deploy, logs,
env and every "apps ..." verb that takes an app name use it when none is
given. Without a link, the name in ./app.yaml is used. The file holds a name
only, never a token.
`, prog)
}

func runLink(prog string, args []string, stdout, stderr io.Writer, _ func(string) (string, bool)) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		_, _ = fmt.Fprint(stdout, linkUsage(prog))
		return exitOK
	}
	switch {
	case len(args) == 0:
		name, src := linkedAppName()
		if name == "" {
			_, _ = fmt.Fprintf(stderr, "%s: no app linked here, run \"%s link <app>\"\n", prog, prog)
			return exitUsage
		}
		_, _ = fmt.Fprintf(stdout, "%s (from %s)\n", name, src)
		return exitOK
	case len(args) == 1 && args[0] == "--remove":
		if err := os.Remove(filepath.FromSlash(appLinkFile)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return reportError(stdout, stderr, false, err)
		}
		_, _ = fmt.Fprintln(stdout, "unlinked")
		return exitOK
	case len(args) == 1 && !strings.HasPrefix(args[0], "-"):
		if !linkedAppNameRE.MatchString(args[0]) {
			return reportError(stdout, stderr, false, newValidationError("%q is not a valid app name", args[0]))
		}
		if err := os.MkdirAll(filepath.Dir(filepath.FromSlash(appLinkFile)), 0o750); err != nil {
			return reportError(stdout, stderr, false, err)
		}
		if err := os.WriteFile(filepath.FromSlash(appLinkFile), []byte(args[0]+"\n"), 0o644); err != nil { //nolint:gosec // holds an app name only, meant to be committed
			return reportError(stdout, stderr, false, err)
		}
		_, _ = fmt.Fprintf(stdout, "linked this directory to %s (%s)\n", args[0], appLinkFile)
		return exitOK
	default:
		_, _ = fmt.Fprint(stderr, linkUsage(prog))
		return exitUsage
	}
}
