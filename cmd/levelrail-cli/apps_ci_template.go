package main

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsCITemplate serves "apps ci template <app> [provider]": the example pipeline for one CI, or the list of providers.
func runAppsCITemplate(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps ci template", "print the templates as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		_, _ = fmt.Fprintf(stderr, "usage: %s apps ci template <app> [provider]\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetCITemplates(context.Background(), rest[0])
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get ci templates for %s: %w", rest[0], err))
	}
	if len(rest) == 2 && !hasCITemplate(res, rest[1]) {
		_, _ = fmt.Fprintf(stderr, "%s: unknown provider %q, run %s apps ci template %s to list them\n", prog, rest[1], prog, rest[0])
		return exitUsage
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if len(rest) == 1 {
			for _, t := range res.Templates {
				_, _ = fmt.Fprintf(stdout, "%-10s %-28s auth: %s\n", t.ID, t.Target, t.Auth)
			}
			return
		}
		for _, t := range res.Templates {
			if t.ID != rest[1] {
				continue
			}
			_, _ = fmt.Fprint(stdout, t.Content)
			if res.APIURL != "" {
				_, _ = fmt.Fprintf(stderr, "note: set LEVELRAIL_API_URL to %s in your CI\n", res.APIURL)
			} else {
				_, _ = fmt.Fprintf(stderr, "note: no dashboard URL is set yet, set one in Settings so CI can reach this control plane\n")
			}
			if t.Auth == "token" {
				_, _ = fmt.Fprintf(stderr, "note: this file needs a stored token, create one with: %s\n", deployerTokenCommand(prog, rest[0], res))
			}
		}
	})
}

func hasCITemplate(res apiclient.CITemplates, id string) bool {
	for _, t := range res.Templates {
		if t.ID == id {
			return true
		}
	}
	return false
}

// deployerTokenCommand is the tokens create line for a deployer token, limited to the app's project when it has one,
// followed by what the token can reach.
func deployerTokenCommand(prog, app string, res apiclient.CITemplates) string {
	if sc := res.DeployerScope; sc != nil {
		return fmt.Sprintf("%s tokens create --name ci-%s --abilities %s --scope-team %s --scope-project %s --scope-role %s\n      (the token can read and deploy every app in project %s, not only %s)", prog, app, strings.Join(res.DeployerAbilities, ","), sc.TeamID, sc.ProjectID, sc.TeamRole, sc.ProjectID, app)
	}
	cmd := fmt.Sprintf("%s tokens create --name ci-deployer --abilities %s", prog, strings.Join(res.DeployerAbilities, ","))
	if res.DeployerScopeProjectMissing {
		return cmd + "\n      (the app names a project that no longer exists, so this token can deploy any app on this server; fix the app's project first)"
	}
	if res.DeployerScopeUnavailable {
		return cmd + "\n      (this server cannot scope tokens to the app's project, so the token can deploy any app on this server)"
	}
	return cmd + "\n      (the app is in no project, so this token can deploy any app on this server)"
}
