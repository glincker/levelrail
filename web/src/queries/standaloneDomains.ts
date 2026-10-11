// Standalone domains (internal/api/standalone_domains.go): domains not owned
// by any app that proxy to a port, address, URL, app or container.
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { domainKeys } from './domains'

export type StandaloneTargetKind =
  'port' | 'address' | 'url' | 'app' | 'container'

export interface StandaloneDomain {
  domain: string
  kind: 'standalone'
  scope: string
  target: string
  target_kind: StandaloneTargetKind
  target_url: string
  upstream_tls: boolean
  tls_skip_verify: boolean
  tls_server_name?: string
  host_header: string
  websocket: boolean
  dial_timeout_seconds: number
  read_timeout_seconds: number
  strip_prefix?: string
  description?: string
  created_by?: string
  created_at?: string
  updated_at?: string
}

export interface StandaloneDomainInput {
  domain?: string
  target?: string
  upstream_tls?: boolean
  tls_skip_verify?: boolean
  tls_server_name?: string
  host_header?: string
  websocket?: boolean
  dial_timeout_seconds?: number
  read_timeout_seconds?: number
  strip_prefix?: string
  description?: string
  confirm?: boolean
}

export interface StandaloneAssessment {
  dial?: string
  addresses?: string[]
  internal: boolean
  loopback: boolean
  control_port?: { port: number; label: string }
  needs_confirmation?: string[]
}

export interface StandaloneValidation {
  domain?: string
  domain_error?: string
  target_url?: string
  target_error?: string
  assessment: StandaloneAssessment
}

export interface StandaloneCandidates {
  host_ports: { port: number; label?: string; blocked?: boolean }[]
  containers: { name: string; ports?: number[] }[]
  apps: { name: string; port: number }[]
}

// Thrown on 409 when the target is private, loopback or a control port and
// the caller has not confirmed yet.
export class StandaloneConfirmationError extends ApiError {
  readonly reasons: string[]

  constructor(message: string, reasons: string[]) {
    super(409, message)
    this.name = 'StandaloneConfirmationError'
    this.reasons = reasons
  }
}

const BASE = '/api/v1/domains/standalone'

export const standaloneDomainKeys = {
  all: ['domains', 'standalone'] as const,
  candidates: ['domains', 'standalone', 'candidates'] as const,
  validate: (input: StandaloneDomainInput) =>
    ['domains', 'standalone', 'validate', input] as const,
}

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
  if (res.status === 409) {
    const data = (await res.json().catch(() => null)) as {
      error?: string
      needs_confirmation?: string[]
    } | null
    if (data?.needs_confirmation?.length) {
      throw new StandaloneConfirmationError(
        data.error ?? what,
        data.needs_confirmation,
      )
    }
    throw new ApiError(409, data?.error ?? `${what} failed: 409`)
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

export function standaloneDomainsQueryOptions() {
  return queryOptions({
    queryKey: standaloneDomainKeys.all,
    queryFn: () =>
      send<StandaloneDomain[]>(
        BASE,
        'GET',
        undefined,
        'list standalone domains',
      ),
    staleTime: 30_000,
  })
}

export function useStandaloneDomains() {
  return useQuery(standaloneDomainsQueryOptions())
}

export function standaloneCandidatesQueryOptions() {
  return queryOptions({
    queryKey: standaloneDomainKeys.candidates,
    queryFn: () =>
      send<StandaloneCandidates>(
        `${BASE}/candidates`,
        'GET',
        undefined,
        'list proxy targets',
      ),
    staleTime: 30_000,
  })
}

export function useStandaloneCandidates(enabled: boolean) {
  return useQuery({ ...standaloneCandidatesQueryOptions(), enabled })
}

export function useValidateStandalone(
  input: StandaloneDomainInput,
  enabled: boolean,
) {
  return useQuery({
    queryKey: standaloneDomainKeys.validate(input),
    queryFn: () =>
      send<StandaloneValidation>(
        `${BASE}/validate`,
        'POST',
        input,
        'check target',
      ),
    enabled,
    staleTime: 10_000,
  })
}

function useInvalidateDomains() {
  const qc = useQueryClient()
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: standaloneDomainKeys.all }),
      qc.invalidateQueries({ queryKey: domainKeys.all }),
    ])
}

export function useCreateStandaloneDomain() {
  const invalidate = useInvalidateDomains()
  return useMutation({
    mutationFn: (input: StandaloneDomainInput) =>
      send<StandaloneDomain>(BASE, 'POST', input, 'add standalone domain'),
    onSuccess: invalidate,
  })
}

export function useUpdateStandaloneDomain(domain: string) {
  const invalidate = useInvalidateDomains()
  return useMutation({
    mutationFn: (input: StandaloneDomainInput) =>
      send<StandaloneDomain>(
        `${BASE}/${encodeURIComponent(domain)}`,
        'PUT',
        input,
        'update standalone domain',
      ),
    onSuccess: invalidate,
  })
}

export function useDeleteStandaloneDomain() {
  const invalidate = useInvalidateDomains()
  return useMutation({
    mutationFn: (domain: string) =>
      send<undefined>(
        `${BASE}/${encodeURIComponent(domain)}`,
        'DELETE',
        undefined,
        'remove standalone domain',
      ),
    onSuccess: invalidate,
  })
}
