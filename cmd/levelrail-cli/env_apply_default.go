package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

const envApplyOnSetKey = "apps.env_apply_on_set"

// resolveEnvApply decides whether an env write restarts the app. An explicit
// --apply or --no-apply wins; otherwise the runtime setting decides. A failed
// lookup keeps the safe default (no restart).
func resolveEnvApply(ctx context.Context, client *Client, apply, noApply bool) (bool, string) {
	if noApply {
		return false, ""
	}
	if apply {
		return true, ""
	}
	res, err := client.GetRuntimeSetting(ctx, envApplyOnSetKey)
	if err != nil {
		return false, ""
	}
	if v, ok := res.Value.(string); ok && strings.EqualFold(v, "true") {
		return true, "env_apply_on_set"
	}
	if v, ok := res.Value.(bool); ok && v {
		return true, "env_apply_on_set"
	}
	return false, ""
}

// envApplyConflict reports the --apply with --no-apply mistake.
func envApplyConflict(stderr io.Writer, prog string, apply, noApply bool) bool {
	if apply && noApply {
		_, _ = fmt.Fprintf(stderr, "%s: --apply and --no-apply cannot be used together\n", prog)
		return true
	}
	return false
}
