// Query-key factory and fetcher for GET /api/v1/apps/{name}/diagnose
// (internal/api/diagnose.go's handleDiagnoseApp). Unlike
// queries/deploys.ts and queries/deployAttempts.ts, this is fetched
// lazily (plain useQuery with `enabled`, not useSuspenseQuery): most
// apps most of the time have nothing to diagnose, so this should never
// fire on a normal page load, only when DiagnosisPanel actually renders
// (a failed attempt or a crashlooping app) or an operator expands it.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import type { Diagnosis, DiagnosisFix } from '../types/diagnosis'
import { patchApp } from '../lib/diagnosisFix'
import { appKeys, fetchApp, updateApp } from './apps'
import { applyTriggerDeployResult, triggerDeploy } from './deploys'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const diagnosisKeys = {
  detail: (appName: string, deployId?: string) =>
    [...appKeys.detail(appName), 'diagnosis', deployId ?? 'latest'] as const,
}

export async function fetchDiagnosis(
  appName: string,
  deployId?: string,
): Promise<Diagnosis> {
  let url = `/api/v1/apps/${encodeURIComponent(appName)}/diagnose`
  if (deployId) {
    url += `?deploy_id=${encodeURIComponent(deployId)}`
  }
  const res = await fetch(url)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch diagnosis failed: ${res.status}`),
    )
  }
  return (await res.json()) as Diagnosis
}

export function diagnosisQueryOptions(appName: string, deployId?: string) {
  return queryOptions({
    queryKey: diagnosisKeys.detail(appName, deployId),
    queryFn: () => fetchDiagnosis(appName, deployId),
  })
}

export function useDiagnosis(
  appName: string,
  deployId: string | undefined,
  enabled: boolean,
) {
  return useQuery({ ...diagnosisQueryOptions(appName, deployId), enabled })
}

export interface ApplyDiagnosisFixInput {
  fix: DiagnosisFix
  inputs: Record<string, string>
  redeploy: boolean
}

// Applies a fix client side through the ordinary app update path: fetch the
// current app, patch the fields the fix names, PUT it back with the caller's
// own permissions, then optionally redeploy.
export function useApplyDiagnosisFix(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ fix, inputs, redeploy }: ApplyDiagnosisFixInput) => {
      const current = await fetchApp(appName)
      const saved = await updateApp(patchApp(current, fix.changes, inputs))
      queryClient.setQueryData(appKeys.detail(appName), saved)
      if (redeploy) {
        const result = await triggerDeploy(appName, { image: saved.image })
        applyTriggerDeployResult(queryClient, appName, result)
      }
      return saved
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: [...appKeys.detail(appName), 'diagnosis'],
      })
    },
  })
}
