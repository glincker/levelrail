import { HEALTH_CHECK_DEFAULT_PATH } from '../../lib/healthCheckDefaults'
import type { DomainCheckStatus } from '../../queries/domainCheck'
import type { PendingChanges } from '../../queries/appTimeline'
import type { TrafficStats } from '../../queries/appTraffic'
import type { AppDetail } from '../../types/appDetail'
import type { DeployAttemptStatus } from '../../types/deployAttempt'
import { ERROR_RATE_DANGER, MEMORY_HIGH_RATIO } from './config'
import type { Tone } from '@/components/kit'

export const MAX_SUGGESTIONS = 3

export type SuggestionActionKind =
  | 'add_health'
  | 'restart'
  | 'apply_pending'
  | 'open_logs'
  | 'show_fix'
  | 'open_deploy'
  | 'open_domains'
  | 'open_resources'
  | 'connect_git'

export interface SuggestionDescriptor {
  id: string
  priority: number
  tone: Tone
  title: string
  detail?: string
  action: { kind: SuggestionActionKind; label: string }
}

export interface SuggestionInput {
  app: Pick<
    AppDetail,
    'health' | 'resources' | 'domains' | 'env_dirty' | 'suspended'
  >
  pending?: PendingChanges | null
  traffic?: TrafficStats
  memory?: { usage: number; limit: number }
  latestAttemptStatus?: DeployAttemptStatus
  topCauseTitle?: string
  domainStatus?: DomainCheckStatus
  gitSourceKnown: boolean
  hasGitSource: boolean
  dismissed: ReadonlySet<string>
}

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`
}

function pendingCount(pending: PendingChanges): number {
  const keys = pending.changes.reduce((s, c) => s + (c.keys?.length ?? 1), 0)
  return Math.max(keys, 1)
}

export function computeSuggestions(
  input: SuggestionInput,
): SuggestionDescriptor[] {
  const { app } = input
  const out: SuggestionDescriptor[] = []

  if (input.latestAttemptStatus === 'failed') {
    out.push({
      id: 'failed_deploy',
      priority: 100,
      tone: 'danger',
      title: input.topCauseTitle
        ? `Last deploy failed: ${input.topCauseTitle}`
        : 'Last deploy failed',
      action: input.topCauseTitle
        ? { kind: 'show_fix', label: 'See the fix' }
        : { kind: 'open_deploy', label: 'View deploy' },
    })
  }

  const errorRate = input.traffic?.current.errorRate ?? 0
  if (
    input.traffic &&
    input.traffic.current.requests > 0 &&
    errorRate >= ERROR_RATE_DANGER
  ) {
    out.push({
      id: 'high_errors',
      priority: 90,
      tone: 'danger',
      title: `${(errorRate * 100).toFixed(1)}% of requests are failing`,
      action: { kind: 'open_logs', label: 'Open logs' },
    })
  }

  if (input.pending?.pending) {
    const restart = input.pending.apply_action === 'restart'
    out.push({
      id: 'pending_changes',
      priority: 80,
      tone: 'warning',
      title: `${plural(pendingCount(input.pending), 'change')} not applied`,
      action: {
        kind: 'apply_pending',
        label: restart ? 'Restart to apply' : 'Redeploy to apply',
      },
    })
  } else if (app.env_dirty) {
    out.push({
      id: 'pending_changes',
      priority: 80,
      tone: 'warning',
      title: 'Env changes not applied',
      action: { kind: 'restart', label: 'Restart to apply' },
    })
  }

  const hasProbe = Boolean(app.health?.readiness || app.health?.liveness)
  if (!hasProbe && !app.suspended) {
    out.push({
      id: 'no_health_check',
      priority: 60,
      tone: 'warning',
      title: 'No health check',
      detail: `${HEALTH_CHECK_DEFAULT_PATH} is the usual path`,
      action: {
        kind: 'add_health',
        label: `Add ${HEALTH_CHECK_DEFAULT_PATH}`,
      },
    })
  }

  const noLimit = !app.resources?.memory_bytes
  if (
    noLimit &&
    input.memory &&
    input.memory.limit > 0 &&
    input.memory.usage / input.memory.limit >= MEMORY_HIGH_RATIO
  ) {
    out.push({
      id: 'memory_no_limit',
      priority: 50,
      tone: 'warning',
      title: `Memory at ${Math.round((input.memory.usage / input.memory.limit) * 100)}% with no limit`,
      action: { kind: 'open_resources', label: 'Set a limit' },
    })
  }

  const primary = app.domains?.[0]
  if (
    primary &&
    (input.domainStatus === 'not_resolving' ||
      input.domainStatus === 'resolves_elsewhere')
  ) {
    out.push({
      id: 'domain_not_serving',
      priority: 40,
      tone: 'warning',
      title: `${primary} is not pointing here yet`,
      action: { kind: 'open_domains', label: 'Check DNS' },
    })
  }

  if (input.gitSourceKnown && !input.hasGitSource) {
    out.push({
      id: 'no_git_source',
      priority: 10,
      tone: 'info',
      title: 'Deploy on every push',
      detail: 'No git source connected',
      action: { kind: 'connect_git', label: 'Connect git' },
    })
  }

  return out
    .filter((s) => !input.dismissed.has(s.id))
    .sort((a, b) => b.priority - a.priority)
    .slice(0, MAX_SUGGESTIONS)
}

export interface SetupItem {
  id: 'domain' | 'health' | 'git' | 'limits'
  label: string
  done: boolean
}

/** Actionable setup steps only; a step is done when configured. */
export function computeSetup(input: {
  app: Pick<AppDetail, 'health' | 'resources' | 'domains'>
  hasGitSource: boolean
}): { items: SetupItem[]; done: number } {
  const { app } = input
  const items: SetupItem[] = [
    {
      id: 'domain',
      label: 'Connect a domain',
      done: (app.domains?.length ?? 0) > 0,
    },
    {
      id: 'health',
      label: 'Add a health check',
      done: Boolean(app.health?.readiness || app.health?.liveness),
    },
    { id: 'git', label: 'Connect a git source', done: input.hasGitSource },
    {
      id: 'limits',
      label: 'Set resource limits',
      done: Boolean(app.resources?.memory_bytes || app.resources?.nano_cpus),
    },
  ]
  return { items, done: items.filter((i) => i.done).length }
}
