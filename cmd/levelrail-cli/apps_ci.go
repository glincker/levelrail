package main

import (
	"fmt"
	"io"
)

// runAppsCI dispatches "apps ci token|trust|workflow <app>", the provider-neutral home of the CI commands.
func runAppsCI(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, appsCIUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, appsCIUsage(prog))
		return exitOK
	case "token":
		return runCIToken(prog, "apps ci token", args[1:], stdout, stderr, lookupEnv)
	case "trust":
		return runCITrust(prog, "apps ci trust", args[1:], stdout, stderr, lookupEnv)
	case "workflow":
		return runAppsGitHubActionsWorkflow(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown apps ci subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, appsCIUsage(prog))
		return exitUsage
	}
}

func appsCIUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps ci token <app> [flags]                swap the job OIDC identity for a 15 minute deploy token (--provider github, gitlab or circleci)
  %[1]s apps ci trust list|add|remove <app> [flags]   which pipelines may deploy the app (--provider, --repo, --ref, --issuer, --org-id)
  %[1]s apps ci workflow <app> [flags]             print a GitHub Actions deploy workflow

"apps github-actions token" and "apps github-actions trust" still work as deprecated aliases.
On GitLab and CircleCI the token command reads the job identity token from LEVELRAIL_ID_TOKEN,
or from the variable named by --id-token-env.
`, prog)
}
