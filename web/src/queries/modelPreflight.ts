// Query keys, fetchers and mutations for POST /api/v1/models/preflight and
// /api/v1/model-cache (internal/api/models_hf.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  CachePruneResult,
  CacheReport,
  PreflightRequest,
  PreflightResult,
} from '../types/modelPreflight'
import { requestJson } from './models'

const PREFLIGHT_STALE_MS = 60_000

export const preflightKeys = {
  all: ['model-preflight'] as const,
  check: (
    repo: string,
    engine: string,
    quant: string,
    node: string,
    tokenFingerprint: string,
  ) =>
    [
      ...preflightKeys.all,
      repo,
      engine,
      quant,
      node,
      tokenFingerprint,
    ] as const,
}

export const cacheKeys = {
  all: ['model-cache'] as const,
  report: () => [...cacheKeys.all, 'report'] as const,
}

export function fetchPreflight(
  req: PreflightRequest,
  signal?: AbortSignal,
): Promise<PreflightResult> {
  return requestJson<PreflightResult>(
    '/api/v1/models/preflight',
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
      signal,
    },
    'check repository',
  )
}

export function usePreflight(
  req: PreflightRequest | null,
  tokenFingerprint: string,
) {
  return useQuery({
    queryKey: preflightKeys.check(
      req?.repo ?? '',
      req?.engine ?? '',
      req?.quant ?? '',
      req?.node_id ?? '',
      tokenFingerprint,
    ),
    queryFn: ({ signal }) => {
      if (!req) throw new Error('no repository to check')
      return fetchPreflight(req, signal)
    },
    enabled: req !== null,
    staleTime: PREFLIGHT_STALE_MS,
    retry: false,
  })
}

export function useModelCache() {
  return useQuery({
    queryKey: cacheKeys.report(),
    queryFn: () =>
      requestJson<CacheReport>(
        '/api/v1/model-cache',
        undefined,
        'fetch model cache',
      ),
  })
}

export function usePruneModelCache() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (vars: { dryRun: boolean; volumes?: string[] }) =>
      requestJson<CachePruneResult>(
        '/api/v1/model-cache/prune',
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            dry_run: vars.dryRun,
            volumes: vars.volumes ?? [],
          }),
        },
        'prune model cache',
      ),
    onSuccess: (result) => {
      if (!result.dry_run) {
        void queryClient.invalidateQueries({ queryKey: cacheKeys.all })
      }
    },
  })
}
