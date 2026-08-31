import type { ServiceHealth } from '../types/appDetail'

// Matches docs/app-spec-reference.md's own top-of-file example
// (readiness: 5s interval / 2s timeout, liveness: 30s interval / 3
// failures) and the CLI's interactive wizard default
// (cmd/levelrail-cli/apps_create_interactive.go), so a health check
// enabled through either creation form behaves identically to one
// hand-written in app.yaml.
export const HEALTH_CHECK_DEFAULT_PATH = '/healthz'

const NS_PER_SECOND = 1_000_000_000
const READINESS_INTERVAL_NS = 5 * NS_PER_SECOND
const READINESS_TIMEOUT_NS = 2 * NS_PER_SECOND
const LIVENESS_INTERVAL_NS = 30 * NS_PER_SECOND
const LIVENESS_FAILURES = 3

export function healthCheckFrom(
  enabled: boolean,
  path: string,
): ServiceHealth | undefined {
  if (!enabled) return undefined
  const trimmed = path.trim()
  return {
    readiness: {
      path: trimmed,
      interval: READINESS_INTERVAL_NS,
      timeout: READINESS_TIMEOUT_NS,
    },
    liveness: {
      path: trimmed,
      interval: LIVENESS_INTERVAL_NS,
      failures: LIVENESS_FAILURES,
    },
  }
}
