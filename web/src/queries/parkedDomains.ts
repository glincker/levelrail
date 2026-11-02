// Domain lifecycle: park, move, attach and delete a parked domain.
// Routes live in internal/api/routes_parked_domains.go.
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'
import { domainKeys } from './domains'

export interface ParkedDomain {
  domain: string
  kind: 'parked'
  parked_from?: string
  note?: string
  parked_by?: string
  parked_at?: string
  // False for rows parked before settings snapshots existed.
  has_settings?: boolean
  dns?: ParkedDns
  tls?: ParkedTls
}

export interface ParkedDns {
  status: 'pointing' | 'not_pointing' | 'unknown'
  checked_at?: string
}

// An expired certificate reads as issued with a negative days_left.
export interface ParkedTls {
  state: 'issued' | 'pending' | 'none'
  not_after?: string
  days_left?: number
}

export interface DomainLifecycleResult {
  domain: string
  from?: string
  to?: string
  applied: boolean
  requires_confirmation?: boolean
  message?: string
}

export interface AttachResult extends DomainLifecycleResult {
  restored_settings?: string[]
}

export interface MoveInput {
  domain: string
  from: string
  to: string
  confirm?: boolean
}

export const parkedDomainKeys = {
  all: ['domains', 'parked'] as const,
}

const BASE = '/api/v1/domains'

async function send<T>(
  url: string,
  method: string,
  body: unknown,
  what: string,
): Promise<T> {
  const res = await fetch(url, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  // A 409 with requires_confirmation is a normal answer to a move, not a failure.
  if (res.status === 409) {
    const data = (await res
      .clone()
      .json()
      .catch(() => null)) as DomainLifecycleResult | null
    if (data?.requires_confirmation) return { ...data, applied: false } as T
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export function parkedDomainsQueryOptions() {
  return queryOptions({
    queryKey: parkedDomainKeys.all,
    queryFn: () =>
      send<ParkedDomain[]>(
        `${BASE}/parked`,
        'GET',
        undefined,
        'list parked domains',
      ),
    staleTime: 30_000,
  })
}

export function useParkedDomains() {
  return useQuery(parkedDomainsQueryOptions())
}

function useInvalidateLifecycle() {
  const qc = useQueryClient()
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: parkedDomainKeys.all }),
      qc.invalidateQueries({ queryKey: domainKeys.all }),
      qc.invalidateQueries({ queryKey: appKeys.list() }),
    ])
}

export function useParkDomain() {
  const invalidate = useInvalidateLifecycle()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (input: { app: string; domain: string; note?: string }) =>
      send<DomainLifecycleResult>(`${BASE}/park`, 'POST', input, 'park domain'),
    onSuccess: invalidate,
  })
}

export function useMoveDomain() {
  const invalidate = useInvalidateLifecycle()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (input: MoveInput) =>
      send<DomainLifecycleResult>(`${BASE}/move`, 'POST', input, 'move domain'),
    onSuccess: (result) => (result.applied ? invalidate() : undefined),
  })
}

export function useAttachParkedDomain() {
  const invalidate = useInvalidateLifecycle()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (input: {
      domain: string
      app: string
      discardSettings?: boolean
    }) =>
      send<AttachResult>(
        `${BASE}/parked/${encodeURIComponent(input.domain)}/attach`,
        'POST',
        { app: input.app, discard_settings: input.discardSettings === true },
        'attach domain',
      ),
    onSuccess: invalidate,
  })
}

export function useDeleteParkedDomain() {
  const invalidate = useInvalidateLifecycle()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (domain: string) =>
      send<void>(
        `${BASE}/parked/${encodeURIComponent(domain)}`,
        'DELETE',
        undefined,
        'delete parked domain',
      ),
    onSuccess: invalidate,
  })
}
