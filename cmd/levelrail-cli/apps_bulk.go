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
	var tag, env, names string
	var dryRun, yes bool
	fs.StringVar(&tag, "tag", "", "select every app carrying this tag")
	fs.StringVar(&env, "env", "", "select every app in this environment (name or ID)")
	fs.StringVar(&names, "names", "", "comma-separated app names to act on")
	fs.BoolVar(&dryRun, "dry-run", false, "show what would be applied without changing anything")
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
	req := apiclient.BulkAppsRequest{Action: rest[0], Tag: tag, Environment: env, DryRun: dryRun}
	if len(rest) == 2 {
		req.Value = rest[1]
	}
	for _, n := range strings.Split(names, ",") {
		if n = strings.TrimSpace(n); n != "" {
			req.Names = append(req.Names, n)
		}
	}
	if len(req.Names) == 0 && tag == "" && env == "" {
		_, _ = fmt.Fprintf(stderr, "%s: select apps with --tag, --env or --names\n", prog)
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
		req.Tag, req.Environment = "", ""
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
  %[1]s apps bulk <action> [value] (--tag T | --env E | --names a,b) [--dry-run] [--yes]

Actions: redeploy, restart, stop, start, add-tag <tag>, remove-tag <tag>,
set-environment <env>, move-to-project <project-id>, delete.

Each app is authorized separately; denied apps are listed, not hidden, and
do not fail the rest. Without --yes the targets are shown and confirmed
first. delete always lists the exact apps it removes. redeploy skips apps in
protected environments (deploy those individually).

Flags:
  --tag string, --env string, --names string   select target apps
  --dry-run                                     show what would happen, change nothing
  --yes                                         skip the confirmation prompt
  --token, --api-url, --profile, --json, --output, --query   as for other commands
`, prog)
}
