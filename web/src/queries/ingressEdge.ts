import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface EdgeLimitValue {
  value: number
  source: 'env' | 'setting' | 'default'
}

export interface EdgeEffective {
  max_header_bytes: EdgeLimitValue
  max_body_bytes: EdgeLimitValue
  read_header_timeout_secs: EdgeLimitValue
  idle_timeout_secs: EdgeLimitValue
}

export interface EdgeDetection {
  proxy: 'none' | 'cloudflare' | 'coolify' | 'traefik' | 'proxy'
  evidence?: string[]
  peer_addr?: string
  suggested?: string
  ranges_rotate?: boolean
}

export interface EdgeSettings {
  trusted_proxies: string
  trusted_proxies_source: 'env' | 'setting' | 'default'
  trusted_proxies_active: boolean
  max_header_bytes: number | null
  max_body_bytes: number | null
  read_header_timeout_secs: number | null
  idle_timeout_secs: number | null
  effective: EdgeEffective
  defaults: EdgeEffective
  detected: EdgeDetection
  needs_trusted_proxies: boolean
  env_locked: Record<string, boolean>
}

export interface EdgeSettingsUpdate {
  trusted_proxies: string
  max_header_bytes: number | null
  max_body_bytes: number | null
  read_header_timeout_secs: number | null
  idle_timeout_secs: number | null
}

export interface AppIngressLimits {
  max_body_bytes: number | null
  instance_max_body_bytes: number
}

export const edgeKeys = {
  settings: ['settings', 'ingress', 'edge'] as const,
  app: (appName: string) => ['apps', appName, 'ingress-limits'] as const,
}

const EDGE_PATH = '/api/v1/settings/ingress/edge'

async function send<T>(
  url: string,
  what: string,
  init?: RequestInit,
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

function json(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export function edgeSettingsQueryOptions() {
  return queryOptions({
    queryKey: edgeKeys.settings,
    queryFn: () => send<EdgeSettings>(EDGE_PATH, 'edge settings'),
    retry: false,
  })
}

export function useEdgeSettingsActions() {
  const qc = useQueryClient()
  const store = (data: EdgeSettings) => {
    qc.setQueryData(edgeKeys.settings, data)
    void qc.invalidateQueries({ queryKey: ['settings', 'ingress'] })
  }
  const save = useMutation<EdgeSettings, ApiError, EdgeSettingsUpdate>({
    mutationFn: (body) =>
      send<EdgeSettings>(EDGE_PATH, 'edge settings save', json('PUT', body)),
    onSuccess: store,
  })
  const fixProxy = useMutation<EdgeSettings, ApiError, void>({
    mutationFn: () =>
      send<EdgeSettings>(
        `${EDGE_PATH}/trusted-proxies/apply-detected`,
        'trusted proxy fix',
        json('POST'),
      ),
    onSuccess: store,
  })
  const recheck = useMutation<EdgeSettings, ApiError, void>({
    mutationFn: () => send<EdgeSettings>(EDGE_PATH, 'edge re-check'),
    onSuccess: store,
  })
  return { save, fixProxy, recheck }
}

function appPath(appName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/ingress-limits`
}

export function appIngressLimitsQueryOptions(appName: string) {
  return queryOptions({
    queryKey: edgeKeys.app(appName),
    queryFn: () => send<AppIngressLimits>(appPath(appName), 'ingress limits'),
    retry: false,
  })
}

export function useAppIngressLimitsSave(appName: string) {
  const qc = useQueryClient()
  return useMutation<AppIngressLimits, ApiError, number | null>({
    mutationFn: (maxBodyBytes) =>
      send<AppIngressLimits>(
        appPath(appName),
        'ingress limits save',
        json('PUT', { max_body_bytes: maxBodyBytes }),
      ),
    onSuccess: (data) => {
      qc.setQueryData(edgeKeys.app(appName), data)
    },
  })
}
