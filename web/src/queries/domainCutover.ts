// Test-before-you-switch checklist (internal/api/domain_cutover.go).
import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { domainCertKeys } from './domainCert'

export type CutoverState = 'pass' | 'wait' | 'fail' | 'skip'
export type CutoverRisk = 'none' | 'likely' | 'unknown'
export type CutoverStepId =
  | 'app'
  | 'certificate'
  | 'current_dns'
  | 'switch_dns'
  | 'authoritative'
  | 'live'

export interface CutoverAction {
  kind: string
  label: string
  api?: string
  href?: string
  cli?: string
}

export interface CutoverStep {
  id: CutoverStepId
  state: CutoverState
  title: string
  detail: string
  eta_seconds?: number
  action?: CutoverAction
}

export interface CutoverRecord {
  type: string
  name: string
  value: string
  ttl_seconds: number
}

export interface DomainCutover {
  domain: string
  risk: CutoverRisk
  ready_to_switch: boolean
  summary: string
  proxied: boolean
  proxy_docs?: string
  steps: CutoverStep[]
  dns: {
    points: 'none' | 'this_server' | 'previous_server' | 'cloudflare_proxy'
    status: string
    resolved?: string[]
    ttl_seconds?: number
    lower_ttl_to: number
    records?: CutoverRecord[]
  }
  certificate: {
    state: string
    error?: string
    explanation?: string
    provider_connected: boolean
    runbook_href: string
  }
  cli: string[]
  guide_href: string
}

const POLL_MS = 10_000

export const domainCutoverKeys = {
  one: (domain: string) => ['domains', domain, 'cutover'] as const,
}

export function domainCutoverQueryOptions(domain: string) {
  return queryOptions({
    queryKey: domainCutoverKeys.one(domain),
    queryFn: async (): Promise<DomainCutover> => {
      const res = await fetch(
        `/api/v1/domains/${encodeURIComponent(domain)}/cutover`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `cutover failed: ${res.status}`),
        )
      }
      return (await res.json()) as DomainCutover
    },
    refetchInterval: POLL_MS,
  })
}

export function useCutoverCertificate(domain: string) {
  const queryClient = useQueryClient()
  return useMutation<unknown, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(
        `/api/v1/domains/${encodeURIComponent(domain)}/cutover/certificate`,
        { method: 'POST' },
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `certificate failed: ${res.status}`),
        )
      }
      return (await res.json()) as unknown
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: domainCutoverKeys.one(domain),
      })
      void queryClient.invalidateQueries({
        queryKey: domainCertKeys.status(domain),
      })
    },
  })
}
