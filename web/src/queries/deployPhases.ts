// Fetcher for GET /api/v1/deployments/{id}/phases (internal/api/deploy_phases.go):
// stages of one deploy with timing and an ETA, polled only while it runs.
import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type DeployPhaseKey =
  'upload' | 'install' | 'build' | 'export_image' | 'health_check' | 'cutover'

export type DeployPhaseStatus =
  'pending' | 'running' | 'done' | 'failed' | 'skipped'

export interface DeployPhase {
  phase: DeployPhaseKey
  status: DeployPhaseStatus
  started_at: string | null
  finished_at: string | null
  duration_ms: number | null
  typical_ms: number | null
}

export interface DeployPhases {
  deploy_id: string
  status: string
  elapsed_ms: number
  phases: DeployPhase[]
  eta_ms: number | null
  typical_total_ms: number | null
  samples: number
}

const POLL_MS =
  Number(import.meta.env.VITE_DEPLOY_PHASES_POLL_SECONDS ?? 5) * 1000

// Safety net while the log stream is healthy: health and cutover come from trace rows, not log lines.
const STREAM_POLL_MS =
  Number(import.meta.env.VITE_DEPLOY_PHASES_STREAM_POLL_SECONDS ?? 30) * 1000

const INSTALL_RE =
  /\b(npm (ci|install|i)|yarn install|pnpm (i|install)|pip3? install|bundle install|go mod download|composer install|poetry install)\b/i
const BUILD_RE =
  /\b(next build|npm run build|yarn build|pnpm( run)? build|go build|cargo build|vite build|gradle|mvn)\b|creating an optimized production build/i
const EXPORT_RE =
  /exporting to (image|oci|docker)|exporting layers|writing image|naming to/i

/** phaseMarkerStage ranks a log line: 0 none, 1 install, 2 build, 3 export. */
export function phaseMarkerStage(line: string): number {
  if (EXPORT_RE.test(line)) return 3
  if (BUILD_RE.test(line)) return 2
  if (INSTALL_RE.test(line)) return 1
  return 0
}

export const deployPhasesKeys = {
  one: (deployId: string) => ['deploy-phases', deployId] as const,
}

export async function fetchDeployPhases(
  deployId: string,
): Promise<DeployPhases> {
  const res = await fetch(
    `/api/v1/deployments/${encodeURIComponent(deployId)}/phases`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch deploy phases failed: ${res.status}`),
    )
  }
  return (await res.json()) as DeployPhases
}

export function deployPhasesQueryOptions(
  deployId: string,
  running: boolean,
  streamHealthy = false,
) {
  const interval = streamHealthy ? Math.max(POLL_MS, STREAM_POLL_MS) : POLL_MS
  return queryOptions({
    queryKey: deployPhasesKeys.one(deployId),
    queryFn: () => fetchDeployPhases(deployId),
    refetchInterval: running ? interval : false,
    retry: false,
  })
}
