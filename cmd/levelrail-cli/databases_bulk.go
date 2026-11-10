package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runDatabasesBulk implements "databases bulk start|stop|backup": POST /api/v1/databases/bulk.
func runDatabasesBulk(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "databases bulk", "print the per-database results as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, databasesBulkUsage(prog)) }
	var selector string
	var dryRun bool
	confirmCount := -1
	fs.StringVar(&selector, "selector", "", "select databases by engine=E, project=ID and environment=ID (comma separated)")
	fs.BoolVar(&dryRun, "dry-run", false, "show the plan (what would run, what is skipped and why) without changing anything")
	fs.Func("confirm-count", "number of target databases, required to stop", func(v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return fmt.Errorf("must be a non-negative number")
		}
		confirmCount = n
		return nil
	})

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) < 1 {
		_, _ = fmt.Fprintf(stderr, "%s: databases bulk requires an action (start, stop or backup)\n\n%s", prog, databasesBulkUsage(prog))
		return exitUsage
	}
	req := apiclient.BulkDatabasesRequest{Action: rest[0], Names: rest[1:], DryRun: dryRun}
	if req.Action != "start" && req.Action != "stop" && req.Action != "backup" {
		_, _ = fmt.Fprintf(stderr, "%s: unknown bulk action %q (use start, stop or backup)\n", prog, req.Action)
		return exitUsage
	}
	if err := applyDatabaseSelector(&req, selector); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitUsage
	}
	if len(req.Names) == 0 && selector == "" {
		_, _ = fmt.Fprintf(stderr, "%s: name the databases or pass --selector\n", prog)
		return exitUsage
	}
	if len(req.Names) > 0 && selector != "" {
		_, _ = fmt.Fprintf(stderr, "%s: pass names or --selector, not both\n", prog)
		return exitUsage
	}
	if confirmCount >= 0 {
		req.ConfirmCount = confirmCount
	}
	if req.Action == "stop" && !dryRun && confirmCount < 0 {
		_, _ = fmt.Fprintf(stderr, "%s: stop needs --confirm-count N equal to the number of targets (run with --dry-run to see them)\n", prog)
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	resp, err := client.BulkDatabases(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("bulk %s: %w", req.Action, err))
	}
	code := writeScheduledTaskResult(stdout, stderr, of, resp, func() {
		for _, r := range resp.Results {
			line := fmt.Sprintf("%-32s %s", r.Name, r.Status)
			if r.Message != "" {
				line += "  " + r.Message
			}
			if r.BackupID != "" {
				line += "  backup " + r.BackupID
			}
			_, _ = fmt.Fprintln(stdout, line)
		}
	})
	if code == exitOK && (resp.Counts["error"] > 0 || resp.Counts["denied"] > 0 || resp.Counts["not_found"] > 0 || resp.Counts["protected"] > 0) {
		return exitAPIError
	}
	return code
}

func applyDatabaseSelector(req *apiclient.BulkDatabasesRequest, selector string) error {
	for _, part := range strings.Split(selector, ",") {
		if part = strings.TrimSpace(part); part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(v) == "" {
			return fmt.Errorf("selector %q: want key=value", part)
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "engine":
			req.Engine = v
		case "project":
			req.ProjectID = v
		case "environment", "env":
			req.EnvironmentID = v
		default:
			return fmt.Errorf("selector key %q: use engine, project or environment", k)
		}
	}
	return nil
}

func databasesBulkUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s databases bulk start|stop|backup <names...|--selector S> [--dry-run] [--confirm-count N] [--output json]

Runs one action on many databases in one audited call. Each database is
authorized on its own at the same ability as the single-database command;
denied, protected, missing and failed databases are listed, never hidden,
and do not stop the rest. --dry-run returns the plan and changes nothing.
stop needs --confirm-count equal to the number of targets, and refuses
databases in a protected environment (stop those individually).

Flags:
  --selector string     engine=E,project=ID,environment=ID instead of names
  --dry-run             show what would run and what is skipped, change nothing
  --confirm-count int   number of targets, required for stop
  --token, --api-url, --profile, --json, --output, --query   as for other commands
`, prog)
}
