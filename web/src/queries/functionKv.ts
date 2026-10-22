// /api/v1/functions/{name}/kv and /functions/starters (internal/api/function_kv.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface KVKey {
  key: string
  size: number
  preview?: string
  expires_at?: string
  updated_at: string
}

export interface KVList {
  enabled: boolean
  keys: KVKey[]
  count: number
  bytes: number
  limits: {
    max_key_bytes: number
    max_value_bytes: number
    max_keys: number
    max_total_bytes: number
    max_ttl_seconds: number
  }
}

export interface KVValue {
  key: string
  value: string
  size: number
  expires_at?: string
}

export interface FunctionStarter {
  id: string
  preset: string
  name: string
  runtime: string
  uses_kv: boolean
  description: string
  env: { name: string; required: boolean; description: string }[]
  files: { path: string; content: string }[]
}

export const kvKeys = {
  list: (name: string, prefix: string) =>
    ['functions', name, 'kv', prefix] as const,
  all: (name: string) => ['functions', name, 'kv'] as const,
  starters: ['functions', 'starters'] as const,
}

const base = (name: string) =>
  `/api/v1/functions/${encodeURIComponent(name)}/kv`

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(res.status, await readErrorMessage(res, what))
}

export function useFunctionKV(name: string, prefix: string) {
  return useQuery({
    queryKey: kvKeys.list(name, prefix),
    queryFn: async () => {
      const res = await fetch(
        `${base(name)}?prefix=${encodeURIComponent(prefix)}`,
      )
      if (!res.ok) throw await failure(res, 'list keys')
      return (await res.json()) as KVList
    },
  })
}

export async function fetchKVValue(
  name: string,
  key: string,
): Promise<KVValue> {
  const res = await fetch(`${base(name)}/value?key=${encodeURIComponent(key)}`)
  if (!res.ok) throw await failure(res, 'read key')
  return (await res.json()) as KVValue
}

export function usePutKV(name: string) {
  const qc = useQueryClient()
  return useMutation<
    void,
    ApiError,
    { key: string; value: string; ttl_seconds: number }
  >({
    mutationFn: async (input) => {
      const res = await fetch(`${base(name)}/value`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!res.ok) throw await failure(res, 'save key')
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: kvKeys.all(name) }),
  })
}

export function useDeleteKV(name: string) {
  const qc = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: async (key) => {
      const res = await fetch(
        `${base(name)}/value?key=${encodeURIComponent(key)}`,
        {
          method: 'DELETE',
        },
      )
      if (!res.ok) throw await failure(res, 'delete key')
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: kvKeys.all(name) }),
  })
}

export function useRotateKVToken(name: string) {
  const qc = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(`${base(name)}/token`, { method: 'POST' })
      if (!res.ok) throw await failure(res, 'issue token')
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: kvKeys.all(name) }),
  })
}

export function useFunctionStarters() {
  return useQuery({
    queryKey: kvKeys.starters,
    queryFn: async () => {
      const res = await fetch('/api/v1/functions/starters')
      if (!res.ok) throw await failure(res, 'fetch starters')
      const body = (await res.json()) as { starters?: FunctionStarter[] }
      return body.starters ?? []
    },
    staleTime: Infinity,
    retry: false,
  })
}
