import type { VariantProps } from 'class-variance-authority'
import type { badgeVariants } from '@/components/ui/badge'
import type { AlertRule } from '../types/alerts'

export const STATE_LABEL = {
  firing: 'Firing',
  pending: 'Pending',
  ok: 'OK',
} as const

export type RuleState = keyof typeof STATE_LABEL

// Sourced from the shared destructive/warning/success variants in
// badgeVariants (components/ui/badge.tsx) rather than a locally hand-rolled
// class string, same as ConditionsPanel's STATUS_BADGE_VARIANT map.
export const STATE_BADGE_VARIANT: Record<
  RuleState,
  VariantProps<typeof badgeVariants>['variant']
> = {
  firing: 'destructive',
  pending: 'warning',
  ok: 'success',
}

// firing implies a non-null pending_since too (advanceState,
// internal/alerting/evaluate.go, keeps the original PendingSince set
// once a rule transitions to firing rather than clearing it), so firing
// is checked first: pending_since alone only means "pending" once
// firing is ruled out.
export function ruleState(rule: AlertRule): RuleState {
  if (rule.firing) {
    return 'firing'
  }
  if (rule.pending_since) {
    return 'pending'
  }
  return 'ok'
}

export function formatCondition(rule: AlertRule): string {
  if (rule.kind === 'threshold') {
    const forPart = rule.for_duration ? ` for ${rule.for_duration}` : ''
    return `${rule.metric ?? '?'} ${rule.comparator ?? '?'} ${rule.threshold}${forPart}`
  }
  if (rule.kind === 'cert_expiry') {
    return 'any certificate expiring soon or expired (platform-wide)'
  }
  if (rule.kind === 'patch_status') {
    return 'any node over its pending security-patch threshold (platform-wide)'
  }
  if (rule.kind === 'node_disk_space') {
    return 'any node over its disk-usage percentage threshold (platform-wide)'
  }
  if (rule.kind === 'node_offline') {
    return 'any node offline (platform-wide)'
  }
  if (rule.kind === 'node_cert_expiring') {
    return "any node's agent certificate close to expiry or expired (platform-wide)"
  }
  if (rule.kind === 'budget_threshold') {
    return 'any project or team budget at its warning level, over its limit or forecast to overrun'
  }
  if (rule.kind === 'version_skew') {
    return "running build behind the configured update channel's latest release"
  }
  if (rule.kind === 'node_resource_usage') {
    return 'any node over its summed CPU or memory threshold (platform-wide)'
  }
  if (rule.kind === 'scheduled_task_failure') {
    return `job ${rule.scheduled_task_id ?? '?'} fails ${rule.restart_count_threshold} runs in a row`
  }
  if (rule.kind === 'scheduled_task_missed') {
    return `job ${rule.scheduled_task_id ?? '?'} misses scheduled runs`
  }
  if (rule.kind === 'scheduled_task_running_long') {
    return `job ${rule.scheduled_task_id ?? '?'} runs past its warning threshold`
  }
  if (rule.kind === 'domain_health') {
    const forPart = rule.for_duration ? ` for ${rule.for_duration}` : ''
    return `any of this app's own domains not resolving correctly or pointing elsewhere${forPart}`
  }
  if (rule.kind === 'slo_burn') {
    const slo = rule.slo
    if (!slo) return 'SLO burn rate'
    const what =
      slo.objective === 'latency'
        ? `${slo.target}% of requests under ${slo.latency_ms ?? '?'}ms`
        : `${slo.target}% availability`
    return `${what}, alerts on 30 day error budget burn`
  }
  if (rule.kind === 'log_archive_stale') {
    return `a log archive policy failing or not succeeding within ${rule.for_duration || 'its default age limit'} (platform-wide)`
  }
  if (rule.kind === 'control_plane_backup_stale') {
    return `control plane's newest snapshot older than ${rule.for_duration || '3d (default)'} (platform-wide)`
  }
  if (rule.kind === 'database_disk_high') {
    return `${rule.backup_database_name ?? '?'} data disk at or above ${rule.threshold || 85}%`
  }
  if (rule.kind === 'database_connections_high') {
    return `${rule.backup_database_name ?? '?'} connections at or above ${rule.threshold || 80}% of the limit`
  }
  if (rule.kind === 'database_down') {
    return `${rule.backup_database_name ?? '?'} not accepting connections`
  }
  if (rule.kind === 'database_backup_stale') {
    return `${rule.backup_database_name ?? '?'} last good backup older than ${rule.threshold || 26}h`
  }
  if (rule.kind === 'backup_missing') {
    const target =
      rule.backup_resource_kind === 'volume'
        ? `${rule.backup_service_name ?? '?'}/${rule.backup_volume_name ?? '?'}`
        : (rule.backup_database_name ?? '?')
    const grace = rule.for_duration || '6h (default)'
    return `${target} has no successful backup within its schedule plus ${grace}`
  }
  return `${rule.restart_count_threshold} restarts in ${rule.restart_window ?? '?'}`
}

