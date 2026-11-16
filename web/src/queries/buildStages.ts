import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import {
  normalizeBuildStages,
  stagesPollInterval,
  type BuildStageName,
  type BuildStagesResponse,
} from '../lib/buildStages'
import { appKeys } from './apps'

export const buildStageKeys = {
  detail: (appName: string, deployId: string) =>
    [...appKeys.detail(appName), 'deploy-stages', deployId] as const,
}

export const BUILD_STAGES_POLL_MS = Number(
  import.meta.env.VITE_BUILD_STAGES_POLL_MS ?? 3000,
)

export async function fetchBuildStages(
  appName: string,
  deployId: string,
): Promise<BuildStagesResponse> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploys/${encodeURIComponent(deployId)}/stages`,
  )
  // A deploy from before stage recording has no stages, not an error.
  if (res.status === 404) return normalizeBuildStages(null)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch build stages failed: ${res.status}`),
    )
  }
  return normalizeBuildStages(
    (await res.json()) as Partial<BuildStagesResponse> | null,
  )
}

export function buildStagesQueryOptions(appName: string, deployId: string) {
  return queryOptions({
    queryKey: buildStageKeys.detail(appName, deployId),
    queryFn: () => fetchBuildStages(appName, deployId),
    refetchInterval: (query) =>
      stagesPollInterval(query.state.data, BUILD_STAGES_POLL_MS),
  })
}

export function buildStageLogUrl(
  appName: string,
  deployId: string,
  stage: BuildStageName,
): string {
  const url = new URL(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploys/${encodeURIComponent(deployId)}/logs`,
    window.location.origin,
  )
  url.searchParams.set('stage', stage)
  return url.toString()
}
