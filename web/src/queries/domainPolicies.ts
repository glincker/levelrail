// Per-domain traffic controls: internal/api/domain_policies*.go.
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError } from '../lib/apiError'
import type {
  CacheStats,
  CanonicalPreset,
  DomainPolicies,
  DomainPorts,
  DomainRedirects,
  FieldError,
  GeoLookup,
  PolicyDraft,
  PolicyKind,
  PreviewResult,
  RedirectSettings,
  SpecByKind,
} from './domainPolicyTypes'

// PolicyApiError carries the per-field messages a 400 returns, so editors
// can show them next to the input that caused them.
export class PolicyApiError extends ApiError {
  readonly fields: FieldError[]

  constructor(status: number, message: string, fields: FieldError[]) {
    super(status, message)
    this.name = 'PolicyApiError'
    this.fields = fields
  }
}

export async function toPolicyError(
  res: Response,
  fallback: string,
): Promise<PolicyApiError> {
  const body = (await res.json().catch(() => null)) as {
    error?: string
    fields?: FieldError[]
  } | null
  return new PolicyApiError(
    res.status,
    body?.error ?? fallback,
    body?.fields ?? [],
  )
}

export function fieldErrorsFor(
  error: unknown,
  prefix: string,
): Record<string, string> {
  const out: Record<string, string> = {}
  if (!(error instanceof PolicyApiError)) return out
  for (const f of error.fields) {
    const key = f.field.startsWith(prefix + '.')
      ? f.field.slice(prefix.length + 1)
      : f.field
    out[key] = f.message
  }
  return out
}

