// GET/PUT /api/v1/functions/{name}/settings and GET .../versions (internal/api/functions_controls.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { functionKeys } from './functions'

export interface FunctionSettings {
  name: string
  port: number
  idle_minutes: number
  memory_bytes: number
  nano_cpus: number
  request_timeout_seconds: number
  max_concurrency: number
  effective_timeout_seconds: number
  effective_concurrency: number
  bounds: {
    default_timeout_seconds: number
    max_timeout_seconds: number
    default_concurrency: number
    max_concurrency: number
  }
}

export interface FunctionSettingsUpdate {
  request_timeout_seconds?: number
  max_concurrency?: number
}

export interface FunctionVersion {
  deploy_id: string
  image: string
  commit_sha?: string
  status: string
  started_at: string
  live: boolean
  rollbackable: boolean
}

export const functionControlKeys = {
  settings: (name: string) => [...functionKeys.all, 'settings', name] as const,
  versions: (name: string) => [...functionKeys.all, 'versions', name] as const,
}

async function request<T>(
  path: string,
  what: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

const base = (name: string) => `/api/v1/functions/${encodeURIComponent(name)}`

export function useFunctionSettings(name: string) {
  return useQuery({
    queryKey: functionControlKeys.settings(name),
    queryFn: () =>
      request<FunctionSettings>(`${base(name)}/settings`, 'fetch settings'),
  })
}

export function useUpdateFunctionSettings(name: string) {
  const qc = useQueryClient()
  return useMutation<FunctionSettings, ApiError, FunctionSettingsUpdate>({
    mutationFn: (body) =>
      request<FunctionSettings>(`${base(name)}/settings`, 'save settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      }),
    onSuccess: (data) => {
      qc.setQueryData(functionControlKeys.settings(name), data)
    },
  })
}

export function useFunctionVersions(name: string) {
  return useQuery({
    queryKey: functionControlKeys.versions(name),
    queryFn: async () =>
      (
        await request<{ versions: FunctionVersion[] }>(
          `${base(name)}/versions`,
          'fetch versions',
        )
      ).versions,
  })
}
