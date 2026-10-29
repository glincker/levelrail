// GET/PUT /api/v1/apps/{name}/predeploy-snapshot, GET .../predeploy-snapshots and
// POST .../deploys/{id}/rollback-with-data (internal/api/predeploy_snapshot.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type {
  PredeploySnapshot,
  PredeploySnapshotSettings,
  RollbackWithDataResult,
} from '../types/predeploySnapshot'
import { appKeys } from './apps'
import { deployAttemptKeys } from './deployAttempts'
import { postJson } from './deployControl'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const predeploySnapshotKeys = {
  settings: (appName: string) =>
    [...appKeys.detail(appName), 'predeploy-snapshot'] as const,
  list: (appName: string) =>
    [...appKeys.detail(appName), 'predeploy-snapshots'] as const,
}

function base(appName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}`
}

async function getJson<T>(url: string, what: string): Promise<T> {
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function predeploySettingsQueryOptions(appName: string) {
  return queryOptions({
    queryKey: predeploySnapshotKeys.settings(appName),
    queryFn: () =>
      getJson<PredeploySnapshotSettings>(
        `${base(appName)}/predeploy-snapshot`,
        'fetch pre-deploy snapshot setting',
      ),
    retry: false,
  })
}

export function usePredeploySettings(appName: string) {
  return useQuery(predeploySettingsQueryOptions(appName))
}

export function useSetPredeploySettings(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<
    PredeploySnapshotSettings,
    ApiError,
    PredeploySnapshotSettings
  >({
    mutationFn: async (settings) => {
      const res = await fetch(`${base(appName)}/predeploy-snapshot`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(settings),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `set pre-deploy snapshot setting failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as PredeploySnapshotSettings
    },
    onSuccess: (result) => {
      queryClient.setQueryData(predeploySnapshotKeys.settings(appName), result)
    },
  })
}

export function usePredeploySnapshots(appName: string, enabled = true) {
  return useQuery({
    queryKey: predeploySnapshotKeys.list(appName),
    queryFn: () =>
      getJson<PredeploySnapshot[]>(
        `${base(appName)}/predeploy-snapshots`,
        'fetch pre-deploy snapshots',
      ),
    enabled,
    retry: false,
  })
}

export function useRollbackWithData(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<RollbackWithDataResult, ApiError, string>({
    mutationFn: (deployId) =>
      postJson<RollbackWithDataResult>(
        `${base(appName)}/deploys/${encodeURIComponent(deployId)}/rollback-with-data`,
        {},
        'roll back with data',
      ),
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: deployAttemptKeys.list(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: predeploySnapshotKeys.list(appName),
      })
    },
  })
}
