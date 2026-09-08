package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/GLINCKER/levelrail/internal/spec"
	"gopkg.in/yaml.v3"
)

// appSpecNotTrackedFields lists spec.Service fields GET
// /api/v1/apps/{name}/spec (internal/api/app_spec_export.go, reusing
// specServiceFromDesired) never populates, so diffAppSpecServices below
// never compares them: reporting a manufactured "difference" for a field
// the deployed side can never carry any value for would make "apps diff"
// noisy on essentially every real app.yaml instead of useful.
var appSpecNotTrackedFields = []string{"build", "hostPort", "replicas", "strategy", "volumes", "hooks"}

// appSpecDiffResult is "apps diff"'s --json/--output json shape.
type appSpecDiffResult struct {
	App           string   `json:"app"`
	File          string   `json:"file"`
	Differences   []string `json:"differences"`
	NotComparable []string `json:"not_comparable,omitempty"`
	NotTracked    []string `json:"not_tracked"`
}

// runAppsDiff implements "apps diff <name> <file>": parses file with
// internal/spec's own Parse, fetches name's deployed spec via the same
// GET /api/v1/apps/{name}/spec endpoint "apps export" uses
// (client.ExportAppSpec), and diffs the one service in file whose key
// matches name (or file's only service, if it declares exactly one)
// against the deployed one. Exits exitDiffFound when a real difference
// is found, so a CI pipeline can fail a "drifted" check without parsing
// stdout.
func runAppsDiff(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps diff", "print the diff result as JSON to stdout and nothing else", stderr)
	var quiet bool
	fs.BoolVar(&quiet, "quiet", false, "suppress diff output; only the exit code reports the result")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsDiffUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	rest, ok := requireArgs(fs, stderr, prog, "apps diff", "an app name and a local app.yaml path", 2)
	if !ok {
		return exitUsage
	}
	name, file := rest[0], rest[1]

	data, readErr := os.ReadFile(file) //nolint:gosec // operator-supplied CLI flag, same pattern apps_deploy_spec.go's own --file read uses
	if readErr != nil {
		return reportDiffError(stdout, stderr, jsonOut, quiet, fmt.Errorf("read %s: %w", file, readErr))
	}
	fileSpec, parseErr := spec.Parse(data)
	if parseErr != nil {
		return reportDiffError(stdout, stderr, jsonOut, quiet, newValidationError("parse %s: %v", file, parseErr))
	}

	localSvc, selErr := selectDiffService(fileSpec, name, file)
	if selErr != nil {
		return reportDiffError(stdout, stderr, jsonOut, quiet, selErr)
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	deployedYAML, err := client.ExportAppSpec(context.Background(), name)
	if err != nil {
		return reportDiffError(stdout, stderr, jsonOut, quiet, fmt.Errorf("export deployed spec for %q: %w", name, err))
	}
	// yaml.Unmarshal, not spec.Parse: the deployed spec always has an
	// empty build: block (build config isn't tracked in desired state,
	// see appSpecNotTrackedFields), which fails spec.Parse's schema
	// validation (build.type is a required enum). That's fine here: this
	// document is the control plane's own already-computed output, not
	// user-authored input that needs validating.
	var deployedSpec spec.Spec
	if err := yaml.Unmarshal(deployedYAML, &deployedSpec); err != nil {
		return reportDiffError(stdout, stderr, jsonOut, quiet, fmt.Errorf("parse deployed spec for %q: %w", name, err))
	}
	deployedSvc, ok := deployedSpec.Services[name]
	if !ok {
		return reportDiffError(stdout, stderr, jsonOut, quiet, fmt.Errorf("deployed spec for %q has no service named %q", name, name))
	}

	diff := diffAppSpecServices(localSvc, deployedSvc)

	if !quiet {
		result := appSpecDiffResult{
			App: name, File: file,
			Differences:   diff.Differences,
			NotComparable: diff.NotComparable,
			NotTracked:    appSpecNotTrackedFields,
		}
		if err := renderResult(stdout, of.Format, of.Query, result, func() { printAppSpecDiffHuman(stdout, result) }); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitCodeForError(err)
		}
	}

	if len(diff.Differences) > 0 {
		return exitDiffFound
	}
	return exitOK
}

// reportDiffError is reportError with a --quiet override: a --quiet run
// that fails before it can even compute a diff still needs the error on
// stderr (silently exiting nonzero with no explanation at all would be
// worse than the noise --quiet is meant to suppress), but must not print
// the --json error body, since --quiet's whole contract is "nothing on
// stdout but the exit code".
func reportDiffError(stdout, stderr io.Writer, jsonOut, quiet bool, err error) int {
	if quiet {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return reportError(stdout, stderr, jsonOut, err)
}

// selectDiffService picks the one service in fileSpec to diff against
// name's deployed state: the entry keyed name itself, or, when file
// declares exactly one service under a different key, that sole entry
// (the natural shape for a single-service app.yaml that doesn't happen
// to name its one service after the deployed app).
func selectDiffService(fileSpec *spec.Spec, name, file string) (spec.Service, error) {
	if svc, ok := fileSpec.Services[name]; ok {
		return svc, nil
	}
	if len(fileSpec.Services) == 1 {
		for _, svc := range fileSpec.Services {
			return svc, nil
		}
	}
	keys := make([]string, 0, len(fileSpec.Services))
	for k := range fileSpec.Services {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return spec.Service{}, newValidationError("%s declares no service named %q (services: %v); name it %q or leave exactly one service in the file", file, name, keys, name)
}

// printAppSpecDiffHuman renders result as a readable line-per-difference
// summary, the same shape "apps deploys compare"'s own
// printDeployCompareHuman establishes for a comparable "what changed"
// view.
func printAppSpecDiffHuman(w io.Writer, result appSpecDiffResult) {
	if len(result.Differences) == 0 {
		_, _ = fmt.Fprintf(w, "%s: no differences from %s\n", result.App, result.File)
	} else {
		_, _ = fmt.Fprintf(w, "%s: %d difference(s) from %s\n", result.App, len(result.Differences), result.File)
		for _, d := range result.Differences {
			_, _ = fmt.Fprintf(w, "  %s\n", d)
		}
	}
	if len(result.NotComparable) > 0 {
		_, _ = fmt.Fprintf(w, "not comparable (cross-resource { from: ... } references aren't preserved in the deployed export): %v\n", result.NotComparable)
	}
	_, _ = fmt.Fprintf(w, "not compared (not tracked in the deployed spec export): %v\n", result.NotTracked)
}

func appsDiffUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps diff <name> <file> [flags]

Compares the service in <file> named <name> (or <file>'s only service,
if it declares exactly one) against <name>'s actually-deployed state
(the same reconstruction "%[1]s apps export" writes). Prints each
field that differs (port, domains, env vars, resources, health checks,
labels) and exits nonzero if any do, for a "fail if drifted" check in
CI. build/hostPort/replicas/strategy/volumes/hooks are never compared:
they aren't tracked in the deployed spec export.

Flags:
  --quiet                  suppress diff output; only the exit code reports the result
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print the result as JSON to stdout and nothing else
  --output string          output format: json, table, or text (default table)
  --query string           JMESPath expression to filter the result before printing
  -h, --help              show this help

Exit codes:
  0   identical
  %[5]d   a difference was found
  1-4 the usual flag/network/API error tiers
`, prog, envAPIToken, envAPIURL, defaultAPIURL, exitDiffFound)
}
