package spec

import (
	"fmt"
	"strings"
)

// Restart policies a compose file or app.yaml may declare.
const (
	RestartPolicyNo            = "no"
	RestartPolicyOnFailure     = "on-failure"
	RestartPolicyAlways        = "always"
	RestartPolicyUnlessStopped = "unless-stopped"
)

// ParseRestartPolicy normalizes a restart: value (case, spacing, quotes) and rejects
// anything but no, always, unless-stopped, on-failure or on-failure:N with N a
// non-negative integer. N is accepted and ignored: nonzero exits always restart with crash backoff.
func ParseRestartPolicy(raw string) (string, error) {
	policy := strings.ToLower(strings.Trim(strings.TrimSpace(raw), `"'`))
	switch policy {
	case "", RestartPolicyNo, RestartPolicyOnFailure, RestartPolicyAlways, RestartPolicyUnlessStopped:
		return policy, nil
	}
	n, ok := strings.CutPrefix(policy, RestartPolicyOnFailure+":")
	if ok && n != "" && strings.Trim(n, "0123456789") == "" {
		return policy, nil
	}
	return "", fmt.Errorf("restart %q: want no, always, unless-stopped, on-failure or on-failure:N with N a non-negative integer", raw)
}

// IsRunToCompletionRestart reports whether a normalized restart value is "no" or "on-failure[:N]".
func IsRunToCompletionRestart(policy string) bool {
	return policy == RestartPolicyNo || policy == RestartPolicyOnFailure || strings.HasPrefix(policy, RestartPolicyOnFailure+":")
}
