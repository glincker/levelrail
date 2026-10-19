package main

import (
	"context"
	"fmt"
	"io"
)

type deployURLResult struct {
	App      string `json:"app"`
	DeployID string `json:"deploy_id"`
	URL      string `json:"url"`
	Live     bool   `json:"live"`
}

// runAppsDeploysURL implements "apps deploys url <name> <deploy-id>": the
// address that serves one past deploy, kept so it can be compared without a rollback.
func runAppsDeploysURL(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps deploys url", "print the URL as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps deploys url <name> <deploy-id> [flags]\n\nPrints the deploy's own URL. The live deploy answers on the app's own URL.\n", prog)
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps deploys url requires an app name and a deploy id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name, id := rest[0], rest[1]
	ctx := context.Background()
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	attempts, err := client.ListDeployAttempts(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list deploys for app %q: %w", name, err))
	}
	res := deployURLResult{App: name, DeployID: id}
	found := false
	for _, a := range attempts {
		if a.ID != id {
			continue
		}
		found, res.URL, res.Live = true, a.DeployURL, a.Live
		break
	}
	if !found {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy %q not found for app %q", id, name))
	}
	if res.URL == "" && res.Live {
		app, err := client.GetApp(ctx, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get app %q: %w", name, err))
		}
		res.URL = appURL(app.Domains, app.FallbackURL)
	}
	if res.URL == "" {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("deploy %q has no URL: it is not among the newest successful deploys that keep one (see APP_DEPLOY_URL_KEEP)", id))
	}
	if err := renderResult(stdout, of.Format, of.Query, res, func() { _, _ = fmt.Fprintln(stdout, res.URL) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}
