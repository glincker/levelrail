import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { BellRingingIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { AlertTemplatePicker } from './AlertTemplatePicker'
import { CreateAlertRuleDialog } from './CreateAlertRuleDialog'
import { EditAlertRuleDialog } from './EditAlertRuleDialog'
import { LogRuleCondition } from './LogRuleCondition'
import { DeleteAlertRuleDialog } from './DeleteAlertRuleDialog'
import { RuleNoiseDialog } from './RuleNoiseDialog'
import { RecentChanges } from './RecentChanges'
import { SilenceRuleMenu } from './SilenceRuleMenu'
import { AppSloCard } from './AppSloCard'
import { SloSuggestion } from './SloSuggestion'
import {
  STATE_BADGE_VARIANT,
  STATE_LABEL,
  formatCondition,
  formatDate,
  formatLastValue,
  maskNotifyUrl,
  ruleState,
  type RuleState,
} from './alertRulesFormat'
import { StateDot } from './AlertStateDot'
import { useAlertRules } from '../queries/alerts'
import type { AlertRule } from '../types/alerts'
import type { AppVolume } from '../types/appDetail'

// Alert rules for the app detail page. A firing crashloop rule links to the
// app's own log view: the API has no endpoint returning the log lines that
// were attached to the outbound notification event.

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

function buildColumns(appName: string): ResponsiveColumn<AlertRule>[] {
  return [
    {
      id: 'name',
      header: 'Name',
      primary: true,
      cell: (rule) => (
        <span className="font-medium text-foreground">
          {rule.name}
          {!rule.enabled ? (
            <Badge variant="muted" className="ml-2">
              Disabled
            </Badge>
          ) : null}
        </span>
      ),
    },
    {
      id: 'state',
      header: 'State',
      cell: (rule) => {
        const state: RuleState = ruleState(rule)
        return (
          <div className="flex flex-col items-start gap-1">
            <Badge variant={STATE_BADGE_VARIANT[state]}>
              <StateDot state={state} />
              {STATE_LABEL[state]}
            </Badge>
            {rule.silenced ? (
              <Badge variant="muted" title="Notifications are silenced">
                Silenced
              </Badge>
            ) : null}
            {rule.kind === 'crashloop' && state === 'firing' ? (
              <Link
                to="/apps/$name/logs"
                params={{ name: appName }}
                className="text-xs text-primary underline underline-offset-2"
              >
                View logs
              </Link>
            ) : null}
          </div>
        )
      },
    },
    {
      id: 'kind',
      header: 'Kind',
      priority: 2,
      cell: (rule) => (
        <Badge variant={rule.kind === 'crashloop' ? 'destructive' : 'outline'}>
          {rule.kind}
        </Badge>
      ),
    },
    {
      id: 'condition',
      header: 'Condition',
      priority: 2,
      cell: (rule) => (
        <span className="block font-mono text-xs break-words text-muted-foreground">
          {rule.log_rule ? (
            <LogRuleCondition rule={rule} />
          ) : (
            formatCondition(rule)
          )}
        </span>
      ),
    },
    {
      id: 'lastEvaluated',
      header: 'Last evaluated',
      priority: 3,
      cell: (rule) => (
        <span className="text-muted-foreground">
          {formatDate(rule.last_evaluated_at, 'Never')}
        </span>
      ),
    },
    {
      id: 'lastValue',
      header: 'Last value',
      priority: 3,
      cell: (rule) => (
        <span className="text-muted-foreground">{formatLastValue(rule)}</span>
      ),
    },
    {
      id: 'notify',
      header: 'Notify',
      priority: 3,
      cell: (rule) => <NotifyChannelCell rule={rule} />,
    },
  ]
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

export function AlertRulesPanel({
  appName,
  volumes,
}: {
  appName: string
  volumes?: AppVolume[]
}) {
  const { t } = useTranslation('common')
  const { data, isLoading, error } = useAlertRules(appName)
  const rules = data ?? []
  const columns = useMemo(() => buildColumns(appName), [appName])
  const anyFiring = rules.some((r) => ruleState(r) === 'firing')

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
            Threshold, crashloop, certificate expiry, scheduled task failure,
            domain health, and missing backup rules over this app&apos;s
            metrics, restarts, jobs, domains, and backup schedules. Notifies via
            webhook, Slack, or Discord on a firing/resolved transition.
          </p>
        </div>
        <div className="flex items-center gap-3">
          {rules.length > 0 ? <StateSummary rules={rules} /> : null}
          {rules.length > 0 ? <AlertTemplatePicker appName={appName} /> : null}
          <CreateAlertRuleDialog appName={appName} volumes={volumes} />
        </div>
      </div>

      <AppSloCard appName={appName} rules={rules} />
      <SloSuggestion appName={appName} rules={rules} />

      <div className="mt-3 space-y-3">
        {isLoading ? (
          <TableSkeleton columnCount={8} rowCount={3} />
        ) : error ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : rules.length === 0 ? (
          <EmptyState
            icon={<BellRingingIcon className="size-5" />}
            title="No alert rules yet"
            description="Get notified the moment a metric crosses a threshold or a container starts crashlooping, via webhook, Slack, or Discord."
            action={
              <div className="flex flex-wrap items-center justify-center gap-2">
                <AlertTemplatePicker appName={appName} />
                <CreateAlertRuleDialog appName={appName} volumes={volumes} />
              </div>
            }
          />
        ) : (
          <>
            <ResponsiveTable
              rows={rules}
              columns={columns}
              getRowId={(rule) => rule.id}
              ariaLabel={t('tableLabels.alertRules')}
              renderActions={(rule) => (
                <span className="flex flex-wrap justify-end gap-2">
                  <SilenceRuleMenu
                    appName={appName}
                    ruleId={rule.id}
                    ruleName={rule.name}
                  />
                  <RuleNoiseDialog appName={appName} rule={rule} />
                  {rule.log_rule ? null : (
                    <EditAlertRuleDialog
                      appName={appName}
                      rule={rule}
                      volumes={volumes}
                    />
                  )}
                  <DeleteAlertRuleDialog appName={appName} rule={rule} />
                </span>
              )}
            />
            {anyFiring ? (
              <div className="rounded-lg border border-border bg-muted/30 p-3">
                <RecentChanges app={appName} />
              </div>
            ) : null}
          </>
        )}
      </div>
    </section>
  )
}
