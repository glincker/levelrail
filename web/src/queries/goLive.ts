// Add a domain and go live: the go-live status and plan endpoints, the
// automation policy, its history with undo, and the DNS zone hint
// (internal/api/go_live*.go, domain_automation_policy.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { DomainResolver } from './domainCheck'
import { domainKeys } from './domains'

export type GoLiveStepId =
  | 'dns'
  | 'counterpart'
  | 'redirect'
  | 'proxy_route'
  | 'force_https'
  | 'propagation'
  | 'certificate'
  | 'http'

export type GoLiveStepState =
  'done' | 'pending' | 'skipped' | 'failed' | 'manual' | 'conflict'

export type GoLiveState = 'live' | 'pending' | 'failed' | 'planned' | 'applied'

export interface GoLiveStep {
  id: GoLiveStepId
  state: GoLiveStepState
  detail?: string
  provider?: string
  record?: { name: string; type: string; value: string; ttl_seconds: number }
  resolvers?: DomainResolver[]
  issuer?: string
  not_after?: string
  url?: string
  http_status?: number
}

export interface DomainDnsResult {
  domain: string
  dns: string
  planned?: string
  provider?: string
  zone?: string
  message?: string
  proxied?: boolean
}

export interface GoLivePolicy {
  auto_dns: boolean
  auto_proxy_route: boolean
  force_https: boolean
  www_policy: 'off' | 'redirect_to_apex' | 'redirect_to_www'
  attach_www_counterpart: boolean
  wildcard_for_base_domain: boolean
  verify_after: boolean
}

export interface GoLiveResult {
  run_id?: string
  app: string
  domain: string
  state: GoLiveState
  url?: string
  steps: GoLiveStep[]
  dns?: DomainDnsResult[]
  policy: GoLivePolicy
  undoable?: boolean
  checked_at: string
}

export interface DomainAutomation extends GoLivePolicy {
  dns_provider: 'cloudflare' | 'route53' | 'none'
  proxy_integration_enabled: boolean
  wildcard_dns?: DomainDnsResult
}

export interface AutomationRun {
  id: string
  app: string
  domain: string
  result: GoLiveState
  steps: GoLiveStep[]
  created_at: string
  undone_at?: string
  undoable: boolean
}

export interface DnsZone {
  domain: string
  provider: string
  configured: boolean
  found: boolean
  zone?: string
  message?: string
}

export interface BaseDomainBackfill {
  dry_run: boolean
  base_domain: string
  items: { app: string; domain: string; status: string }[]
}

export const goLiveKeys = {
  status: (app: string, domain: string) =>
    ['apps', app, 'domains', domain, 'go-live'] as const,
  automation: ['domain-automation'] as const,
  runs: ['domain-automation', 'runs'] as const,
  zone: (domain: string) => ['dns-zone', domain] as const,
}

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

const goLivePath = (app: string, domain: string) =>
  `/api/v1/apps/${encodeURIComponent(app)}/domains/${encodeURIComponent(domain)}/go-live`

// Polls while the domain is not live; stops after maxPolls so a stuck
// domain does not poll forever. The server bounds each probe itself.
const GO_LIVE_POLL_MS = 4_000
const GO_LIVE_MAX_POLLS = 60

export function goLiveStatusQueryOptions(app: string, domain: string) {
  return queryOptions({
    queryKey: goLiveKeys.status(app, domain),
    queryFn: () =>
      request<GoLiveResult>(goLivePath(app, domain), undefined, 'go-live'),
    refetchInterval: (query) => {
      const state = query.state.data?.state
      if (state === 'live' || state === 'failed') return false
      return query.state.dataUpdateCount >= GO_LIVE_MAX_POLLS
        ? false
        : GO_LIVE_POLL_MS
    },
    staleTime: 0,
  })
}

export function useGoLiveStatus(app: string, domain: string, enabled = true) {
  return useQuery({ ...goLiveStatusQueryOptions(app, domain), enabled })
}

export function useRunGoLive(app: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: { dns?: string; replace?: boolean }) =>
      request<GoLiveResult>(
        goLivePath(app, domain),
        jsonInit('POST', body),
        'go-live',
      ),
    onSuccess: (data) => {
      queryClient.setQueryData(goLiveKeys.status(app, domain), data)
      void queryClient.invalidateQueries({ queryKey: goLiveKeys.runs })
      void queryClient.invalidateQueries({ queryKey: domainKeys.all })
    },
  })
}

export interface GoLivePlanInput {
  domain: string
  automation?: Partial<GoLivePolicy>
}

export function useGoLivePlan(app: string) {
  return useMutation({
    mutationFn: (input: GoLivePlanInput) =>
      request<{ plans: GoLiveResult[] }>(
        `/api/v1/apps/${encodeURIComponent(app)}/domains/go-live/plan`,
        jsonInit('POST', input),
        'plan',
      ),
  })
}

export function domainAutomationQueryOptions() {
  return queryOptions({
    queryKey: goLiveKeys.automation,
    queryFn: () =>
      request<DomainAutomation>(
        '/api/v1/settings/domain-automation',
        undefined,
        'fetch domain automation',
      ),
    staleTime: 30_000,
  })
}

export function useDomainAutomation() {
  return useQuery(domainAutomationQueryOptions())
}

export function useUpdateDomainAutomation() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (policy: GoLivePolicy) =>
      request<DomainAutomation>(
        '/api/v1/settings/domain-automation',
        jsonInit('PUT', policy),
        'update domain automation',
      ),
    onSuccess: (data) => {
      queryClient.setQueryData(goLiveKeys.automation, data)
    },
  })
}

export function automationRunsQueryOptions(limit = 10) {
  return queryOptions({
    queryKey: [...goLiveKeys.runs, limit] as const,
    queryFn: () =>
      request<AutomationRun[]>(
        `/api/v1/domains/automation/runs?limit=${limit}`,
        undefined,
        'fetch automation runs',
      ),
    staleTime: 15_000,
  })
}

export function useAutomationRuns(limit = 10) {
  return useQuery(automationRunsQueryOptions(limit))
}

export function useUndoAutomationRun() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<AutomationRun & { reverted: string[]; skipped?: string[] }>(
        `/api/v1/domains/automation/runs/${encodeURIComponent(id)}/undo`,
        jsonInit('POST'),
        'undo',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: goLiveKeys.runs })
      void queryClient.invalidateQueries({ queryKey: domainKeys.all })
    },
  })
}

export function dnsZoneQueryOptions(domain: string) {
  return queryOptions({
    queryKey: goLiveKeys.zone(domain),
    queryFn: () =>
      request<DnsZone>(
        `/api/v1/dns/zone?domain=${encodeURIComponent(domain)}`,
        undefined,
        'zone lookup',
      ),
    staleTime: 30_000,
  })
}

export function useBackfillBaseDomain() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (confirm: boolean) =>
      request<BaseDomainBackfill>(
        '/api/v1/settings/ingress/apps-base-domain/backfill',
        jsonInit('POST', { confirm }),
        'backfill',
      ),
    onSuccess: (_data, confirm) => {
      if (confirm) {
        void queryClient.invalidateQueries({ queryKey: domainKeys.all })
      }
    },
  })
}
