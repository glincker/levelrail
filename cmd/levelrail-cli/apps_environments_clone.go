package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
)

// runAppsEnvironmentsClonePreview implements "apps environments
// clone-preview <id> --new-name NAME": GET
// /api/v1/environments/{id}/clone/preview
// (internal/api/environment_clone.go), a read-only look at what cloning
// id into a new environment named --new-name would create, before
// running "apps environments clone" for real.
func runAppsEnvironmentsClonePreview(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps environments clone-preview", "print the preview as JSON to stdout and nothing else", stderr)
	var newName string
	fs.StringVar(&newName, "new-name", "", "name for the new environment (required)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsEnvironmentsClonePreviewUsage(prog)) }

	id, tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseEnvironmentIDCommand(fs, args, stderr, prog, "apps environments clone-preview", apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP})
	if !ok {
		return exitCode
	}
	if newName == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--new-name is required"))
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	preview, err := client.PreviewEnvironmentClone(context.Background(), id, newName)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("preview clone of environment %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, preview, func() { printEnvironmentClonePreviewHuman(stdout, preview) })
}

func printEnvironmentClonePreviewHuman(w io.Writer, prev environmentClonePreviewResource) {
	_, _ = fmt.Fprintf(w, "clone %q -> new environment %q\n", prev.SourceEnvironment.Name, prev.NewEnvironmentName)
	if len(prev.Apps) == 0 {
		_, _ = fmt.Fprint(w, "\nno apps tagged with this environment; nothing to clone\n")
	} else {
		_, _ = fmt.Fprint(w, "\napps:\n")
		for _, a := range prev.Apps {
			_, _ = fmt.Fprintf(w, "  %-20s -> %-20s  image %s\n", a.SourceApp, a.SuggestedNewName, a.Image)
			if len(a.CurrentDomains) > 0 {
				_, _ = fmt.Fprintf(w, "      current domains (not copied): %s\n", strings.Join(a.CurrentDomains, ", "))
			}
			if len(a.SecretEnvKeys) > 0 {
				_, _ = fmt.Fprintf(w, "      secret env vars (values not copied unless --copy-secret-values): %s\n", strings.Join(a.SecretEnvKeys, ", "))
			}
			if a.ScheduledTaskCount > 0 {
				_, _ = fmt.Fprintf(w, "      scheduled tasks: %d\n", a.ScheduledTaskCount)
			}
		}
	}
	if len(prev.EnvironmentEnvVarKeys) > 0 {
		_, _ = fmt.Fprintf(w, "\nshared env vars: %s\n", strings.Join(prev.EnvironmentEnvVarKeys, ", "))
	}
	if len(prev.EnvironmentSecretEnvKeys) > 0 {
		_, _ = fmt.Fprintf(w, "shared secret env vars: %s\n", strings.Join(prev.EnvironmentSecretEnvKeys, ", "))
	}
	_, _ = fmt.Fprintf(w, "\nnot cloned: %v\n%s\n", prev.UnclonedFields, prev.Note)
}

func appsEnvironmentsClonePreviewUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps environments clone-preview <id> --new-name NAME [flags]

Shows what cloning environment <id> into a new environment named NAME
would create: one entry per tagged app (its suggested new name, current
image, domains that will NOT be copied, declared secret env var names),
plus the environment's own shared env var keys. Nothing is created.

Flags:
  --new-name string       name for the new environment (required)
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string        control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string        named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                     print the preview as JSON to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// runAppsEnvironmentsClone implements "apps environments clone <id>
// --new-name NAME [--app-rename SOURCE=NEWNAME ...] [--domain
// SOURCE=domain1,domain2 ...] [--copy-secret-values]": POST
// /api/v1/environments/{id}/clone (internal/api/environment_clone.go).
// Creates a new environment in <id>'s own project plus a real, deployed
// copy of every app tagged with <id>. --app-rename/--domain only need to
// be given for apps whose default (auto-suggested name, no domains)
// isn't what's wanted; every other tagged app clones with its default.
func runAppsEnvironmentsClone(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps environments clone", "print the created environment and apps as JSON to stdout and nothing else", stderr)
	var newName string
	var copySecretValues bool
	renames := stringMapFlag{}
	domains := stringMapFlag{}
	fs.StringVar(&newName, "new-name", "", "name for the new environment (required)")
	fs.BoolVar(&copySecretValues, "copy-secret-values", false, "also copy real secret values (per-app and shared) onto the clone; without this flag every secret is declared but left unset, the safe default")
	fs.Var(renames, "app-rename", "override a cloned app's own new name, as SOURCE=NEWNAME, repeatable")
	fs.Var(domains, "domain", "assign domains to a cloned app, as SOURCE=domain1,domain2,..., repeatable")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsEnvironmentsCloneUsage(prog)) }

	id, tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseEnvironmentIDCommand(fs, args, stderr, prog, "apps environments clone", apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP})
	if !ok {
		return exitCode
	}
	if newName == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--new-name is required"))
	}

	apps := clonedAppInputsFromFlags(renames, domains)

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	result, err := client.CloneEnvironment(context.Background(), id, environmentCloneRequest{
		NewEnvironmentName: newName,
		CopySecretValues:   copySecretValues,
		Apps:               apps,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clone environment %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		_, _ = fmt.Fprintf(stdout, "environment %q cloned into %q (id %s); reconcile is asynchronous, check \"%s apps status <name>\" for each app below\n", id, result.Environment.Name, result.Environment.ID, prog)
		for _, a := range result.Apps {
			_, _ = fmt.Fprintf(stdout, "  %s -> %s (image %s)\n", a.SourceApp, a.NewApp, a.Image)
		}
	})
}

