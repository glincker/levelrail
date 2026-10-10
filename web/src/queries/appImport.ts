// Guided app import (internal/api/app_import*.go). The source token only
// travels in the create and connect request bodies, never the query cache.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type AppImportVerdict =
  'ready' | 'ready-with-notes' | 'needs-attention' | 'unsupported'

export type AppImportState =
  | 'planned'
  | 'staged'
  | 'building'
  | 'verified'
  | 'verify-failed'
  | 'stage-failed'
  | 'routed'
  | 'rolled-back'

export type AppImportStep =
  | 'inventory'
  | 'preflight'
  | 'stage'
  | 'images'
  | 'verify'
  | 'volumes'
  | 'cutover'

export interface AppImportFinding {
  reason: string
  next?: string
}

export interface AppImportVolumeEntry {
  name: string
  container_path: string
  host_path?: string
  size_bytes: number
  size_known: boolean
}

export interface AppImportDatabase {
  source_id: string
  name: string
  host: string
  target?: string
  target_host?: string
}

export interface AppImportEntry {
  source_id: string
  name: string
  kind: 'app' | 'service'
  project?: string
  environment?: string
  server?: string
  source: string
  repo?: string
  branch?: string
  image?: string
  image_id?: string
  host_built?: boolean
  build_pack?: string
  maps_to?: string
  port?: number
  domains?: string[]
  env: { plain: number; secret: number; empty: number }
  volumes?: AppImportVolumeEntry[]
  health?: { path: string; interval_seconds?: number }
  memory_bytes?: number
  nano_cpus?: number
  databases?: AppImportDatabase[]
  verdict: AppImportVerdict
  findings?: AppImportFinding[]
}

export interface AppImportVolumeState {
  name: string
  source_name?: string
  container_path: string
  copied: boolean
  copied_at?: string
}

export interface AppImportItem {
  source_id: string
  name: string
  target?: string
  kind: string
  state: AppImportState
  selected: boolean
  reason?: string
  entry: AppImportEntry
  domains?: string[]
  volumes?: AppImportVolumeState[]
  remaining?: string[]
  app_path?: string
}

export interface AppImportMapping {
  from: string
  to: string
}

export interface AppImportCheck {
  app?: string
  id: string
  status: 'pass' | 'warn' | 'fail'
  detail: string
  fix?: string
}

export interface AppImportChange {
  app: string
  key: string
  secret: boolean
  count: number
  before: string
  after: string
}

export interface AppImportPreflight {
  checks: AppImportCheck[]
  diff: AppImportChange[]
  failed: number
  warnings: number
  env_checked: boolean
  can_stage: boolean
}

export interface AppImportSession {
  id: string
  platform: string
  source_url: string
  step: AppImportStep
  collision: 'suffix' | 'skip'
  mappings: AppImportMapping[]
  suggested_mappings: AppImportMapping[]
  connected: boolean
  running: boolean
  items: AppImportItem[]
  databases: {
    source_id: string
    name: string
    host: string
    target?: string
  }[]
  preflight?: AppImportPreflight
  counts: Record<string, number>
  states: Record<string, number>
  created_at: string
  updated_at: string
  step_order: AppImportStep[]
}

export interface AppImportSource {
  platform: string
  url: string
  token: string
  // docker platform: the docker inspect JSON, used instead of url and token.
  snapshot?: string
  allow_loopback?: boolean
  allow_private?: boolean
  insecure_tls?: boolean
  collision?: 'suffix' | 'skip'
}

export interface AppImportVolumeGuide {
  source_id: string
  index: number
  app: string
  kind: string
  source: string
  target: string
  container_path: string
  command: string
  copied: boolean
  copied_at?: string
}

export interface AppImportVolumes {
  source: string
  guides: AppImportVolumeGuide[]
  warning: string
}

export interface AppImportCutoverCheck {
  id: string
  status: 'pass' | 'warn' | 'fail'
  detail: string
  fix?: string
}

export interface AppImportCutoverDomain {
  app: string
  domain: string
  verdict: 'go' | 'wait' | 'no-go' | 'switched'
  checks: AppImportCutoverCheck[]
  change?: {
    type: string
    name: string
    value: string
    ttl: number
    replaces?: string
  }
}

