import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
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
// own live/historical log view (LogSearchPanel, #log-search) instead of
// trying to reconstruct or fabricate the exact lines that fired. That is
// the real, buildable thing this API surface supports today.

const STATE_LABEL = {
  firing: 'Firing',
  pending: 'Pending',
  ok: 'OK',
} as const

type RuleState = keyof typeof STATE_LABEL

// No "success"/"warning" badge variant exists in badgeVariants
// (components/ui/badge.tsx), so these override its background/text
// classes directly, same as ConditionsPanel's STATUS_STYLES map and the
// amber precedent in routes/apps/$name/deploys/$deployId/logs.tsx.
const STATE_BADGE_CLASS: Record<RuleState, string> = {
  firing: 'bg-red-100 text-red-800 dark:bg-red-900/40 dark:text-red-300',
  pending:
    'bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300',
  ok: 'bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-300',
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
  return `${rule.restart_count_threshold} restarts in ${rule.restart_window ?? '?'}`
}

function formatDate(iso: string | undefined, fallback: string): string {
  return iso ? new Date(iso).toLocaleString() : fallback
}

function formatLastValue(rule: AlertRule): string {
  if (rule.last_value === undefined) {
    return '-'
  }
  return rule.kind === 'crashloop'
    ? `${rule.last_value} restarts`
    : String(rule.last_value)
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
          <Badge className={STATE_BADGE_CLASS[state]}>
            {STATE_LABEL[state]}
          </Badge>
          {showLogsLink ? (
            <a
              href="#log-search"
              className="text-xs text-primary underline underline-offset-2"
            >
              View logs
            </a>
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

export function AlertRulesPanel({ appName }: { appName: string }) {
  const { data, isLoading, error } = useAlertRules(appName)
  const rules = data ?? []

  return (
    <section className="rounded-lg border border-neutral-200 p-4 dark:border-neutral-800">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold text-neutral-900 dark:text-neutral-100">
            Alert rules
          </h2>
          <p className="mt-1 text-xs text-neutral-500 dark:text-neutral-400">
            Threshold and crashloop rules over this app&apos;s metrics and
            restarts. Notifies via webhook, Slack, or Discord on a
            firing/resolved transition.
          </p>
        </div>
        <CreateAlertRuleDialog appName={appName} />
      </div>

      <div className="mt-3">
        {isLoading ? (
          <p className="text-sm text-neutral-500 dark:text-neutral-400">
            Loading...
          </p>
        ) : error ? (
          <p className="text-sm text-red-600 dark:text-red-400">
            {error.message}
          </p>
        ) : rules.length === 0 ? (
          <p className="text-sm text-neutral-500 dark:text-neutral-400">
            No alert rules yet. Create one to get notified on a metric threshold
            or a crashloop.
          </p>
        ) : (
          <div className="rounded-lg border border-neutral-200 dark:border-neutral-800">
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