function base(app: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/domains/${encodeURIComponent(domain)}`
}

export const domainPolicyKeys = {
  all: (app: string, domain: string) =>
    ['apps', app, 'domains', domain, 'policies'] as const,
  redirects: (app: string, domain: string) =>
    ['apps', app, 'domains', domain, 'policies', 'redirects'] as const,
  ports: (app: string, domain: string) =>
    ['apps', app, 'domains', domain, 'policies', 'ports'] as const,
  cacheStats: (app: string, domain: string) =>
    ['apps', app, 'domains', domain, 'policies', 'cache-stats'] as const,
  geoLookup: (ip: string) => ['system', 'geoip', ip] as const,
}

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  fallback: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) throw await toPolicyError(res, `${fallback}: ${res.status}`)
  return (await res.json()) as T
}

function jsonInit(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

export function domainPoliciesQueryOptions(app: string, domain: string) {
  return queryOptions({
    queryKey: domainPolicyKeys.all(app, domain),
    queryFn: () =>
      request<DomainPolicies>(
        `${base(app, domain)}/policies`,
        undefined,
        'load traffic controls failed',
      ),
    enabled: app.length > 0,
    staleTime: 15_000,
  })
}

export function useDomainPolicies(app: string, domain: string) {
  return useQuery(domainPoliciesQueryOptions(app, domain))
}

export function domainRedirectsQueryOptions(app: string, domain: string) {
  return queryOptions({
    queryKey: domainPolicyKeys.redirects(app, domain),
    queryFn: () =>
      request<DomainRedirects>(
        `${base(app, domain)}/redirects`,
        undefined,
        'load redirects failed',
      ),
    staleTime: 15_000,
  })
}

export function useDomainRedirects(app: string, domain: string) {
  return useQuery(domainRedirectsQueryOptions(app, domain))
}

export function useDomainPorts(app: string, domain: string) {
  return useQuery({
    queryKey: domainPolicyKeys.ports(app, domain),
    queryFn: () =>
      request<DomainPorts>(
        `${base(app, domain)}/ports`,
        undefined,
        'load ports failed',
      ),
    staleTime: 15_000,
  })
}

export function useDomainCacheStats(app: string, domain: string) {
  return useQuery({
    queryKey: domainPolicyKeys.cacheStats(app, domain),
    queryFn: () =>
      request<CacheStats>(
        `${base(app, domain)}/cache/stats`,
        undefined,
        'load cache stats failed',
      ),
    refetchInterval: 15_000,
  })
}

export function useGeoLookup(ip: string) {
  return useQuery({
    queryKey: domainPolicyKeys.geoLookup(ip),
    queryFn: () =>
      request<GeoLookup>(
        `/api/v1/system/geoip?ip=${encodeURIComponent(ip)}`,
        undefined,
        'country lookup failed',
      ),
    enabled: ip.length > 0,
    staleTime: 60_000,
  })
}

export interface PreviewInput {
  policy: PolicyDraft
  method?: string
  path?: string
  urls?: string[]
  canonical_preset?: CanonicalPreset
}

// Keyed by the draft itself so an unchanged draft reuses the last answer.
export function usePolicyPreview(
  app: string,
  domain: string,
  input: PreviewInput | null,
) {
  return useQuery({
    queryKey: [...domainPolicyKeys.all(app, domain), 'preview', input],
    queryFn: () =>
      request<PreviewResult>(
        `${base(app, domain)}/policies/preview`,
        jsonInit('POST', input),
        'preview failed',
      ),
    enabled: input !== null,
    staleTime: 30_000,
    placeholderData: (prev) => prev,
  })
}

function useInvalidateAll(app: string, domain: string) {
  const queryClient = useQueryClient()
  return () =>
    queryClient.invalidateQueries({
      queryKey: domainPolicyKeys.all(app, domain),
    })
}

export function useSavePolicy<K extends PolicyKind>(
  app: string,
  domain: string,
  kind: K,
) {
  const invalidate = useInvalidateAll(app, domain)
  return useMutation<unknown, PolicyApiError, SpecByKind[K]>({
    mutationFn: (spec) =>
      request(
        `${base(app, domain)}/${kind}`,
        jsonInit('PUT', spec),
        `save ${kind} failed`,
      ),
    onSuccess: () => invalidate(),
  })
}

export function useResetPolicy(app: string, domain: string) {
  const invalidate = useInvalidateAll(app, domain)
  return useMutation<unknown, PolicyApiError, PolicyKind | 'redirects'>({
    mutationFn: (kind) =>
      request(
        `${base(app, domain)}/${kind}`,
        { method: 'DELETE' },
        `reset ${kind} failed`,
      ),
    onSuccess: () => invalidate(),
  })
}

export function usePurgeCache(app: string, domain: string) {
  const invalidate = useInvalidateAll(app, domain)
  return useMutation<
    { purged: number },
    PolicyApiError,
    { scope: 'url' | 'prefix' | 'all'; value?: string }
  >({
    mutationFn: (body) =>
      request(
        `${base(app, domain)}/cache/purge`,
        jsonInit('POST', body),
        'purge failed',
      ),
    onSuccess: () => invalidate(),
  })
}

function useRedirectMutation<V>(
  app: string,
  domain: string,
  build: (v: V) => { path: string; init: RequestInit },
) {
  const queryClient = useQueryClient()
  return useMutation<DomainRedirects, PolicyApiError, V>({
    mutationFn: (v) => {
      const { path, init } = build(v)
      return request<DomainRedirects>(
        `${base(app, domain)}${path}`,
        init,
        'save redirects failed',
      )
    },
    onSuccess: (data) => {
      queryClient.setQueryData(domainPolicyKeys.redirects(app, domain), data)
      void queryClient.invalidateQueries({ queryKey: ['domains'] })
    },
  })
}

export function useSaveRedirectSettings(app: string, domain: string) {
  return useRedirectMutation<RedirectSettings>(app, domain, (s) => ({
    path: '/redirects',
    init: jsonInit('PUT', s),
  }))
}

export function useSetCanonical(app: string, domain: string) {
  return useRedirectMutation<{
    preset: CanonicalPreset
    status_code?: number
    replace?: boolean
  }>(app, domain, (body) => ({
    path: '/redirects/canonical',
    init: jsonInit('POST', body),
  }))
}

export function useSetAliases(app: string, domain: string) {
  return useRedirectMutation<{ aliases: string[]; status_code: number }>(
    app,
    domain,
    (body) => ({ path: '/redirects/aliases', init: jsonInit('PUT', body) }),
  )
}

export function useRestrictPort(app: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<
    DomainPorts,
    PolicyApiError,
    { port: number; sources: string[] | null }
  >({
    mutationFn: ({ port, sources }) =>
      request<DomainPorts>(
        `${base(app, domain)}/ports/${port}/restrict`,
        sources === null ? { method: 'DELETE' } : jsonInit('PUT', { sources }),
        'update port access failed',
      ),
    onSuccess: (data) => {
      queryClient.setQueryData(domainPolicyKeys.ports(app, domain), data)
    },
  })
}
