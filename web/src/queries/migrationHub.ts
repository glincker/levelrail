// Server migration hub (internal/api/migration_hub*.go). The source password
// only travels in the create and apply request bodies, never the query cache.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { CopyTable } from './migration'

export type HubStep = 'inventory' | 'preflight' | 'copy' | 'verify' | 'cutover'
export type HubItemStatus = 'pending' | 'copying' | 'verified' | 'failed'
export type HubSeverity = 'ok' | 'warn' | 'block'

export interface HubCheck {
  id: string
  severity: HubSeverity
  message: string
  next_action?: string
}

export interface HubPreflight {
  checks: HubCheck[]
  target_version: string
  required_bytes: number
  estimate_seconds: number
  blocked: boolean
}

export interface HubItem {
  source_db: string
  size_bytes: number
  tables: number
  extensions?: string[]
  target_name: string
  target_version: string
  selected: boolean
  status: HubItemStatus
  reason?: string
  preflight: HubPreflight
  checked: number
  mismatched: number
  table_counts?: CopyTable[]
  finished_at?: string
}

export interface HubSummary {
  databases: number
  selected: number
  blocked: number
  warnings: number
  verified: number
  failed: number
  copying: number
  total_bytes: number
  required_bytes: number
  estimate_seconds: number
  can_apply: boolean
}

export interface HubSession {
  id: string
  engine: string
  host: string
  port: number
  user?: string
  server_version: string
  free_bytes: number
  source_container?: string
  helper_network?: string
  step: HubStep
  password_held: boolean
  running: boolean
  items: HubItem[]
  summary: HubSummary
  updated_at: string
}

export interface HubSource {
  engine: string
  host: string
  port?: number
  user?: string
  password?: string
  tls?: boolean
  container?: string
}

export interface HubLocalSource {
  container: string
  image: string
  engine: string
  running: boolean
  network?: string
  host?: string
  port?: number
  problem?: string
}

export interface HubSelection {
  source_db: string
  selected?: boolean
  target_name?: string
  target_version?: string
}

export interface HubEnvVar {
  env_var: string
  field: string
  value?: string
  secret?: boolean
}

export interface HubConnection {
  database: string
  engine: string
  host: string
  port: number
  username?: string
  name?: string
  url: string
  env: HubEnvVar[]
  reference: string
  revealed: boolean
}

const base = '/api/v1/migration/hub/sessions'

export const hubKeys = {
  list: ['migration', 'hub', 'sessions'] as const,
  one: (id: string) => ['migration', 'hub', 'session', id] as const,
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

export function useHubSessions() {
  return useQuery({
    queryKey: hubKeys.list,
    queryFn: () => send<HubSession[]>(base),
  })
}

export function useHubLocalSources() {
  return useQuery({
    queryKey: ['migration', 'hub', 'local-sources'] as const,
    queryFn: () =>
      send<HubLocalSource[]>(`${base.replace('/sessions', '')}/local-sources`),
  })
}

export function useHubSession(id: string | null) {
  return useQuery({
    queryKey: hubKeys.one(id ?? ''),
    enabled: id !== null,
    queryFn: () => send<HubSession>(`${base}/${encodeURIComponent(id ?? '')}`),
    refetchInterval: (q) => (q.state.data?.running ? 2000 : false),
  })
}

function useSessionMutation<V>(
  run: (v: V) => Promise<HubSession>,
  idOf: (v: V) => string | null,
) {
  const qc = useQueryClient()
  return useMutation<HubSession, ApiError, V>({
    mutationFn: run,
    onSuccess: (s, v) => {
      qc.setQueryData(hubKeys.one(s.id), s)
      void qc.invalidateQueries({ queryKey: hubKeys.list })
      const id = idOf(v)
      if (id) void qc.invalidateQueries({ queryKey: hubKeys.one(id) })
    },
  })
}

export function useCreateHubSession() {
  return useSessionMutation<HubSource>(
    (src) => send<HubSession>(base, json('POST', src)),
    () => null,
  )
}

export function useSelectHubItems() {
  return useSessionMutation<{ id: string; items: HubSelection[] }>(
    ({ id, items }) =>
      send<HubSession>(
        `${base}/${encodeURIComponent(id)}/selection`,
        json('PUT', { items }),
      ),
    (v) => v.id,
  )
}

export function useApplyHubSession() {
  return useSessionMutation<{ id: string; password?: string }>(
    ({ id, password }) =>
      send<HubSession>(
        `${base}/${encodeURIComponent(id)}/apply`,
        json('POST', { password: password || undefined }),
      ),
    (v) => v.id,
  )
}

export function useSetHubStep() {
  return useSessionMutation<{ id: string; step: HubStep }>(
    ({ id, step }) =>
      send<HubSession>(
        `${base}/${encodeURIComponent(id)}/step`,
        json('PUT', { step }),
      ),
    (v) => v.id,
  )
}

export function useDeleteHubSession() {
  const qc = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: (id) =>
      send<void>(`${base}/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: hubKeys.list }),
  })
}

export function useHubConnection(id: string, db: string, enabled: boolean) {
  return useQuery({
    queryKey: ['migration', 'hub', 'connection', id, db] as const,
    enabled,
    queryFn: () =>
      send<HubConnection>(
        `${base}/${encodeURIComponent(id)}/items/${encodeURIComponent(db)}/connection`,
      ),
  })
}

export function useRevealHubConnection() {
  return useMutation<HubConnection, ApiError, { id: string; db: string }>({
    mutationFn: ({ id, db }) =>
      send<HubConnection>(
        `${base}/${encodeURIComponent(id)}/items/${encodeURIComponent(db)}/reveal`,
        { method: 'POST' },
      ),
  })
}

export function hubReceiptUrl(id: string): string {
  return `${base}/${encodeURIComponent(id)}/receipt`
}
