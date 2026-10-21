// GET /api/v1/system/proxy-integration and its setup/apply/verify actions:
// make a domain live behind an existing reverse proxy (Traefik, nginx, Caddy).
import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { useRef } from 'react'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { nextPollInterval } from '../lib/proxyIntegration'

export type ProxyKind = 'traefik' | 'nginx' | 'caddy' | 'haproxy' | 'none'
export type ProxyProviderKind =
  'embedded' | 'traefik' | 'nginx' | 'caddy' | 'haproxy'
export type ProxyProviderMode = 'builtin' | 'managed' | 'manual'
export type ProxyStepId =
  'detect' | 'tls_upstream' | 'routes' | 'dns' | 'verify'
export type ProxyStepState = 'done' | 'todo' | 'blocked' | 'error'
export type ProxyRouteState = 'missing' | 'written' | 'stale' | 'error'

export interface DetectedProxy {
  kind: ProxyKind
  container: string
  image: string
  published_ports: number[]
  dynamic_dir: string
  entrypoint_http: string
  entrypoint_https: string
  cert_resolver: string
  upstream_host: string
  cert_dir?: string
  webroot?: string
  complete: boolean
  missing: string[]
}

export interface ProxySettings {
  integration: 'off' | 'traefik_file' | 'nginx_file'
  dynamic_dir: string
  entrypoint_http: string
  entrypoint_https: string
  cert_resolver: string
  upstream_host: string
  container?: string
  cert_dir?: string
  webroot?: string
}

export interface ProxyProvider {
  kind: ProxyProviderKind
  mode: ProxyProviderMode
  detected: boolean
  selected: boolean
}

export interface ProxyIngress {
  http_port: number
  https_port: number
  dashboard_addr: string
  tls_terminated_upstream: boolean
  public_https_port: number
}

export interface ProxyCertificate {
  issuer: string
  not_after: string
  valid: boolean
}

export interface ProxyDomain {
  domain: string
  target: 'app' | 'dashboard'
  app: string
  file: string
  state: ProxyRouteState
  proxy_loaded: boolean | null
  reachable: boolean
  certificate: ProxyCertificate | null
  last_error: string
  checked_at: string
  // delivered: verification proved requests reach the built-in ingress.
  delivered?: boolean
}

export interface ProxyStep {
  id: ProxyStepId
  state: ProxyStepState
  detail: string
}

export interface ProxyIntegration {
  detected: DetectedProxy
  settings: ProxySettings
  ingress: ProxyIngress
  domains: ProxyDomain[]
  steps: ProxyStep[]
  provider?: ProxyProviderKind
  providers?: ProxyProvider[]
}

export interface ProxySetupRequest {
  confirm: boolean
  dynamic_dir?: string
  provider?: ProxyProviderKind | 'auto'
  cert_dir?: string
}

// Setup answers with the integration plus what it did or would do. A
// manual provider (Caddy, HAProxy) returns a snippet and never applies.
export interface ProxySetupResult extends ProxyIntegration {
  dry_run?: boolean
  changes?: string[]
  manual?: boolean
  snippet?: string
}

export const proxyIntegrationKeys = {
  all: ['system', 'proxy-integration'] as const,
}

const BASE = '/api/v1/system/proxy-integration'

// 404 and 501 mean this server version has no proxy integration: null, not an error.
export async function fetchProxyIntegration(): Promise<ProxyIntegration | null> {
  const res = await fetch(BASE)
  if (res.status === 404 || res.status === 501) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `proxy integration failed: ${res.status}`),
    )
  }
  return (await res.json()) as ProxyIntegration
}

export function proxyIntegrationQueryOptions() {
  return queryOptions({
    queryKey: proxyIntegrationKeys.all,
    queryFn: fetchProxyIntegration,
    staleTime: 5_000,
    retry: false,
  })
}

// Polls only while a domain is not live, and at most MAX_POLLS times.
export function useProxyIntegration() {
  const polls = useRef(0)
  const query = useQuery({
    ...proxyIntegrationQueryOptions(),
    refetchInterval: (q) => {
      const next = nextPollInterval(q.state.data, polls.current)
      if (next !== false) polls.current += 1
      return next
    },
  })
  return {
    ...query,
    restartPolling: () => {
      polls.current = 0
    },
  }
}

async function post<T>(path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `proxy integration failed: ${res.status}`),
    )
  }
  return (await res.json().catch(() => ({}))) as T
}

function useInvalidating<V, R>(fn: (vars: V) => Promise<R>) {
  const queryClient = useQueryClient()
  return useMutation<R, ApiError, V>({
    mutationFn: fn,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: proxyIntegrationKeys.all })
    },
  })
}

export function useSetupProxyIntegration() {
  return useInvalidating<ProxySetupRequest, ProxySetupResult>((req) =>
    post(`${BASE}/setup`, req),
  )
}

export function useApplyProxyIntegration() {
  return useInvalidating<void, ProxyIntegration>(() => post(`${BASE}/apply`))
}

export function useVerifyProxyDomain() {
  return useInvalidating<string, ProxyDomain>((domain) =>
    post(`${BASE}/verify?domain=${encodeURIComponent(domain)}`),
  )
}

export function useUpdateProxySettings() {
  return useInvalidating<ProxySettings, ProxySettings>(async (settings) => {
    const res = await fetch('/api/v1/settings/proxy-integration', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(settings),
    })
    if (!res.ok) {
      throw new ApiError(
        res.status,
        await readErrorMessage(
          res,
          `save proxy settings failed: ${res.status}`,
        ),
      )
    }
    return (await res.json()) as ProxySettings
  })
}
