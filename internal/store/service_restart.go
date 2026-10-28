package store

import "strings"

// Compose restart: values that opt a service into running to completion.
const (
	RestartPolicyNo        = "no"
	RestartPolicyOnFailure = "on-failure"
)

// IsRunToCompletionRestart reports whether a compose restart: value is "no" or
// "on-failure[:N]". Only an explicit value counts: an unset one keeps the service running.
func IsRunToCompletionRestart(policy string) bool {
	return policy == RestartPolicyNo || policy == RestartPolicyOnFailure || strings.HasPrefix(policy, RestartPolicyOnFailure+":")
}

// RunsToCompletion reports a service whose exit code 0 means done, not crashed.
func (s DesiredService) RunsToCompletion() bool {
	return s.Health != nil && IsRunToCompletionRestart(s.Health.Restart)
}
