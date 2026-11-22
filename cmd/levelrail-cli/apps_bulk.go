package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAppsBulk implements "apps bulk <action> [value]": POST /api/v1/apps/bulk.
func runAppsBulk(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps bulk", "print the per-app results as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsBulkUsage(prog)) }
	var tag, env, project, names string
	var dryRun, yes, haltOnFail bool
	var batch, pause, replicas, minReplicas, maxReplicas int
	fs.StringVar(&tag, "tag", "", "select every app carrying this tag")
	fs.StringVar(&env, "env", "", "select every app in this environment (name or ID)")
	fs.StringVar(&project, "project", "", "select every app in this project (ID)")
	fs.StringVar(&names, "names", "", "comma-separated app names to act on")
	fs.BoolVar(&dryRun, "dry-run", false, "show what would be applied without changing anything")
	fs.IntVar(&batch, "batch", 0, "apply in waves of this many apps (0 runs them all in one pass)")
	fs.IntVar(&pause, "pause", 0, "seconds to wait between waves")
	fs.BoolVar(&haltOnFail, "halt-on-fail", false, "stop before the next wave when a wave has a failure")
	fs.IntVar(&replicas, "replicas", 0, "scale: set this many replicas on apps without autoscale")
	fs.IntVar(&minReplicas, "min", 0, "scale: set the autoscale minimum (use with --max) on autoscaled apps")
	fs.IntVar(&maxReplicas, "max", 0, "scale: set the autoscale maximum (use with --min) on autoscaled apps")
	fs.BoolVar(&yes, "yes", false, "skip the confirmation prompt")

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 || len(rest) > 2 {
		_, _ = fmt.Fprintf(stderr, "%s: apps bulk requires an action and, for add-tag, remove-tag, set-environment and move-to-project, a value\n\n%s", prog, appsBulkUsage(prog))
		return exitUsage
	}
	req := apiclient.BulkAppsRequest{Action: rest[0], Tag: tag, Environment: env, Project: project, DryRun: dryRun, BatchSize: batch, PauseSeconds: pause, HaltOnFail: haltOnFail}
	if len(rest) == 2 {
		req.Value = rest[1]
	}
	if replicas > 0 {
		req.Replicas = &replicas
	}
	if minReplicas > 0 || maxReplicas > 0 {
		req.MinReplicas, req.MaxReplicas = &minReplicas, &maxReplicas
	}
	for _, n := range strings.Split(names, ",") {
		if n = strings.TrimSpace(n); n != "" {
			req.Names = append(req.Names, n)
		}
	}
	if len(req.Names) == 0 && tag == "" && env == "" && project == "" {
		_, _ = fmt.Fprintf(stderr, "%s: select apps with --tag, --env, --project or --names\n", prog)
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	if !dryRun {
		preview := req
		preview.DryRun = true
		plan, err := client.BulkApps(ctx, preview)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("bulk %s: %w", req.Action, err))
		}
		var targets []string
		for _, r := range plan.Results {
			if r.Status == "would_apply" {
				targets = append(targets, r.Name)
			}
		}
		if len(targets) == 0 {
			_, _ = fmt.Fprintln(stderr, "no apps to act on (all denied or missing)")
			return exitOK
		}
		if !yes {
			_, _ = fmt.Fprintf(stderr, "%s %d app(s): %s\nProceed? [y/N] ", req.Action, len(targets), strings.Join(targets, ", "))
			line, _ := bufio.NewReader(stdin).ReadString('\n')
			if a := strings.ToLower(strings.TrimSpace(line)); a != "y" && a != "yes" {
				_, _ = fmt.Fprintln(stderr, "aborted")
				return exitUsage
			}
		}
		req.Names = targets
		req.Tag, req.Environment, req.Project = "", "", ""
		req.ConfirmCount = len(targets)
		if req.Action == "delete" {
			req.ConfirmNames = targets
		}
	}

	resp, err := client.BulkApps(ctx, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("bulk %s: %w", req.Action, err))
	}
	code := writeScheduledTaskResult(stdout, stderr, of, resp, func() {
		for _, r := range resp.Results {
			line := fmt.Sprintf("%-32s %s", r.Name, r.Status)
			if r.Message != "" {
				line += "  " + r.Message
			}
			_, _ = fmt.Fprintln(stdout, line)
		}
	})
	if code == exitOK && (resp.Counts["error"] > 0 || resp.Counts["denied"] > 0) {
		return exitAPIError
	}
	return code
}

func appsBulkUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps bulk <action> [value] (--tag T | --env E | --project P | --names a,b) [--dry-run] [--yes]
      [--batch N] [--pause SECONDS] [--halt-on-fail]

Actions: redeploy, restart, stop, start, add-tag <tag>, remove-tag <tag>,
set-environment <env>, move-to-project <project-id>, delete,
scale (--replicas N, or --min N --max M for autoscaled apps), rollback (each app
goes back to its previous release; apps without one are skipped).

Each app is authorized separately; denied apps are listed, not hidden, and
do not fail the rest. Without --yes the targets are shown and confirmed
first. restart, stop, redeploy and rollback of more than 10 apps (setting bulk.confirm_above)
are also confirmed by count server side. delete always lists the exact apps it removes. redeploy skips apps in
protected environments (deploy those individually).

Flags:
  --tag, --env, --project, --names   select target apps
  --replicas N, --min N, --max M                scale targets
  --dry-run                                     show what would happen, change nothing
  --yes                                         skip the confirmation prompt
  --token, --api-url, --profile, --json, --output, --query   as for other commands
`, prog)
}