export interface AppImportCutoverItem {
  source_id: string
  name: string
  target: string
  state: AppImportState
  verdict: AppImportCutoverDomain['verdict']
  routed: boolean
  volumes_pending: number
  domains: AppImportCutoverDomain[]
}

export interface AppImportCutover {
  phase: 'pre-switch' | 'post-switch'
  target_ips: string[]
  verdict: AppImportCutoverDomain['verdict']
  items: AppImportCutoverItem[]
  guidance: string[]
}

const base = '/api/v1/migration/apps/sessions'

export const appImportKeys = {
  list: ['migration', 'apps', 'sessions'] as const,
  one: (id: string) => ['migration', 'apps', 'session', id] as const,
}

async function send<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `request failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function json(method: string, body: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  }
}

const enc = encodeURIComponent

export function useAppImportSessions() {
  return useQuery({
    queryKey: appImportKeys.list,
    queryFn: async () =>
      (await send<{ sessions: AppImportSession[] }>(base)).sessions,
  })
}

export function useAppImportSession(id: string | null) {
  return useQuery({
    queryKey: appImportKeys.one(id ?? ''),
    enabled: id !== null,
    queryFn: () => send<AppImportSession>(`${base}/${enc(id ?? '')}`),
    refetchInterval: (q) =>
      q.state.data?.running || (q.state.data?.states.building ?? 0) > 0
        ? 2000
        : false,
  })
}

function useSessionMutation<V>(
  run: (v: V) => Promise<AppImportSession>,
  idOf: (v: V) => string | null,
) {
  const qc = useQueryClient()
  return useMutation<AppImportSession, ApiError, V>({
    mutationFn: run,
    onSuccess: (s, v) => {
      qc.setQueryData(appImportKeys.one(s.id), s)
      void qc.invalidateQueries({ queryKey: appImportKeys.list })
      const id = idOf(v)
      if (id) void qc.invalidateQueries({ queryKey: appImportKeys.one(id) })
    },
    onError: (_e, v) => {
      const id = idOf(v)
      if (id) void qc.invalidateQueries({ queryKey: appImportKeys.one(id) })
    },
  })
}

export function useCreateAppImportSession() {
  return useSessionMutation<AppImportSource>(
    (src) => send<AppImportSession>(base, json('POST', src)),
    () => null,
  )
}

export function useConnectAppImportSession() {
  return useSessionMutation<{ id: string; source: AppImportSource }>(
    ({ id, source }) =>
      send<AppImportSession>(
        `${base}/${enc(id)}/connect`,
        json('POST', source),
      ),
    (v) => v.id,
  )
}

export interface AppImportPlanUpdate {
  mappings?: AppImportMapping[]
  selected?: string[]
  collision?: 'suffix' | 'skip'
  step?: AppImportStep
}

export function usePutAppImportPlan() {
  return useSessionMutation<{ id: string; update: AppImportPlanUpdate }>(
    ({ id, update }) =>
      send<AppImportSession>(`${base}/${enc(id)}/plan`, json('PUT', update)),
    (v) => v.id,
  )
}

export function useStageAppImport() {
  return useSessionMutation<{ id: string }>(
    ({ id }) =>
      send<AppImportSession>(`${base}/${enc(id)}/stage`, json('POST', {})),
    (v) => v.id,
  )
}

export function useVerifyAppImport() {
  return useSessionMutation<{ id: string; items?: string[] }>(
    ({ id, items }) =>
      send<AppImportSession>(
        `${base}/${enc(id)}/verify`,
        json('POST', { items }),
      ),
    (v) => v.id,
  )
}

export function useRollbackAppImport() {
  return useSessionMutation<{ id: string; items?: string[] }>(
    ({ id, items }) =>
      send<AppImportSession>(
        `${base}/${enc(id)}/rollback`,
        json('POST', { items }),
      ),
    (v) => v.id,
  )
}

export function useRouteAppImport() {
  return useSessionMutation<{
    id: string
    item: string
    enable: boolean
    ignoreVolumes?: boolean
  }>(
    ({ id, item, enable, ignoreVolumes }) =>
      send<AppImportSession>(
        `${base}/${enc(id)}/items/${enc(item)}/route`,
        json('POST', { enable, ignore_volumes: ignoreVolumes }),
      ),
    (v) => v.id,
  )
}

export function useDeleteAppImportSession() {
  const qc = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: (id) => send<void>(`${base}/${enc(id)}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: appImportKeys.list }),
  })
}

