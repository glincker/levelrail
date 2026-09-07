// Query-key factory and fetcher for GET /api/v1/apps/{name}/hook-runs
// (internal/api/apps_hooks.go's handleGetAppHookRuns): the most recent
// outcome of each of an app's pre/post-deploy hooks.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface HookRun {
  hook_type: 'pre_deploy' | 'post_deploy'
  command: string
  exit_code: number
  success: boolean
  output: string
  ran_at: string
}

export interface AppHookRuns {
  pre_deploy?: HookRun
  post_deploy?: HookRun
}

export const appHookRunsKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'hook-runs'] as const,
}

export async function fetchAppHookRuns(
  appName: string,
): Promise<AppHookRuns> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/hook-runs`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app hook runs failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppHookRuns
}

export function appHookRunsQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appHookRunsKeys.detail(appName),
    queryFn: () => fetchAppHookRuns(appName),
  })
}

// Not a suspense query, unlike useApp: this is supplementary data for
// HooksEditor's own "last run" panel, only fetched when the app actually
// has a hook configured (see enabled below), and a slow/failed fetch here
// should never block the rest of the deploy-settings page from
// rendering.
export function useAppHookRuns(appName: string, enabled: boolean) {
  return useQuery({ ...appHookRunsQueryOptions(appName), enabled })
}
