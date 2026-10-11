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

export type ProxyKind = 'traefik' | 'nginx' | 'caddy' | 'none'
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
  complete: boolean
  missing: string[]
}

export interface ProxySettings {
  integration: 'off' | 'traefik_file'
  dynamic_dir: string
  entrypoint_http: string
  entrypoint_https: string
  cert_resolver: string
  upstream_host: string
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
}

export interface ProxySetupRequest {
  confirm: true
  dynamic_dir?: string
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
  return useInvalidating<ProxySetupRequest, ProxyIntegration>((req) =>
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
