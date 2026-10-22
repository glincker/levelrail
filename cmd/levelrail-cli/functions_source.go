package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/functions"
	"github.com/GLINCKER/levelrail/internal/spec"
)

type functionSourceFlags struct {
	repo, ref, dir string
	wait           time.Duration
}

// deployFunctionFromRepo builds a handler repo with Railpack, then puts the app to sleep when idle with request hold.
func deployFunctionFromRepo(ctx context.Context, client *Client, name string, port int, idle time.Duration, envs map[string]string, src functionSourceFlags, of outputFlags, stdout, stderr io.Writer, jsonOut bool) int {
	detected := detectLocalGit()
	repo := src.repo
	if repo == "" {
		repo = detected.RepoURL
	}
	if repo == "" {
		_, _ = fmt.Fprintln(stderr, "functions deploy needs --image, or --repo (a git URL, auto-detected from the origin remote of the current directory)")
		return exitUsage
	}
	ref := resolveRef(src.ref, detected.Ref)

	if _, err := client.GetApp(ctx, name); err != nil {
		env := map[string]string{"PORT": fmt.Sprint(port)}
		for k, v := range envs {
			env[k] = v
		}
		if _, err := client.CreateApp(ctx, appResource{Name: name, Image: pendingImageTag(name), Port: port, Env: env}); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("create function %q: %w", name, err))
		}
	}
	req := buildTriggerRequest{
		RepoURL:   repo,
		Ref:       ref,
		ImageRepo: name,
		Build:     buildTriggerRequestBuild{Type: spec.BuildRailpack, BaseDirectory: src.dir},
	}
	attempt, err := client.TriggerBuild(ctx, name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("build function %q from %s: %w", name, repo, err))
	}
	if src.wait > 0 {
		res, err := client.WaitForDeploy(ctx, name, attempt.ID, apiclient.WaitOptions{Interval: 2 * time.Second, Window: src.wait})
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("wait for function %q: %w", name, err))
		}
		if !res.Done {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("function %q is still building after %s (follow with: apps deploys logs %s %s)", name, src.wait, name, attempt.ID))
		}
		if res.Deploy.Outcome != apiclient.DeployOutcomeHealthy {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("function %q did not become healthy (outcome %s, see: apps deploys logs %s %s)", name, res.Deploy.Outcome, name, attempt.ID))
		}
	}
	if _, err := client.SetAppSleepHold(ctx, name, int(idle.Minutes()), true); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("function %q was built but sleep could not be set (fix with: apps sleep enable %s --hold): %w", name, name, err))
	}
	app, err := client.GetApp(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get function %q: %w", name, err))
	}
	row := functionRow{Name: name, Image: app.Image, IdleMinutes: int(idle.Minutes()), URL: appURL(app.Domains, app.FallbackURL)}
	return writeScheduledTaskResult(stdout, stderr, of, row, func() {
		_, _ = fmt.Fprintf(stdout, "function %q built from %s and live, sleeps after %dm idle\n", name, repo, row.IdleMinutes)
		if row.URL != "" {
			_, _ = fmt.Fprintf(stdout, "url: %s\n", row.URL)
		}
	})
}

// runFunctionsInit writes a starter handler for a runtime into dir without overwriting anything.
func runFunctionsInit(prog string, args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || len(args) > 2 {
		_, _ = fmt.Fprintf(stderr, "Usage: %s functions init <%s> [dir]\n", prog, strings.Join(runtimeIDs(), "|"))
		return exitUsage
	}
	rtm, ok := functions.Lookup(args[0])
	if starter, isStarter := functions.LookupStarter(args[0]); !ok && isStarter {
		rtm, ok = functions.Runtime{ID: starter.ID, Name: starter.Name, Files: starter.Files}, true
	}
	if !ok {
		_, _ = fmt.Fprintf(stderr, "%s: unknown runtime %q, choose one of: %s\n", prog, args[0], strings.Join(runtimeIDs(), ", "))
		return exitUsage
	}
	dir := "."
	if len(args) == 2 {
		dir = args[1]
	}
	for _, f := range rtm.Files {
		if _, err := os.Stat(filepath.Join(dir, f.Path)); err == nil {
			_, _ = fmt.Fprintf(stderr, "%s: %s already exists, refusing to overwrite\n", prog, filepath.Join(dir, f.Path))
			return exitUsage
		}
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: create %s: %v\n", prog, dir, err)
		return exitAPIError
	}
	for _, f := range rtm.Files {
		if err := os.WriteFile(filepath.Join(dir, f.Path), []byte(f.Content), 0o600); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: write %s: %v\n", prog, f.Path, err)
			return exitAPIError
		}
		_, _ = fmt.Fprintf(stdout, "wrote %s\n", filepath.Join(dir, f.Path))
	}
	_, _ = fmt.Fprintf(stdout, "next: commit and push, then run: %s functions deploy <name> --repo <git url>\n", prog)
	return exitOK
}

func runtimeIDs() []string {
	var ids []string
	for _, r := range functions.Runtimes() {
		ids = append(ids, r.ID)
	}
	sort.Strings(ids)
	return ids
}

func runFunctionsRuntimes(stdout io.Writer) int {
	for _, r := range functions.Runtimes() {
		_, _ = fmt.Fprintf(stdout, "%-8s %s\n", r.ID, r.Name)
	}
	return exitOK
}
