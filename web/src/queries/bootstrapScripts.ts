import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface BootstrapScript {
  id: string
  name: string
  body?: string
  size_bytes: number
  created_at: string
  updated_at: string
}

export interface BootstrapLimits {
  max_bytes: number
  max_count: number
}

const BASE = '/api/v1/bootstrap-scripts'
const KEY = ['bootstrap-scripts'] as const

async function call(url: string, what: string, init?: RequestInit) {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return res
}

export function bootstrapScriptsQueryOptions() {
  return queryOptions({
    queryKey: KEY,
    queryFn: async (): Promise<BootstrapScript[]> => {
      const res = await call(BASE, 'fetch bootstrap scripts')
      return (await res.json()) as BootstrapScript[]
    },
  })
}

export function bootstrapLimitsQueryOptions() {
  return queryOptions({
    queryKey: [...KEY, 'limits'] as const,
    queryFn: async (): Promise<BootstrapLimits> => {
      const res = await call(`${BASE}/limits`, 'fetch bootstrap limits')
      return (await res.json()) as BootstrapLimits
    },
  })
}

export async function fetchBootstrapScript(
  id: string,
): Promise<BootstrapScript> {
  const res = await call(`${BASE}/${encodeURIComponent(id)}`, 'fetch script')
  return (await res.json()) as BootstrapScript
}

export interface SaveScriptVars {
  id?: string
  name: string
  body: string
}

export function useSaveBootstrapScript() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, name, body }: SaveScriptVars) => {
      await call(
        id ? `${BASE}/${encodeURIComponent(id)}` : BASE,
        'save script',
        {
          method: id ? 'PUT' : 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name, body }),
        },
      )
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}

export function useDeleteBootstrapScript() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (id: string) => {
      await call(`${BASE}/${encodeURIComponent(id)}`, 'delete script', {
        method: 'DELETE',
      })
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: KEY }),
  })
}
