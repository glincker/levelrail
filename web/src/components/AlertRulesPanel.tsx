import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { BellRingingIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { VariantProps } from 'class-variance-authority'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { CreateAlertRuleDialog } from './CreateAlertRuleDialog'
import { DeleteAlertRuleDialog } from './DeleteAlertRuleDialog'
import { useAlertRules } from '../queries/alerts'
import type { AlertRule } from '../types/alerts'

// Alert rules section for the app detail page (TASKS.md 2.5/2.7),
// wired against GET/POST/DELETE /api/v1/apps/{name}/alerts. This is the
// dashboard-side half of the gap TASKS.md 2.7 leaves explicit: the
// backend already evaluates threshold and crashloop rules and dispatches
// webhook/Slack/Discord notifications, but until this panel there was no
// way to see or manage a rule without leaving the dashboard.
//
// One honest gap this panel does NOT paper over: TASKS.md 2.7 also says
// the last 200 lines of a crashlooping container's logs get attached to
// the *notification event* (internal/alerting's Engine.dispatch,
// fetchRecentLogLines), sent to whatever notify_url is configured. There
// is no API endpoint that returns "the log lines attached to the most
// recent firing event" back to this dashboard, only to the outbound
// webhook payload, so a firing crashloop rule below links to this app's
// own live/historical log view (/apps/$name/logs) instead of trying to
// reconstruct or fabricate the exact lines that fired. That is the real,
// buildable thing this API surface supports today.

const STATE_LABEL = {
  firing: 'Firing',
  pending: 'Pending',
  ok: 'OK',
} as const

type RuleState = keyof typeof STATE_LABEL

// Sourced from the shared destructive/warning/success variants in
// badgeVariants (components/ui/badge.tsx) rather than a locally hand-rolled
// class string, same as ConditionsPanel's STATUS_BADGE_VARIANT map.
const STATE_BADGE_VARIANT: Record<
  RuleState,
  VariantProps<typeof badgeVariants>['variant']
> = {
  firing: 'destructive',
  pending: 'warning',
  ok: 'success',
}

// The solid dot color that pairs with STATE_BADGE_CLASS above, used both
// inside each row's state badge and in the header's firing/pending/ok
// summary counts, so a rule's state reads as a color before the label
// text is even parsed.
const STATE_DOT_CLASS: Record<RuleState, string> = {
  firing: 'bg-red-600 dark:bg-red-400',
  pending: 'bg-amber-600 dark:bg-amber-400',
  ok: 'bg-green-600 dark:bg-green-400',
}

// A firing rule gets a pulsing dot, not just a static one: it is the
// state most likely to need immediate attention, and a still dashboard
// full of quiet badges is exactly the "bolted on" observability feel
// this project set out to avoid, the kind Coolify/Dokploy have.
// Pending/ok stay static so the motion budget on this page is spent
// only where it is meaningful.
function StateDot({ state }: { state: RuleState }) {
  return (
    <span className="relative inline-flex size-2 shrink-0" aria-hidden="true">
      {state === 'firing' ? (
        <span
          className={`absolute inline-flex size-full animate-ping rounded-full opacity-75 ${STATE_DOT_CLASS[state]}`}
        />
      ) : null}
      <span
        className={`relative inline-flex size-2 rounded-full ${STATE_DOT_CLASS[state]}`}
      />
    </span>
  )
}

// firing implies a non-null pending_since too (advanceState,
// internal/alerting/evaluate.go, keeps the original PendingSince set
// once a rule transitions to firing rather than clearing it), so firing
// is checked first: pending_since alone only means "pending" once
// firing is ruled out.
function ruleState(rule: AlertRule): RuleState {
  if (rule.firing) {
    return 'firing'
  }
  if (rule.pending_since) {
    return 'pending'
  }
  return 'ok'
}

function formatCondition(rule: AlertRule): string {
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
  if (rule.kind === 'node_resource_usage') {
    return 'any node over its summed CPU or memory threshold (platform-wide)'
  }
  if (rule.kind === 'scheduled_task_failure') {
    return `task ${rule.scheduled_task_id ?? '?'} fails ${rule.restart_count_threshold} runs in a row`
  }
  if (rule.kind === 'domain_health') {
    const forPart = rule.for_duration ? ` for ${rule.for_duration}` : ''
    return `any of this app's own domains not resolving correctly or pointing elsewhere${forPart}`
  }
  return `${rule.restart_count_threshold} restarts in ${rule.restart_window ?? '?'}`
}

function formatDate(iso: string | undefined, fallback: string): string {
  return iso ? new Date(iso).toLocaleString() : fallback
}

function formatLastValue(rule: AlertRule): string {
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
  if (rule.kind === 'node_resource_usage') {
    // Only the CPU signal is tracked here; memory has no comparable unit
    // to take a single cross-node max with, see EvaluateNodeResourceUsage's
    // own doc comment.
    return `${rule.last_value.toFixed(1)}% CPU used (highest across nodes)`
  }
  if (rule.kind === 'scheduled_task_failure') {
    return `${rule.last_value} consecutive failures`
  }
  if (rule.kind === 'domain_health') {
    return `${rule.last_value} domain(s) unhealthy`
  }
  return String(rule.last_value)
}

// Shows enough of notify_url to identify the destination (host, plus a
// masked path) without printing the full URL: a webhook URL frequently
// embeds a bearer secret in its path (Slack/Discord incoming webhooks
// both do exactly this), so this panel never renders one in full.
function maskNotifyUrl(url: string): string {
  try {
    const parsed = new URL(url)
    return `${parsed.host}/••••`
  } catch {
    return '••••'
  }
}

function NotifyChannelCell({ rule }: { rule: AlertRule }) {
  if (!rule.notify_url) {
    return <span className="text-muted-foreground">Not configured</span>
  }
  return (
    <div className="flex flex-col gap-0.5">
      <Badge variant="outline">{rule.notify_kind ?? 'generic'}</Badge>
      <span
        className="max-w-[16rem] truncate font-mono text-xs text-muted-foreground"
        title={rule.notify_url}
      >
        {maskNotifyUrl(rule.notify_url)}
      </span>
    </div>
  )
}

function RuleRow({ appName, rule }: { appName: string; rule: AlertRule }) {
  const state = ruleState(rule)
  const showLogsLink = rule.kind === 'crashloop' && state === 'firing'

  return (
    <TableRow>
      <TableCell className="font-medium text-foreground">
        {rule.name}
        {!rule.enabled ? (
          <Badge variant="muted" className="ml-2">
            Disabled
          </Badge>
        ) : null}
      </TableCell>
      <TableCell>
        <Badge variant={rule.kind === 'crashloop' ? 'destructive' : 'outline'}>
          {rule.kind}
        </Badge>
      </TableCell>
      <TableCell className="max-w-[20rem] truncate font-mono text-xs text-muted-foreground">
        {formatCondition(rule)}
      </TableCell>
      <TableCell>
        <div className="flex flex-col gap-1">
          <Badge variant={STATE_BADGE_VARIANT[state]}>
            <StateDot state={state} />
            {STATE_LABEL[state]}
          </Badge>
          {showLogsLink ? (
            <Link
              to="/apps/$name/logs"
              params={{ name: appName }}
              className="text-xs text-primary underline underline-offset-2"
            >
              View logs
            </Link>
          ) : null}
        </div>
      </TableCell>
      <TableCell className="text-muted-foreground">
        {formatDate(rule.last_evaluated_at, 'Never')}
      </TableCell>
      <TableCell className="text-muted-foreground">
        {formatLastValue(rule)}
      </TableCell>
      <TableCell>
        <NotifyChannelCell rule={rule} />
      </TableCell>
      <TableCell className="text-right">
        <DeleteAlertRuleDialog appName={appName} rule={rule} />
      </TableCell>
    </TableRow>
  )
}

// Small "N firing / N pending / N ok" chips above the table, so a rule's
// state is scannable at a glance without reading every row, the same
// summary-before-detail shape alerting UIs (Grafana's alert list,
// PagerDuty) use.
function StateSummary({ rules }: { rules: AlertRule[] }) {
  const counts = useMemo(() => {
    const result: Record<RuleState, number> = { firing: 0, pending: 0, ok: 0 }
    for (const rule of rules) {
      result[ruleState(rule)] += 1
    }
    return result
  }, [rules])

  return (
    <div className="flex items-center gap-3 text-xs text-muted-foreground">
      {(Object.keys(STATE_LABEL) as RuleState[]).map((state) =>
        counts[state] > 0 ? (
          <span key={state} className="flex items-center gap-1.5">
            <StateDot state={state} />
            {counts[state]} {STATE_LABEL[state].toLowerCase()}
          </span>
        ) : null,
      )}
    </div>
  )
}

export function AlertRulesPanel({ appName }: { appName: string }) {
  const { data, isLoading, error } = useAlertRules(appName)
  const rules = data ?? []

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <BellRingingIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Alert rules
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Threshold, crashloop, certificate expiry, scheduled task
            failure, and domain health rules over this app&apos;s metrics,
            restarts, jobs, and domains. Notifies via webhook, Slack, or
            Discord on a firing/resolved transition.
          </p>
        </div>
        <div className="flex items-center gap-3">
          {rules.length > 0 ? <StateSummary rules={rules} /> : null}
          <CreateAlertRuleDialog appName={appName} />
        </div>
      </div>

      <div className="mt-3">
        {isLoading ? (
          <TableSkeleton columnCount={8} rowCount={3} />
        ) : error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : rules.length === 0 ? (
          <EmptyState
            icon={<BellRingingIcon className="size-5" />}
            title="No alert rules yet"
            description="Get notified the moment a metric crosses a threshold or a container starts crashlooping, via webhook, Slack, or Discord."
            action={<CreateAlertRuleDialog appName={appName} />}
          />
        ) : (
          <div className="rounded-lg border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Kind</TableHead>
                  <TableHead>Condition</TableHead>
                  <TableHead>State</TableHead>
                  <TableHead>Last evaluated</TableHead>
                  <TableHead>Last value</TableHead>
                  <TableHead>Notify</TableHead>
                  <TableHead className="text-right">Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rules.map((rule) => (
                  <RuleRow key={rule.id} appName={appName} rule={rule} />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </section>
  )
}
