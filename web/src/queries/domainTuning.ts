// GET/PUT/DELETE /api/v1/apps/{name}/domains/{domain}/tuning
// (internal/api/domain_tuning.go): per-domain proxy tuning presets.
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type TuningPreset = 'api' | 'uploads' | 'streaming' | 'websocket'

export interface DomainTuningInput {
  preset: TuningPreset | 'custom' | ''
  compression: boolean
  streaming: boolean
  websocket: boolean
  dial_timeout_seconds: number
  response_header_timeout_seconds: number
  stream_timeout_seconds: number
  stream_close_delay_seconds: number
}

export interface DomainTuning extends DomainTuningInput {
  domain: string
  configured: boolean
  effective: boolean
  min_timeout_seconds: number
  max_timeout_seconds: number
  max_stream_seconds: number
  updated_at?: string
}

export const domainTuningKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'tuning'] as const,
}

function path(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/tuning`
}

async function send(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<DomainTuning> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as DomainTuning
}

export function domainTuningQueryOptions(appName: string, domain: string) {
  return queryOptions({
    queryKey: domainTuningKeys.detail(appName, domain),
    queryFn: () => send(path(appName, domain), undefined, 'fetch tuning'),
    enabled: domain.length > 0,
    staleTime: 30_000,
  })
}

export function useDomainTuning(appName: string, domain: string) {
  return useQuery(domainTuningQueryOptions(appName, domain))
}

export function useSetDomainTuning(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainTuning, ApiError, DomainTuningInput>({
    mutationFn: (input) =>
      send(
        path(appName, domain),
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(input),
        },
        'update tuning',
      ),
    onSuccess: (updated) => {
      queryClient.setQueryData(
        domainTuningKeys.detail(appName, domain),
        updated,
      )
    },
  })
}

export function useResetDomainTuning(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(path(appName, domain), { method: 'DELETE' })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `reset tuning failed: ${res.status}`),
        )
      }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({
        queryKey: domainTuningKeys.detail(appName, domain),
      }),
  })
}
