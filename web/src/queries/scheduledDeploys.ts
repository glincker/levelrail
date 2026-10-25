// /api/v1/apps/{name}/scheduled-deploys (internal/api/scheduled_deploys.go):
// one-shot image or git ref deploys that run once at a set time.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type ScheduledDeployStatus =
  | 'pending'
  | 'running'
  | 'fired'
  | 'pending_approval'
  | 'skipped_freeze'
  | 'missed'
  | 'canceled'
  | 'failed'

export interface ScheduledDeploy {
  id: string
  service_name: string
  kind: 'image' | 'git'
  image?: string
  ref?: string
  run_at: string
  status: ScheduledDeployStatus
  reason?: string
  approval_id?: string
  created_by?: string
  created_at: string
  finished_at?: string
}

export interface ScheduledDeployInput {
  run_at: string
  image?: string
  ref?: string
}

export const scheduledDeployKeys = {
  list: (appName: string) =>
    [...appKeys.detail(appName), 'scheduled-deploys'] as const,
}

function baseURL(appName: string) {
  return `/api/v1/apps/${encodeURIComponent(appName)}/scheduled-deploys`
}

async function fail(res: Response, what: string): Promise<never> {
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function useScheduledDeploys(appName: string) {
  return useQuery({
    queryKey: scheduledDeployKeys.list(appName),
    queryFn: async (): Promise<ScheduledDeploy[]> => {
      const res = await fetch(baseURL(appName))
      if (!res.ok) return fail(res, 'fetch scheduled deploys')
      return (await res.json()) as ScheduledDeploy[]
    },
    retry: false,
  })
}

export function useCreateScheduledDeploy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<ScheduledDeploy, ApiError, ScheduledDeployInput>({
    mutationFn: async (input) => {
      const res = await fetch(baseURL(appName), {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Idempotency-Key': crypto.randomUUID(),
        },
        body: JSON.stringify(input),
      })
      if (!res.ok) return fail(res, 'schedule deploy')
      return (await res.json()) as ScheduledDeploy
    },
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: scheduledDeployKeys.list(appName),
      }),
  })
}

export function useCancelScheduledDeploy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<ScheduledDeploy, ApiError, string>({
    mutationFn: async (id) => {
      const res = await fetch(`${baseURL(appName)}/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      })
      if (!res.ok) return fail(res, 'cancel scheduled deploy')
      return (await res.json()) as ScheduledDeploy
    },
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: scheduledDeployKeys.list(appName),
      }),
  })
}
