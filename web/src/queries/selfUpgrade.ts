// Self-upgrade plan, start and attempts, and server readiness for
// Settings > Updates (internal/api/self_upgrade.go, server_readiness.go).

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { updatesKeys, type EdgeGuidance } from './updates'

export interface SelfUpgradeBreaking {
  id: string
  version: string
  summary: string
  requires_ack: boolean
}

export interface SelfUpgradeDisk {
  known: boolean
  free_bytes: number
  required_bytes: number
  db_bytes: number
  ok: boolean
}

export interface SelfUpgradeBackup {
  will_take: boolean
  dir?: string
  restore_command?: string
}

export interface SelfUpgradePlan {
  current_version: string
  target_version: string
  breaking: SelfUpgradeBreaking[]
  notes_available: boolean
  can_apply: boolean
  cannot_apply_reason?: string
  command: string
  steps: string[]
  db_schema_version?: number
  target_schema_version?: number
  pending_migrations?: number
  disk?: SelfUpgradeDisk
  downtime_seconds?: number
  restart_behaviour?: string
  backup?: SelfUpgradeBackup
  signature_pinned?: boolean
  signature_pinned_version?: string
  edge?: EdgeGuidance
}

export type AttemptOutcome =
  'running' | 'succeeded' | 'rolled_back' | 'refused' | 'failed'

export interface SelfUpgradeStep {
  name: string
  status: 'ok' | 'failed' | 'skipped'
  detail: string
  at: string
  duration_ms: number
}

export interface SelfUpgradeLogLine {
  at: string
  step: string
  line: string
}

export interface SelfUpgradeAttempt {
  id: string
  from_version: string
  to_version: string
  from_schema: number
  to_schema: number
  initiator: string
  outcome: AttemptOutcome
  failed_step: string
  error: string
  backup_name: string
  acked: string[]
  steps: SelfUpgradeStep[]
  current_step: string
  log: SelfUpgradeLogLine[]
  started_at: string
  finished_at: string
  database_restored?: boolean
  rolled_back_minutes?: number
  failure?: SelfUpgradeFailure
}

export interface SelfUpgradeFailure {
  cause: string
  primary: 'cleanup' | 'retry' | 'rollback' | 'none'
  fix_id?: string
  retryable: boolean
  rollbackable: boolean
  cli?: string
}

export type AttemptAction = 'fix' | 'retry' | 'rollback'

export function useAttemptAction() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      id: string
      action: AttemptAction
    }): Promise<{ outcome?: string; detail?: string }> => {
      const res = await fetch(
        `/api/v1/updates/self-upgrade/attempts/${encodeURIComponent(input.id)}/${input.action}`,
        { method: 'POST' },
      )
      if (!res.ok) throw await failure(res, `${input.action} upgrade attempt`)
      return (await res.json()) as { outcome?: string; detail?: string }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: selfUpgradeKeys.all }),
    onError: () =>
      queryClient.invalidateQueries({ queryKey: selfUpgradeKeys.all }),
  })
}

export interface ReadinessCheck {
  id: string
  name: string
  status: 'pass' | 'warn' | 'fail' | 'info'
  detail: string
  fix?: string
}

export interface ServerReadiness {
  checks: ReadinessCheck[]
  mode: 'own_ports' | 'behind_proxy'
  proxy?: string
  holders?: Record<string, string>
  next_step: string
  blocked: boolean
  summary: string
}

export const UPGRADE_POLL_MS = 3_000

export interface SelfUpgradeStatus {
  current_version: string
  running: SelfUpgradeAttempt | null
  last_finished: SelfUpgradeAttempt | null
  verified: boolean
}

export class UpgradeRunningError extends ApiError {
  readonly attemptId: string

  constructor(message: string, attemptId: string) {
    super(409, message)
    this.attemptId = attemptId
  }
}

export const selfUpgradeKeys = {
  all: [...updatesKeys.all, 'self-upgrade'] as const,
  plan: (target: string) =>
    [...updatesKeys.all, 'self-upgrade', 'plan', target] as const,
  attempts: () => [...updatesKeys.all, 'self-upgrade', 'attempts'] as const,
  status: () => [...updatesKeys.all, 'self-upgrade', 'status'] as const,
  readiness: (domain: string) => ['system', 'readiness', domain] as const,
}

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function selfUpgradePlanQueryOptions(target: string) {
  return queryOptions({
    queryKey: selfUpgradeKeys.plan(target),
    queryFn: async (): Promise<SelfUpgradePlan> => {
      const qs = target ? `?target=${encodeURIComponent(target)}` : ''
      const res = await fetch(`/api/v1/updates/self-upgrade/plan${qs}`)
      if (!res.ok) throw await failure(res, 'fetch upgrade plan')
      return (await res.json()) as SelfUpgradePlan
    },
    staleTime: 60_000,
    retry: false,
  })
}

export function selfUpgradeAttemptsQueryOptions(refetchMs: number | false) {
  return queryOptions({
    queryKey: selfUpgradeKeys.attempts(),
    queryFn: async (): Promise<SelfUpgradeAttempt[]> => {
      const res = await fetch('/api/v1/updates/self-upgrade/attempts')
      if (!res.ok) throw await failure(res, 'fetch upgrade attempts')
      const body = (await res.json()) as { attempts: SelfUpgradeAttempt[] }
      return body.attempts
    },
    staleTime: 5_000,
    refetchInterval: refetchMs,
    retry: true,
  })
}

export function selfUpgradeStatusQueryOptions(enabledPoll = true) {
  return queryOptions({
    queryKey: selfUpgradeKeys.status(),
    queryFn: async (): Promise<SelfUpgradeStatus> => {
      const res = await fetch('/api/v1/updates/self-upgrade/status')
      if (!res.ok) throw await failure(res, 'fetch upgrade status')
      return (await res.json()) as SelfUpgradeStatus
    },
    staleTime: 5_000,
    refetchInterval: (q) =>
      enabledPoll && q.state.data?.running ? UPGRADE_POLL_MS : false,
    retry: true,
  })
}

export function serverReadinessQueryOptions(domain: string) {
  return queryOptions({
    queryKey: selfUpgradeKeys.readiness(domain),
    queryFn: async (): Promise<ServerReadiness> => {
      const qs = domain ? `?domain=${encodeURIComponent(domain)}` : ''
      const res = await fetch(`/api/v1/system/readiness${qs}`)
      if (!res.ok) throw await failure(res, 'fetch server readiness')
      return (await res.json()) as ServerReadiness
    },
    staleTime: 30_000,
    retry: false,
  })
}

export function useStartSelfUpgrade() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (input: {
      target: string
      ack: string[]
      skip_backup?: boolean
    }): Promise<{ status: string; target_version: string }> => {
      const res = await fetch('/api/v1/updates/self-upgrade', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (res.status === 409) {
        const body = (await res
          .clone()
          .json()
          .catch(() => ({}))) as { running_attempt_id?: string }
        if (body.running_attempt_id) {
          throw new UpgradeRunningError(
            'an upgrade is already running',
            body.running_attempt_id,
          )
        }
      }
      if (!res.ok) throw await failure(res, 'start upgrade')
      return (await res.json()) as { status: string; target_version: string }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: selfUpgradeKeys.all }),
    onError: () =>
      queryClient.invalidateQueries({ queryKey: selfUpgradeKeys.all }),
  })
}
