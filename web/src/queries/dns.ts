import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError } from '../lib/apiError'
import type {
  DnsDelegationResponse,
  DnsDiscoverResult,
  DnsHealthCheck,
  DnsIssue,
  DnsPlanResult,
  DnsPropagation,
  DnsRecordKey,
  DnsRecordSet,
  DnsRecordsResponse,
  DnsRecordWrite,
  DnsTemplate,
  DnsZone,
  DnsZoneOverview,
  DnsZonesResponse,
} from '../types/dns'

/** DnsApiError keeps the conflict list a 409 carries alongside its message. */
export class DnsApiError extends ApiError {
  readonly issues: DnsIssue[]
  constructor(status: number, message: string, issues: DnsIssue[]) {
    super(status, message)
    this.name = 'DnsApiError'
    this.issues = issues
  }
}

export const dnsKeys = {
  all: ['dns'] as const,
  zones: () => [...dnsKeys.all, 'zones'] as const,
  zone: (zone: string) => [...dnsKeys.all, 'zone', zone] as const,
  records: (zone: string) => [...dnsKeys.zone(zone), 'records'] as const,
  delegation: (zone: string) => [...dnsKeys.zone(zone), 'delegation'] as const,
  templates: () => [...dnsKeys.all, 'templates'] as const,
  healthChecks: () => [...dnsKeys.all, 'health-checks'] as const,
  check: (name: string, type: string) =>
    [...dnsKeys.all, 'check', name, type] as const,
}

const zoneBase = (zone: string) =>
  `/api/v1/dns/zones/${encodeURIComponent(zone)}`

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: string
      issues?: DnsIssue[]
    } | null
    throw new DnsApiError(
      res.status,
      body?.error ?? `request failed: ${res.status}`,
      body?.issues ?? [],
    )
  }
  return (await res.json()) as T
}

