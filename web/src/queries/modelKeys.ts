// Query keys, fetchers and mutations for /api/v1/models/{name}/keys and
// /usage (internal/api/models_keys.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  CreateModelKeyRequest,
  CreatedModelKey,
  ModelKey,
  ModelUsageReport,
} from '../types/models'
import { modelKeys, requestJson, requestVoid } from './models'

const USAGE_REFETCH_MS = 30000

export const modelKeyKeys = {
  keys: (name: string) => [...modelKeys.all, 'keys', name] as const,
  usage: (name: string, hours: number) =>
    [...modelKeys.all, 'usage', name, hours] as const,
}

function keysUrl(name: string): string {
  return `/api/v1/models/${encodeURIComponent(name)}/keys`
}

export function useModelKeys(name: string) {
  return useQuery({
    queryKey: modelKeyKeys.keys(name),
    queryFn: () =>
      requestJson<ModelKey[]>(keysUrl(name), undefined, 'fetch model keys'),
    refetchInterval: USAGE_REFETCH_MS,
  })
}

export function useModelUsage(name: string, hours: number) {
  return useQuery({
    queryKey: modelKeyKeys.usage(name, hours),
    queryFn: () =>
      requestJson<ModelUsageReport>(
        `/api/v1/models/${encodeURIComponent(name)}/usage?since=${String(hours)}h`,
        undefined,
        'fetch model usage',
      ),
    refetchInterval: USAGE_REFETCH_MS,
  })
}

function useInvalidateKeys(name: string) {
  const queryClient = useQueryClient()
  return () => {
    void queryClient.invalidateQueries({ queryKey: modelKeyKeys.keys(name) })
  }
}

export function useCreateModelKey(name: string) {
  const invalidate = useInvalidateKeys(name)
  return useMutation({
    mutationFn: (req: CreateModelKeyRequest) =>
      requestJson<CreatedModelKey>(
        keysUrl(name),
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(req),
        },
        'create model key',
      ),
    onSuccess: invalidate,
  })
}

export function useRevokeModelKey(name: string) {
  const invalidate = useInvalidateKeys(name)
  return useMutation({
    mutationFn: (id: string) =>
      requestVoid(
        `${keysUrl(name)}/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'revoke model key',
      ),
    onSuccess: invalidate,
  })
}

export function useRotateModelKey(name: string) {
  const invalidate = useInvalidateKeys(name)
  return useMutation({
    mutationFn: (args: { id: string; graceSeconds?: number }) =>
      requestJson<CreatedModelKey>(
        `${keysUrl(name)}/${encodeURIComponent(args.id)}/rotate`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(
            args.graceSeconds === undefined
              ? {}
              : { grace_seconds: args.graceSeconds },
          ),
        },
        'rotate model key',
      ),
    onSuccess: invalidate,
  })
}
