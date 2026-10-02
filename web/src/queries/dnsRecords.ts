// Query-key factory and fetcher for GET/POST/PUT/DELETE
// /api/v1/apps/{name}/domains/{domain}/dns-records
// (internal/api/dns_records.go's dnsRecordsResponse): the actual
// A/AAAA/CNAME/TXT/MX/SRV/CAA records in a domain's zone, from whichever
// ACME DNS-01 provider (Cloudflare or Route53) is configured, each with a
// live resolution status. Mirrors queries/domainWaf.ts's shape for the
// mutations; the list itself needs no set/clear pair since create/update/
// delete each return the refreshed list.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors internal/api's dnsRecordResource wire shape exactly.
export interface DnsRecord {
  name: string
  type: string
  value: string
  ttl_seconds: number
  status?: 'resolved' | 'pending' | 'mismatch' | 'unknown'
}

// Mirrors internal/api's dnsRecordsResponse wire shape exactly.
export interface DnsRecordsResponse {
  domain: string
  provider: string
  zone: string
  records: DnsRecord[]
}

// Mirrors internal/api's updateDNSRecordRequest wire shape exactly.
export interface UpdateDnsRecordRequest {
  original: DnsRecord
  record: DnsRecord
}

export const dnsRecordsKeys = {
  list: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'dns-records'] as const,
}

function dnsRecordsPath(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/dns-records`
}

async function sendDnsRecords(
  appName: string,
  domain: string,
  init: RequestInit | undefined,
  failureLabel: string,
): Promise<DnsRecordsResponse> {
  const res = await fetch(dnsRecordsPath(appName, domain), init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${failureLabel} failed: ${res.status}`),
    )
  }
  return (await res.json()) as DnsRecordsResponse
}

export async function fetchDnsRecords(
  appName: string,
  domain: string,
): Promise<DnsRecordsResponse> {
  return sendDnsRecords(appName, domain, undefined, 'fetch dns records')
}

export function dnsRecordsQueryOptions(appName: string, domain: string) {
  return queryOptions({
    queryKey: dnsRecordsKeys.list(appName, domain),
    queryFn: () => fetchDnsRecords(appName, domain),
    enabled: domain.length > 0,
    staleTime: 15_000,
  })
}

export function useDnsRecords(appName: string, domain: string) {
  return useQuery(dnsRecordsQueryOptions(appName, domain))
}

function jsonInit(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

function useDnsRecordsMutation<TBody>(
  appName: string,
  domain: string,
  method: string,
  failureLabel: string,
) {
  const queryClient = useQueryClient()
  return useMutation<DnsRecordsResponse, ApiError, TBody>({
    mutationFn: (body) =>
      sendDnsRecords(appName, domain, jsonInit(method, body), failureLabel),
    onSuccess: (updated) => {
      queryClient.setQueryData(dnsRecordsKeys.list(appName, domain), updated)
    },
  })
}

export function useCreateDnsRecord(appName: string, domain: string) {
  return useDnsRecordsMutation<DnsRecord>(
    appName,
    domain,
    'POST',
    'create dns record',
  )
}

export function useUpdateDnsRecord(appName: string, domain: string) {
  return useDnsRecordsMutation<UpdateDnsRecordRequest>(
    appName,
    domain,
    'PUT',
    'update dns record',
  )
}

export function useDeleteDnsRecord(appName: string, domain: string) {
  return useDnsRecordsMutation<DnsRecord>(
    appName,
    domain,
    'DELETE',
    'delete dns record',
  )
}
