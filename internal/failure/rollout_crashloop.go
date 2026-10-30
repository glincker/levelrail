package failure

import "github.com/GLINCKER/levelrail/internal/deploysafety"

// EnvRolloutCrashLoopRestarts is how many restarts a rollout that never
// served may take before its CrashLoopBackOff counts as a failed deploy.
const EnvRolloutCrashLoopRestarts = "APP_DEPLOY_ROLLOUT_CRASHLOOP_RESTARTS"

// Same bar as the post-cutover watch (deploysafety WatchMaxRestarts), so one
// restart that recovers is still a rollout in progress.
const defaultRolloutCrashLoopRestarts = 2

// RolloutCrashLoopRestarts reads EnvRolloutCrashLoopRestarts from lookup.
func RolloutCrashLoopRestarts(lookup func(string) (string, bool)) int {
	return envInt(lookup, EnvRolloutCrashLoopRestarts, defaultRolloutCrashLoopRestarts)
}

// IsRolloutCrashLoop reports a CrashLoopBackOff condition whose restart count reached limit.
func IsRolloutCrashLoop(reason, message string, limit int) bool {
	if reason != deploysafety.ReasonCrashLoopBackOff || limit <= 0 {
		return false
	}
	n, _, ok := deploysafety.ParseCrashLoop(message)
	return ok && n >= limit
}
