package main

import (
	"context"
	"flag"
	"fmt"
	"io"
)

// runNodesPatchStatus implements "nodes patch-status <id>": GET
// /api/v1/nodes/{id}/patch-status, the latest OS-patch reading
// internal/telemetry.HostPatchCollector wrote for this node.
func runNodesPatchStatus(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP := apiFlagSet(prog, "nodes patch-status", "print patch status as JSON to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesPatchStatusUsage(prog)) }

	if err := fs.Parse(reorderArgsFlagsFirst(fs, args)); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut := *tokenFlagP, *apiURLFlagP, *profileFlagP, *jsonOutP

	id, ok := requireOneArg(fs, stderr, prog, "nodes patch-status", "node id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	status, err := client.GetNodePatchStatus(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get patch status for node %q: %w", id, err))
	}

	if jsonOut {
		if err := writeJSONValue(stdout, status); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintln(stdout, formatPatchStatusHuman(status))
	return exitOK
}

// formatPatchStatusHuman renders a nodePatchStatusResource the way an
// operator reads it at a glance: "unknown, not checked" when
// HostPatchCollector has never written a sample for this node (no
// supported package manager, or the collector hasn't run yet), "up to
// date" for a checked, genuinely empty count, otherwise the total with a
// security count called out separately since that's the number an
// operator actually needs to act on urgently.
func formatPatchStatusHuman(status nodePatchStatusResource) string {
	if !status.Checked {
		return "unknown, not checked"
	}
	if status.Total == 0 {
		return "up to date"
	}
	plural := "s"
	if status.Total == 1 {
		plural = ""
	}
	if status.Security == 0 {
		return fmt.Sprintf("%d update%s available", status.Total, plural)
	}
	return fmt.Sprintf("%d update%s available, %d security", status.Total, plural, status.Security)
}

func nodesPatchStatusUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes patch-status <id> [flags]

Shows a node's latest available-OS-package-updates reading (e.g. "3
updates available, 1 security", "up to date", or "unknown, not checked"
if the collector hasn't run or found no supported package manager).

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print patch status as JSON to stdout, nothing else
  -h, --help              show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
