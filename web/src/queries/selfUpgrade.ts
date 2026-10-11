// Self-upgrade plan, start and attempts, and server readiness for
// Settings > Updates (internal/api/self_upgrade.go, server_readiness.go).

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { updatesKeys } from './updates'

export interface SelfUpgradeBreaking {
  id: string
  version: string
  summary: string
  requires_ack: boolean
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
  started_at: string
  finished_at: string
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

export const selfUpgradeKeys = {
  all: [...updatesKeys.all, 'self-upgrade'] as const,
  plan: (target: string) =>
    [...updatesKeys.all, 'self-upgrade', 'plan', target] as const,
  attempts: () => [...updatesKeys.all, 'self-upgrade', 'attempts'] as const,
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
    }): Promise<{ status: string; target_version: string }> => {
      const res = await fetch('/api/v1/updates/self-upgrade', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(input),
      })
      if (!res.ok) throw await failure(res, 'start upgrade')
      return (await res.json()) as { status: string; target_version: string }
    },
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: selfUpgradeKeys.attempts() }),
  })
}
