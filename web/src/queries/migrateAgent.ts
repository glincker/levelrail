// Read-only migration agent sessions (internal/api/migration_agent.go).
// The install token is only ever held in the mutation result, never cached.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { MigrateChecklist } from './migrateWizard'

export type AgentState =
  'pending' | 'connected' | 'lost' | 'revoked' | 'expired'

export interface AgentSession {
  id: string
  label: string
  state: AgentState
  token_expires_at: string
  session_expires_at?: string
  last_seen_at?: string
  agent_version?: string
  created_at: string
}

export interface AgentCreated {
  id: string
  token: string
  expires_at: string
  ca_fingerprint: string
  control_plane_addr?: string
  binary: string
  command?: string
  session_max_hours: number
}

export interface AgentMount {
  type: string
  name?: string
  source?: string
  destination: string
}

export interface AgentContainer {
  id: string
  name: string
  image: string
  state: string
  mounts?: AgentMount[]
  database_engine?: string
  database_version?: string
  unlisted: boolean
}

export interface AgentInventory {
  docker_version: string
  containers: AgentContainer[]
  volumes: { name: string; driver: string }[]
  network_count: number
}

export interface AgentDetail {
  session: AgentSession
  inventory: AgentInventory
}

export type ComparisonStatus =
  | 'match'
  | 'differs'
  | 'changed_during_scan'
  | 'source_missing'
  | 'target_missing'
  | 'scan_error'

export interface AgentComparison {
  name: string
  source: string
  source_files: number
  source_bytes: number
  target_files: number
  target_bytes: number
  status: ComparisonStatus
  target_from?: string
  detail?: string
}

export interface AgentVerifyResult {
  comparisons: AgentComparison[]
  advance: { advanced: boolean; reason?: string; evidence?: string[] }
  databases_on_source: number
  checklist: MigrateChecklist
}

const base = '/api/v1/migration/agent/sessions'

async function request<T>(
  url: string,
  init: RequestInit,
  what: string,
): Promise<T> {
  const res = await fetch(url, {
    ...init,
    headers: { 'Content-Type': 'application/json' },
    cache: 'no-store',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

const LIVE_STATES: AgentState[] = ['pending', 'connected', 'lost']

export function useAgentSessions() {
  return useQuery({
    queryKey: ['migrate-agent-sessions'],
    queryFn: async ({ signal }): Promise<AgentSession[]> => {
      const body = await request<{ sessions: AgentSession[] }>(
        base,
        { signal },
        'load sessions',
      )
      return body.sessions
    },
    refetchInterval: (q) =>
      q.state.data?.some((s) => LIVE_STATES.includes(s.state)) ? 5000 : false,
  })
}

export function useAgentDetail(id: string | undefined) {
  return useQuery({
    queryKey: ['migrate-agent-detail', id],
    enabled: Boolean(id),
    queryFn: ({ signal }) =>
      request<AgentDetail>(
        `${base}/${encodeURIComponent(id ?? '')}`,
        { signal },
        'load session',
      ),
    retry: (count, err) =>
      !(
        err instanceof ApiError &&
        (err.status === 403 || err.status === 401)
      ) && count < 2,
    refetchInterval: (q) => (q.state.error ? false : 10000),
  })
}

export function useCreateAgentSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (vars: { label: string }) =>
      request<AgentCreated>(
        base,
        { method: 'POST', body: JSON.stringify(vars) },
        'create session',
      ),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: ['migrate-agent-sessions'] }),
  })
}

export function useDiscoverAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<AgentInventory>(
        `${base}/${encodeURIComponent(id)}/discover`,
        { method: 'POST' },
        'discovery',
      ),
    onSuccess: (_inv, id) =>
      qc.invalidateQueries({ queryKey: ['migrate-agent-detail', id] }),
  })
}

export function useRevokeAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<void>(
        `${base}/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'remove session',
      ),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['migrate-agent-sessions'] })
      void qc.invalidateQueries({ queryKey: ['migrate-agent-detail'] })
    },
  })
}

export function useVerifyAgent() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (vars: { id: string; app: string; ackEmpty?: boolean }) =>
      request<AgentVerifyResult>(
        `${base}/${encodeURIComponent(vars.id)}/verify`,
        {
          method: 'POST',
          body: JSON.stringify({
            app: vars.app,
            ack_empty_volumes: vars.ackEmpty === true,
          }),
        },
        'verification',
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['migrate-checklists'] }),
  })
}
