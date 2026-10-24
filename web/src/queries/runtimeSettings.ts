// Queries and mutations for /api/v1/settings/runtime.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { beginRestart } from '../lib/restartTracker'
import type { SystemRestartInfo } from './systemRestart'
import type {
  RuntimeOnboardingResponse,
  RuntimeSetting,
  RuntimeSettingsResponse,
  RuntimeSettingWrite,
} from '../types/runtimeSettings'

const BASE = '/api/v1/settings/runtime'

export const runtimeSettingsKeys = {
  all: ['runtime-settings'] as const,
  list: () => [...runtimeSettingsKeys.all, 'list'] as const,
  onboarding: () => [...runtimeSettingsKeys.all, 'onboarding'] as const,
}

async function json<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function send(method: string, body?: unknown): RequestInit {
  if (body === undefined) return { method }
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function runtimeSettingsQueryOptions() {
  return queryOptions({
    queryKey: runtimeSettingsKeys.list(),
    queryFn: async () =>
      json<RuntimeSettingsResponse>(
        await fetch(BASE),
        'fetch runtime settings',
      ),
    staleTime: 10_000,
  })
}

export function useRuntimeSettings() {
  return useQuery(runtimeSettingsQueryOptions())
}

export function runtimeOnboardingQueryOptions() {
  return queryOptions({
    queryKey: runtimeSettingsKeys.onboarding(),
    queryFn: async () =>
      json<RuntimeOnboardingResponse>(
        await fetch(`${BASE}/onboarding`),
        'fetch runtime settings review',
      ),
    staleTime: 10_000,
  })
}

/** Never throws into the setup guide: the panel hides itself on error. */
export function useRuntimeOnboarding() {
  return useQuery({ ...runtimeOnboardingQueryOptions(), retry: false })
}

function useInvalidate() {
  const queryClient = useQueryClient()
  return () =>
    queryClient.invalidateQueries({ queryKey: runtimeSettingsKeys.all })
}

export function useSetRuntimeSetting() {
  const invalidate = useInvalidate()
  return useMutation({
    mutationFn: async (v: {
      key: string
      value: RuntimeSettingWrite
      confirmWeakening?: boolean
    }) =>
      json<RuntimeSetting>(
        await fetch(
          `${BASE}/${encodeURIComponent(v.key)}`,
          send('PUT', {
            value: v.value,
            ...(v.confirmWeakening ? { confirm_weakening: true } : {}),
          }),
        ),
        'save setting',
      ),
    onSuccess: invalidate,
  })
}

export function useResetRuntimeSetting() {
  const invalidate = useInvalidate()
  return useMutation({
    mutationFn: async (key: string) =>
      json<RuntimeSetting>(
        await fetch(`${BASE}/${encodeURIComponent(key)}`, send('DELETE')),
        'reset setting',
      ),
    onSuccess: invalidate,
  })
}

export function useApplyRuntimeSettings() {
  return useMutation({
    mutationFn: async () =>
      json<SystemRestartInfo>(
        await fetch(`${BASE}/apply`, send('POST')),
        'restart',
      ),
    onSuccess: (info) => {
      beginRestart(info.boot_id)
    },
  })
}

export function useAckRuntimeOnboarding() {
  const invalidate = useInvalidate()
  return useMutation({
    mutationFn: async () => {
      const res = await fetch(`${BASE}/onboarding/ack`, send('POST'))
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `acknowledge failed: ${res.status}`),
        )
      }
    },
    onSuccess: invalidate,
  })
}