export function formatDate(iso: string | undefined, fallback: string): string {
  return iso ? new Date(iso).toLocaleString() : fallback
}

export function formatLastValue(rule: AlertRule): string {
  if (rule.last_value === undefined) {
    return '-'
  }
  if (rule.kind === 'crashloop') {
    return `${rule.last_value} restarts`
  }
  if (rule.kind === 'cert_expiry') {
    return `${rule.last_value.toFixed(1)} days to earliest expiry`
  }
  if (rule.kind === 'patch_status') {
    return `${rule.last_value} security patches (highest across nodes)`
  }
  if (rule.kind === 'node_disk_space') {
    return `${rule.last_value.toFixed(1)}% disk used (highest across nodes)`
  }
  if (rule.kind === 'node_offline') {
    return `${rule.last_value} node(s) offline`
  }
  if (rule.kind === 'node_cert_expiring') {
    return `${rule.last_value.toFixed(1)} days left (soonest node certificate)`
  }
  if (rule.kind === 'version_skew') {
    return rule.last_value ? 'update available' : 'up to date'
  }
  if (rule.kind === 'budget_threshold') {
    return `${rule.last_value.toFixed(0)}% of the most used budget`
  }
  if (rule.kind === 'node_resource_usage') {
    // Only the CPU signal is tracked here; memory has no comparable unit
    // to take a single cross-node max with, see EvaluateNodeResourceUsage's
    // own doc comment.
    return `${rule.last_value.toFixed(1)}% CPU used (highest across nodes)`
  }
  if (rule.kind === 'scheduled_task_failure') {
    return `${rule.last_value} consecutive failures`
  }
  if (rule.kind === 'scheduled_task_missed') {
    return `${rule.last_value} missed runs in a row`
  }
  if (rule.kind === 'scheduled_task_running_long') {
    return `running for ${Math.round(rule.last_value)}s`
  }
  if (rule.kind === 'domain_health') {
    return `${rule.last_value} domain(s) unhealthy`
  }
  if (rule.kind === 'slo_burn') {
    return `${rule.last_value.toFixed(1)}x burn rate (fastest tier)`
  }
  if (rule.kind === 'log_archive_stale') {
    return `${rule.last_value} log archive policy(ies) unhealthy`
  }
  if (rule.kind === 'control_plane_backup_stale') {
    return `${rule.last_value.toFixed(1)}h since newest control plane snapshot`
  }
  if (
    rule.kind === 'database_disk_high' ||
    rule.kind === 'database_connections_high'
  ) {
    return `${rule.last_value.toFixed(0)}%`
  }
  if (rule.kind === 'database_backup_stale') {
    return `${rule.last_value.toFixed(1)}h since last successful backup`
  }
  if (rule.kind === 'backup_missing') {
    return `${rule.last_value.toFixed(1)}h since last successful backup`
  }
  return String(rule.last_value)
}

// Shows enough of notify_url to identify the destination (host, plus a
// masked path) without printing the full URL: a webhook URL frequently
// embeds a bearer secret in its path (Slack/Discord incoming webhooks
// both do exactly this), so this panel never renders one in full.
export function maskNotifyUrl(url: string): string {
  try {
    const parsed = new URL(url)
    return `${parsed.host}/••••`
  } catch {
    return '••••'
  }
}