export function useAppImportVolumes() {
  return useMutation<
    AppImportVolumes,
    ApiError,
    { id: string; source: string }
  >({
    mutationFn: ({ id, source }) =>
      send<AppImportVolumes>(
        `${base}/${enc(id)}/volumes?source=${enc(source)}`,
      ),
  })
}

export function useSetAppImportVolume() {
  const qc = useQueryClient()
  return useMutation<
    AppImportVolumeState[],
    ApiError,
    { id: string; item: string; index: number; copied: boolean }
  >({
    mutationFn: ({ id, item, index, copied }) =>
      send<AppImportVolumeState[]>(
        `${base}/${enc(id)}/items/${enc(item)}/volumes/${index}`,
        json('PUT', { copied }),
      ),
    onSuccess: (_v, vars) =>
      qc.invalidateQueries({ queryKey: appImportKeys.one(vars.id) }),
  })
}

export function useAppImportCutover() {
  return useMutation<
    AppImportCutover,
    ApiError,
    { id: string; verify: boolean; targetIp?: string }
  >({
    mutationFn: ({ id, verify, targetIp }) => {
      const q = targetIp ? `?target_ip=${enc(targetIp)}` : ''
      return send<AppImportCutover>(
        `${base}/${enc(id)}/cutover${verify ? '/verify' : ''}${q}`,
      )
    },
  })
}

export function appImportReceiptUrl(id: string): string {
  return `${base}/${enc(id)}/receipt`
}

export type AppImportImageState =
  'pending' | 'running' | 'verified' | 'loaded' | 'failed' | 'cancelled'

export interface AppImportImage {
  source_id: string
  app: string
  target?: string
  image: string
  source_image_id?: string
  loaded_image_id?: string
  node?: string
  state: AppImportImageState
  bytes: number
  verified: boolean
  error?: string
  updated_at?: string
}

export interface AppImportImages {
  running: boolean
  source?: string
  credentials_held: boolean
  supported: boolean
  max_bytes: number
  images: AppImportImage[]
}

// The private key travels in this request body only, never the query cache.
export interface AppImportImagesTransfer {
  ssh?: string
  port?: number
  private_key?: string
  passphrase?: string
  use_agent?: boolean
  items?: string[]
}

const imageKeys = (id: string) =>
  ['migration', 'apps', 'session', id, 'images'] as const

export function useAppImportImages(id: string) {
  return useQuery({
    queryKey: imageKeys(id),
    queryFn: () => send<AppImportImages>(`${base}/${enc(id)}/images`),
    refetchInterval: (q) => (q.state.data?.running ? 1500 : false),
  })
}

export function useTransferAppImportImages() {
  const qc = useQueryClient()
  return useMutation<
    AppImportImages,
    ApiError,
    { id: string; body: AppImportImagesTransfer }
  >({
    mutationFn: ({ id, body }) =>
      send<AppImportImages>(
        `${base}/${enc(id)}/images/transfer`,
        json('POST', body),
      ),
    onSuccess: (v, vars) => {
      qc.setQueryData(imageKeys(vars.id), v)
      void qc.invalidateQueries({
        queryKey: appImportKeys.one(vars.id),
        exact: true,
      })
    },
  })
}

export function useCancelAppImportImages() {
  const qc = useQueryClient()
  return useMutation<AppImportImages, ApiError, string>({
    mutationFn: (id) =>
      send<AppImportImages>(`${base}/${enc(id)}/images/cancel`, {
        method: 'POST',
      }),
    onSuccess: (v, id) => qc.setQueryData(imageKeys(id), v),
  })
}
