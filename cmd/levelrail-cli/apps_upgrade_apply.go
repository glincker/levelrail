package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsUpgrade applies the template upgrade plan to an app and follows it
// until the new release is stable or has been rolled back.
func runAppsUpgrade(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps upgrade", "print the result as JSON to stdout and nothing else", stderr)
	yes := fs.Bool("yes", false, "apply the upgrade; without it only the plan is shown")
	noWait := fs.Bool("no-wait", false, "return once the upgrade is started instead of following it")
	interval := fs.Duration("interval", 5*time.Second, "how often to poll the upgrade while following it")
	waitMax := fs.Duration("wait-timeout", 15*time.Minute, "stop following after this long")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps upgrade <app-name> --yes [flags]\n\nApplies the current catalog template to an app deployed from one. It refuses\non blockers and when the app volumes have no fresh backup, deploys the new\nspec, watches the release and rolls back on a crash loop or failed deploy.\nWithout --yes it prints the plan and changes nothing.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps upgrade", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	if !*yes {
		plan, err := client.GetTemplateUpgradePlan(ctx, name)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade plan for app %q: %w", name, err))
		}
		code := writeScheduledTaskResult(stdout, stderr, of, plan, func() { printTemplateUpgradePlan(stdout, plan) })
		if code == exitOK && !jsonOut {
			_, _ = fmt.Fprintf(stderr, "nothing applied: re-run with --yes to upgrade %s\n", name)
		}
		return code
	}
	run, err := client.ApplyTemplateUpgrade(ctx, name, true)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("upgrade app %q: %w", name, err))
	}
	if !*noWait {
		run = followTemplateUpgrade(ctx, client, name, run, *interval, *waitMax)
	}
	code := writeScheduledTaskResult(stdout, stderr, of, run, func() { printTemplateUpgradeRun(stdout, run) })
	if code == exitOK && run.State != "succeeded" && run.State != "watching" {
		return exitDeployFailed
	}
	return code
}

func followTemplateUpgrade(ctx context.Context, client *Client, name string, run apiclient.TemplateUpgradeRun, every, limit time.Duration) apiclient.TemplateUpgradeRun {
	deadline := time.Now().Add(limit)
	for run.State == "watching" && time.Now().Before(deadline) {
		time.Sleep(every)
		next, err := client.GetTemplateUpgradeRun(ctx, name)
		if err != nil {
			continue
		}
		run = next
	}
	return run
}

func printTemplateUpgradeRun(out io.Writer, r apiclient.TemplateUpgradeRun) {
	_, _ = fmt.Fprintf(out, "%s: %s (revision %d -> %d)\n", r.AppID, r.State, r.FromRevision, r.ToRevision)
	if r.Message != "" {
		_, _ = fmt.Fprintln(out, r.Message)
	}
	if r.RollbackRevision > 0 {
		_, _ = fmt.Fprintf(out, "rolled back as revision %d\n", r.RollbackRevision)
	}
}
