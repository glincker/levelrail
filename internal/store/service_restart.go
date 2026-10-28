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

// DependsOnCompleted lists the dependencies that must exit 0 before this service starts.
func (s DesiredService) DependsOnCompleted() []string {
	if s.Health == nil {
		return nil
	}
	return s.Health.DependsOnCompleted
}

// KeepComposeHealth returns next with prev's compose-only fields (Restart,
// DependsOnCompleted) where next left them unset. Writers that cannot express
// those fields use it so a probe edit or rollback never reruns a finished one-shot.
func KeepComposeHealth(next, prev *ServiceHealth) *ServiceHealth {
	if prev == nil || (prev.Restart == "" && len(prev.DependsOnCompleted) == 0) {
		return next
	}
	out := ServiceHealth{}
	if next != nil {
		out = *next
	}
	if out.Restart == "" {
		out.Restart = prev.Restart
	}
	if len(out.DependsOnCompleted) == 0 {
		out.DependsOnCompleted = prev.DependsOnCompleted
	}
	return &out
}