function json(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export const dnsZonesQueryOptions = () =>
  queryOptions({
    queryKey: dnsKeys.zones(),
    queryFn: () => request<DnsZonesResponse>('/api/v1/dns/zones'),
    staleTime: 30_000,
  })

export const dnsZoneQueryOptions = (zone: string) =>
  queryOptions({
    queryKey: dnsKeys.zone(zone),
    queryFn: () => request<DnsZoneOverview>(zoneBase(zone)),
    staleTime: 15_000,
  })

export const dnsRecordsQueryOptions = (zone: string) =>
  queryOptions({
    queryKey: dnsKeys.records(zone),
    queryFn: () => request<DnsRecordsResponse>(`${zoneBase(zone)}/records`),
    staleTime: 15_000,
  })

export const dnsTemplatesQueryOptions = () =>
  queryOptions({
    queryKey: dnsKeys.templates(),
    queryFn: () => request<DnsTemplate[]>('/api/v1/dns/templates'),
    staleTime: Infinity,
  })

export function useDnsDelegation(zone: string, enabled = true) {
  return useQuery({
    queryKey: dnsKeys.delegation(zone),
    queryFn: () =>
      request<DnsDelegationResponse>(`${zoneBase(zone)}/delegation`),
    enabled,
    staleTime: 20_000,
    retry: false,
  })
}

export function useDnsHealthChecks(enabled: boolean) {
  return useQuery({
    queryKey: dnsKeys.healthChecks(),
    queryFn: () => request<DnsHealthCheck[]>('/api/v1/dns/health-checks'),
    enabled,
    retry: false,
  })
}

export function useDnsCheck(name: string, type: string, zone: string) {
  return useQuery({
    queryKey: dnsKeys.check(name, type),
    queryFn: () =>
      request<DnsPropagation>(
        `/api/v1/dns/check?${new URLSearchParams({ name, type, zone }).toString()}`,
      ),
    enabled: name !== '',
    retry: false,
    staleTime: 0,
  })
}

export function useDnsDiscover(zone: string, names: string, enabled: boolean) {
  return useQuery({
    queryKey: [...dnsKeys.zone(zone), 'discover', names],
    queryFn: () =>
      request<DnsDiscoverResult>(
        `${zoneBase(zone)}/discover?${new URLSearchParams({ names }).toString()}`,
      ),
    enabled,
    retry: false,
    staleTime: 60_000,
  })
}

function useInvalidateZone(zone: string) {
  const qc = useQueryClient()
  return () =>
    Promise.all([
      qc.invalidateQueries({ queryKey: dnsKeys.zone(zone) }),
      qc.invalidateQueries({ queryKey: dnsKeys.zones() }),
    ])
}

export function useCreateDnsZone() {
  const qc = useQueryClient()
  return useMutation<DnsZone, DnsApiError, { name: string }>({
    mutationFn: (input) => request('/api/v1/dns/zones', json('POST', input)),
    onSuccess: () => qc.invalidateQueries({ queryKey: dnsKeys.zones() }),
  })
}

export function useDeleteDnsZone(zone: string) {
  const qc = useQueryClient()
  return useMutation<
    { deleted: string },
    DnsApiError,
    { confirm: string; force: boolean }
  >({
    mutationFn: (input) => request(zoneBase(zone), json('DELETE', input)),
    onSuccess: () => qc.invalidateQueries({ queryKey: dnsKeys.all }),
  })
}

export function useSaveDnsRecord(zone: string) {
  const invalidate = useInvalidateZone(zone)
  return useMutation<
    DnsRecordWrite,
    DnsApiError,
    { record: DnsRecordSet; original?: DnsRecordKey }
  >({
    mutationFn: ({ record, original }) =>
      original
        ? request(
            `${zoneBase(zone)}/records`,
            json('PUT', { original, record }),
          )
        : request(`${zoneBase(zone)}/records`, json('POST', record)),
    onSuccess: invalidate,
  })
}

export function useDeleteDnsRecord(zone: string) {
  const invalidate = useInvalidateZone(zone)
  return useMutation<unknown, DnsApiError, DnsRecordKey>({
    mutationFn: (k) => {
      const q = new URLSearchParams({ name: k.name, type: k.type })
      if (k.set_identifier) q.set('set_identifier', k.set_identifier)
      return request(`${zoneBase(zone)}/records?${q.toString()}`, {
        method: 'DELETE',
      })
    },
    onSuccess: invalidate,
  })
}

export interface DnsImportInput {
  format: 'json' | 'bind'
  content?: string
  records?: DnsRecordSet[]
  replace?: boolean
  apply?: boolean
  confirm?: string
}

export function useImportDnsRecords(zone: string) {
  const invalidate = useInvalidateZone(zone)
  return useMutation<DnsPlanResult, DnsApiError, DnsImportInput>({
    mutationFn: (input) =>
      request(`${zoneBase(zone)}/records/import`, json('POST', input)),
    onSuccess: (res) => (res.applied > 0 ? invalidate() : undefined),
  })
}

export function useApplyDnsTemplate(zone: string) {
  const invalidate = useInvalidateZone(zone)
  return useMutation<
    DnsPlanResult,
    DnsApiError,
    { id: string; params: Record<string, string>; apply: boolean }
  >({
    mutationFn: ({ id, ...body }) =>
      request(
        `${zoneBase(zone)}/templates/${encodeURIComponent(id)}`,
        json('POST', body),
      ),
    onSuccess: (res) => (res.applied > 0 ? invalidate() : undefined),
  })
}

export function useCreateDnsHealthCheck() {
  const qc = useQueryClient()
  return useMutation<DnsHealthCheck, DnsApiError, DnsHealthCheck>({
    mutationFn: (hc) => request('/api/v1/dns/health-checks', json('POST', hc)),
    onSuccess: () => qc.invalidateQueries({ queryKey: dnsKeys.healthChecks() }),
  })
}

export function useDeleteDnsHealthCheck() {
  const qc = useQueryClient()
  return useMutation<unknown, DnsApiError, string>({
    mutationFn: (id) =>
      request(`/api/v1/dns/health-checks/${encodeURIComponent(id)}`, {
        method: 'DELETE',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: dnsKeys.healthChecks() }),
  })
}

export function exportDnsZoneUrl(zone: string, format: 'json' | 'bind') {
  return `${zoneBase(zone)}/records/export?format=${format}`
}
