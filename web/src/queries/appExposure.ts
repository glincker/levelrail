// Matches internal/api/app_exposure.go: port and exposure of one app.
import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appNetworkKeys } from './appNetwork'

export type ExposureMode = 'proxied' | 'direct'

export interface ExposureChange {
  kind: string
  from?: string
  to?: string
}

export interface ExposureUrl {
  kind: 'domain' | 'fallback' | 'direct'
  url: string
}

export interface PortHolder {
  port: number
  kind: string
  holder: string
}

export interface ExposurePlan {
  mode: ExposureMode
  container_port: number
  host_port?: number
  bind_address: string
  changes: ExposureChange[]
  warnings?: string[]
  recreates: boolean
  urls: ExposureUrl[]
}

export interface AppExposure extends ExposurePlan {
  app: string
  running: boolean
  firewall_rule_id?: string
  ports_in_use: PortHolder[]
  port_range: [number, number]
  local_node: boolean
}

export interface ExposureRequest {
  mode?: ExposureMode
  host_port?: number
  container_port?: number
}

export interface ProbeTarget {
  kind: string
  url: string
  ok: boolean
  status?: number
  latency_ms?: number
  failure?: string
  detail?: string
}

export interface ExposureCheck {
  app: string
  scope: string
  targets: ProbeTarget[]
}

export const appExposureKeys = {
  detail: (app: string) => ['apps', app, 'exposure'] as const,
}

function base(app: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/exposure`
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
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function appExposureQueryOptions(app: string) {
  return queryOptions({
    queryKey: appExposureKeys.detail(app),
    queryFn: () =>
      send<AppExposure>(base(app), 'GET', undefined, 'fetch exposure'),
  })
}

export function useAppExposure(app: string) {
  return useQuery({ ...appExposureQueryOptions(app), refetchInterval: 15_000 })
}

export function usePreviewExposure(app: string) {
  return useMutation({
    mutationFn: (req: ExposureRequest) =>
      send<ExposurePlan>(
        `${base(app)}/preview`,
        'POST',
        req,
        'preview exposure',
      ),
  })
}

export function useSetExposure(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: ExposureRequest) =>
      send<AppExposure>(base(app), 'PUT', req, 'update exposure'),
    onSuccess: (data) => {
      qc.setQueryData(appExposureKeys.detail(app), data)
      void qc.invalidateQueries({ queryKey: appNetworkKeys.detail(app) })
      void qc.invalidateQueries({ queryKey: ['apps', app] })
    },
  })
}

export function useCheckExposure(app: string) {
  return useMutation({
    mutationFn: () =>
      send<ExposureCheck>(
        `${base(app)}/check`,
        'POST',
        undefined,
        'check exposure',
      ),
  })
}
