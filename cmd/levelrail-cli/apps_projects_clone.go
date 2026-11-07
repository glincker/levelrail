package main

import (
	"context"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsProjectsClone implements "apps projects clone <name> --to <new>":
// the configuration of a project (or one environment) as a new project.
func runAppsProjectsClone(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps projects clone", "print the clone plan as JSON", stderr)
	var to, env, domains string
	var dryRun, noEnvVars, noSettings bool
	fs.StringVar(&to, "to", "", "name of the new project (required)")
	fs.StringVar(&env, "env", "", "clone only this environment (name or ID) of the project")
	fs.StringVar(&domains, "domains", "none", "domains: none, or placeholder for unroutable stand-ins")
	fs.BoolVar(&dryRun, "dry-run", false, "print the plan and change nothing")
	fs.BoolVar(&noEnvVars, "no-env-vars", false, "do not copy plain env vars (secret keys are still listed)")
	fs.BoolVar(&noSettings, "no-settings", false, "copy image and port only")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps projects clone <name> --to <new> [--env ENV] [--dry-run] [flags]\n\nCopies app definitions, plain env vars and settings into a new project. Secret\nvalues, volume data, database data and live domains are never copied; secret keys\nare created empty. Cloned apps are created stopped.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 || to == "" {
		_, _ = fmt.Fprintf(stderr, "%s: apps projects clone requires one project name and --to\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	plan, err := client.CloneProject(context.Background(), resolveProjectID(client, rest[0]), apiclient.ProjectCloneRequest{
		Name:        to,
		Environment: env,
		Include:     apiclient.ProjectCloneInclude{EnvVars: !noEnvVars, Domains: domains, Settings: !noSettings},
		DryRun:      dryRun,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clone project %q: %w", rest[0], err))
	}
	if err := renderResult(stdout, of.Format, of.Query, plan, func() { printCloneSummary(stdout, plan) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printCloneSummary(w io.Writer, p apiclient.ProjectClonePlan) {
	verb := "created"
	if p.DryRun {
		verb = "would create"
	}
	_, _ = fmt.Fprintf(w, "%s project %q with %d app(s), stopped\n", verb, p.Project.Name, len(p.Apps))
	for _, a := range p.Apps {
		_, _ = fmt.Fprintf(w, "  app %s -> %s\n", a.Source, a.Name)
	}
	for _, k := range p.SecretKeys {
		_, _ = fmt.Fprintf(w, "  needs a value: %s %s\n", k.Scope, k.Key)
	}
	for _, s := range p.Skipped {
		_, _ = fmt.Fprintf(w, "  skipped %s %s: %s\n", s.Scope, s.Item, s.Reason)
	}
}
