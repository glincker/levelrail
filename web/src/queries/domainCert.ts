// Guided public certificate for one domain (internal/api/domain_cert_flow.go):
// preflight, request and progress.
import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { certificatesKeys } from './certificates'
import type { CertKind } from './certificates'

export type PreflightState = 'pass' | 'warn' | 'fail' | 'skipped'

export interface PreflightItem {
  code: string
  state: PreflightState
  detail: string
  fix?: string
}

export interface CertPreflight {
  domain: string
  staging: boolean
  email: string
  ready: boolean
  items: PreflightItem[]
  next_safe_at?: string
}

export type CertFlowState = 'off' | 'pending' | 'issued' | 'failed'

export interface DomainCertStatus {
  domain: string
  state: CertFlowState
  staging: boolean
  applies_to_all: boolean
  issuer?: string
  kind: CertKind
  not_after?: string
  days_left?: number
  renewal_at?: string
  last_renewal?: { at: string; result: 'ok' | 'failed'; error?: string }
  error?: string
  explanation?: string
  hint?: string
  next_safe_at?: string
}

export interface GetCertRequest {
  email?: string
  staging: boolean
  acknowledge?: string[]
}

const PENDING_POLL_MS = 3_000

export const domainCertKeys = {
  preflight: (domain: string, email: string, staging: boolean) =>
    ['domains', domain, 'cert', 'preflight', email, staging] as const,
  status: (domain: string) => ['domains', domain, 'cert', 'status'] as const,
}

function path(domain: string, suffix: string): string {
  return `/api/v1/domains/${encodeURIComponent(domain)}/cert${suffix}`
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function certPreflightQueryOptions(
  domain: string,
  email: string,
  staging: boolean,
) {
  return queryOptions({
    queryKey: domainCertKeys.preflight(domain, email, staging),
    queryFn: async (): Promise<CertPreflight> => {
      const q = new URLSearchParams()
      if (email) q.set('email', email)
      if (staging) q.set('staging', 'true')
      const qs = q.size > 0 ? `?${q.toString()}` : ''
      const res = await fetch(path(domain, `/preflight${qs}`))
      if (!res.ok) throw await failure(res, 'certificate preflight')
      return (await res.json()) as CertPreflight
    },
    staleTime: 0,
    gcTime: 0,
  })
}

export function certStatusQueryOptions(domain: string, enabled: boolean) {
  return queryOptions({
    queryKey: domainCertKeys.status(domain),
    queryFn: async (): Promise<DomainCertStatus> => {
      const res = await fetch(path(domain, '/status'))
      if (!res.ok) throw await failure(res, 'certificate status')
      return (await res.json()) as DomainCertStatus
    },
    enabled,
    refetchInterval: (query) =>
      query.state.data?.state === 'pending' ? PENDING_POLL_MS : false,
  })
}

export function useGetCertificate(domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainCertStatus, ApiError, GetCertRequest>({
    mutationFn: async (req) => {
      const res = await fetch(path(domain, ''), {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
      if (!res.ok) throw await failure(res, 'get certificate')
      return (await res.json()) as DomainCertStatus
    },
    onSuccess: (status) => {
      queryClient.setQueryData(domainCertKeys.status(domain), status)
      void queryClient.invalidateQueries({ queryKey: certificatesKeys.all })
    },
  })
}
