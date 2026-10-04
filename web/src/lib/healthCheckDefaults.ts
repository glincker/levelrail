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

export interface HealthPathPreset {
  id: string
  path: string
}

// Same preset ids as the CLI's "apps health set --preset" (apps_health.go)
// so the two surfaces offer identical shortcuts.
export const HEALTH_CHECK_PATH_PRESETS: HealthPathPreset[] = [
  { id: 'healthz', path: '/healthz' },
  { id: 'health', path: '/health' },
  { id: 'api-health', path: '/api/health' },
  { id: 'ping', path: '/ping' },
  { id: 'status', path: '/status' },
]

export interface ProbeTimingDefaults {
  intervalSeconds: string
  timeoutSeconds: string
  failures: string
}

// Readiness gates one deploy's cutover (short interval/timeout, no
// failure count); liveness restarts a hung container (longer interval,
// a few failures first). Seconds-string form of the constants above, for
// prefilling the editor's form fields in one click.
export function probeTimingDefaults(
  kind: 'readiness' | 'liveness',
): ProbeTimingDefaults {
  if (kind === 'liveness') {
    return {
      intervalSeconds: String(LIVENESS_INTERVAL_NS / NS_PER_SECOND),
      timeoutSeconds: '',
      failures: String(LIVENESS_FAILURES),
    }
  }
  return {
    intervalSeconds: String(READINESS_INTERVAL_NS / NS_PER_SECOND),
    timeoutSeconds: String(READINESS_TIMEOUT_NS / NS_PER_SECOND),
    failures: '',
  }
}
