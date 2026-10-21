package main

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// runUpgradeAttemptAction implements "upgrade retry|rollback <attempt>".
func runUpgradeAttemptAction(prog, action string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	id := ""
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		id, args = args[0], args[1:]
	}
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "upgrade "+action, "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s upgrade retry <attempt>\n  %s upgrade rollback <attempt>\n\nretry runs the failed upgrade again against the same release. rollback restores the pre-upgrade\nrestore point taken for the attempt. Find attempt ids with: %s upgrade --attempts\n\nFlags:\n", prog, prog, prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if id == "" {
		return reportError(stdout, stderr, jsonOut, errors.New("usage: upgrade "+action+" <attempt>"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()
	if action == "retry" {
		started, err := client.RetrySelfUpgradeAttempt(ctx, id)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("retry %s: %w", id, err))
		}
		return renderUpgradeResult(stdout, stderr, of, started, func() {
			_, _ = fmt.Fprintf(stdout, "retrying the upgrade to %s on the host; follow it with: %s upgrade --attempts\n", started.TargetVersion, prog)
		})
	}
	started, err := client.RollbackSelfUpgradeAttempt(ctx, id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("rollback %s: %w", id, err))
	}
	return renderUpgradeResult(stdout, stderr, of, started, func() {
		_, _ = fmt.Fprintf(stdout, "restoring %s (%s) on the host; follow it with: %s upgrade --attempts\n", started.RestorePoint, started.Version, prog)
	})
}
