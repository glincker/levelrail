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

export function deployPhasesQueryOptions(deployId: string, running: boolean) {
  return queryOptions({
    queryKey: deployPhasesKeys.one(deployId),
    queryFn: () => fetchDeployPhases(deployId),
    refetchInterval: running ? POLL_MS : false,
    retry: false,
  })
}