// clonedAppInputsFromFlags merges --app-rename and --domain into one
// sorted (for deterministic output) []environmentCloneAppInput, one
// entry per source app named by either flag.
func clonedAppInputsFromFlags(renames, domains stringMapFlag) []environmentCloneAppInput {
	names := make(map[string]bool, len(renames)+len(domains))
	for k := range renames {
		names[k] = true
	}
	for k := range domains {
		names[k] = true
	}
	sourceApps := make([]string, 0, len(names))
	for k := range names {
		sourceApps = append(sourceApps, k)
	}
	sort.Strings(sourceApps)

	apps := make([]environmentCloneAppInput, 0, len(sourceApps))
	for _, src := range sourceApps {
		input := environmentCloneAppInput{SourceApp: src, NewName: renames[src]}
		if d := domains[src]; d != "" {
			input.Domains = strings.Split(d, ",")
		}
		apps = append(apps, input)
	}
	return apps
}

func appsEnvironmentsCloneUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps environments clone <id> --new-name NAME [flags]

Clones environment <id>: creates a new environment named NAME in the
same project, then a real, deployed copy of every app tagged with <id>
(image, env vars, resources, health checks, volumes, bind mounts,
labels, scheduled tasks, and more). See "%[1]s apps environments
clone-preview" for the exact list of what is and isn't carried over.

Every cloned app gets an auto-suggested new name (its own name plus the
new environment's own name) and no domains, unless overridden:

Flags:
  --new-name string          name for the new environment (required)
  --app-rename SOURCE=NEWNAME  override a cloned app's own new name, repeatable
  --domain SOURCE=D1,D2,...    assign domains to a cloned app, repeatable
  --copy-secret-values          also copy real secret values (per-app and shared)
                                     onto the clone; without this every secret is
                                     declared but left unset (the safe default)
  --token string             API token (default: %[2]s env var, then the credentials file)
  --api-url string          control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string          named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                       print the created environment and apps as JSON to stdout, nothing else
  --output string            output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string             JMESPath expression to filter the result before printing
  -h, --help                 show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
