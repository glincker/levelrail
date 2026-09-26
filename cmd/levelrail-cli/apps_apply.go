package main

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// pendingChangeCount is how many individual changes are pending: one per
// key, or one for a change that carries none.
func pendingChangeCount(p apiclient.PendingChanges) int {
	n := 0
	for _, c := range p.Changes {
		n += max(len(c.Keys), 1)
	}
	return n
}

// describePendingChanges renders "env A, B; secret TOKEN; config port".
func describePendingChanges(p apiclient.PendingChanges) string {
	parts := make([]string, 0, len(p.Changes))
	for _, c := range p.Changes {
		keys := append([]string(nil), c.Keys...)
		sort.Strings(keys)
		parts = append(parts, strings.TrimSpace(c.Kind+" "+strings.Join(keys, ", ")))
	}
	return strings.Join(parts, "; ")
}

// afterConfigWrite is what "apps env import" and "apps secrets set" do once
// a save landed: with apply it restarts the app right away, otherwise it
// tells the operator that N changes are waiting and how to apply them. It
// never fails the command: the save itself already succeeded.
func afterConfigWrite(ctx context.Context, client *Client, prog, name string, apply bool, out, errOut io.Writer) {
	pending, err := client.GetPendingChanges(ctx, name)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "could not check pending changes for app %q: %v\n", name, err)
		return
	}
	if !pending.Pending {
		return
	}
	if apply {
		if _, err := client.ApplyPendingChanges(ctx, name); err != nil {
			_, _ = fmt.Fprintf(errOut, "could not apply pending changes for app %q: %v\n", name, err)
			return
		}
		_, _ = fmt.Fprintf(out, "applying %d pending change(s) to app %q: restarting it (%s)\n", pendingChangeCount(pending), name, describePendingChanges(pending))
		return
	}
	_, _ = fmt.Fprintf(out, "%d changes pending (%s). Run: %s apps apply %s (or pass --apply)\n", pendingChangeCount(pending), describePendingChanges(pending), prog, name)
}

type appsApplyResult struct {
	App     string                    `json:"app"`
	Applied bool                      `json:"applied"`
	Changes []apiclient.PendingChange `json:"changes"`
}

// runAppsApply implements "apps apply <name>": restart the app so its
// pending env, secret and config changes take effect.
func runAppsApply(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps apply", "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps apply <name> [flags]\n\nRestarts the app so saved env, secret and config changes reach the running\ncontainer. Does nothing when nothing is pending.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps apply", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	pending, err := client.GetPendingChanges(ctx, name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("check pending changes for app %q: %w", name, err))
	}
	result := appsApplyResult{App: name, Changes: pending.Changes}
	if pending.Pending {
		if _, err := client.ApplyPendingChanges(ctx, name); err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("apply pending changes for app %q: %w", name, err))
		}
		result.Applied = true
	}
	return writeScheduledTaskResult(stdout, stderr, of, result, func() {
		if !result.Applied {
			_, _ = fmt.Fprintf(stdout, "nothing pending for app %q\n", name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "applying %d pending change(s) to app %q: restarting it (%s)\n", pendingChangeCount(pending), name, describePendingChanges(pending))
	})
}
