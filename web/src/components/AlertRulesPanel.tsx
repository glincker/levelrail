import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { BellRingGlassIcon } from '@thesvg/ui-react'
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
  const { t } = useTranslation('alertSetup')
  if (!rule.notify_url) {
    return (
      <span className="text-muted-foreground">{t('rules.notConfigured')}</span>
    )
  }
  return (
    <div className="flex flex-col gap-0.5">
      <Badge variant="outline">{rule.notify_kind ?? t('rules.generic')}</Badge>
      <span
        className="max-w-[16rem] truncate font-mono text-xs text-muted-foreground"
        title={rule.notify_url}
      >
        {maskNotifyUrl(rule.notify_url)}
      </span>
    </div>
  )
}

const STATE_KEYS = {
  firing: 'rules.stateFiring',
  pending: 'rules.statePending',
  ok: 'rules.stateOk',
} as const satisfies Record<RuleState, string>

const COUNT_KEYS = {
  firing: 'rules.countFiring',
  pending: 'rules.countPending',
  ok: 'rules.countOk',
} as const satisfies Record<RuleState, string>

function buildColumns(
  appName: string,
  t: TFunction<'alertSetup'>,
): ResponsiveColumn<AlertRule>[] {
  return [
    {
      id: 'name',
      header: t('rules.colName'),
      primary: true,
      cell: (rule) => (
        <span className="font-medium text-foreground">
          {rule.name}
          {!rule.enabled ? (
            <Badge variant="muted" className="ml-2">
              {t('rules.disabled')}
            </Badge>
          ) : null}
        </span>
      ),
    },
    {
      id: 'state',
      header: t('rules.colState'),
      cell: (rule) => {
        const state: RuleState = ruleState(rule)
        return (
          <div className="flex flex-col items-start gap-1">
            <Badge variant={STATE_BADGE_VARIANT[state]}>
              <StateDot state={state} />
              {t(STATE_KEYS[state])}
            </Badge>
            {rule.silenced ? (
              <Badge variant="muted" title={t('rules.silencedTip')}>
                {t('rules.silenced')}
              </Badge>
            ) : null}
            {rule.kind === 'crashloop' && state === 'firing' ? (
              <Link
                to="/apps/$name/logs"
                params={{ name: appName }}
                className="text-xs text-primary underline underline-offset-2"
              >
                {t('rules.viewLogs')}
              </Link>
            ) : null}
          </div>
        )
      },
    },
    {
      id: 'kind',
      header: t('rules.colKind'),
      priority: 2,
      cell: (rule) => (
        <Badge variant={rule.kind === 'crashloop' ? 'destructive' : 'outline'}>
          {rule.kind}
        </Badge>
      ),
    },
    {
      id: 'condition',
      header: t('rules.colCondition'),
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
      header: t('rules.colLastEvaluated'),
      priority: 3,
      cell: (rule) => (
        <span className="text-muted-foreground">
          {formatDate(rule.last_evaluated_at, t('rules.never'))}
        </span>
      ),
    },
    {
      id: 'lastValue',
      header: t('rules.colLastValue'),
      priority: 3,
      cell: (rule) => (
        <span className="text-muted-foreground">{formatLastValue(rule)}</span>
      ),
    },
    {
      id: 'notify',
      header: t('rules.colNotify'),
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
  const { t } = useTranslation('alertSetup')
  const counts = useMemo(() => {
    const result: Record<RuleState, number> = { firing: 0, pending: 0, ok: 0 }
    for (const rule of rules) {
      result[ruleState(rule)] += 1
    }
    return result
  }, [rules])

  return (
    <div className="flex items-center gap-3 text-xs text-muted-foreground">
      {(Object.keys(STATE_KEYS) as RuleState[]).map((state) =>
        counts[state] > 0 ? (
          <span key={state} className="flex items-center gap-1.5">
            <StateDot state={state} />
            {t(COUNT_KEYS[state], { count: counts[state] })}
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
  const { t: ta } = useTranslation('alertSetup')
  const { data, isLoading, error } = useAlertRules(appName)
  const rules = data ?? []
  const columns = useMemo(() => buildColumns(appName, ta), [appName, ta])
  const anyFiring = rules.some((r) => ruleState(r) === 'firing')

  return (
    <section className="rounded-lg border border-border p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <BellRingGlassIcon
              className="size-5 text-muted-foreground"
              aria-hidden="true"
            />
            {ta('rules.title')}
          </h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {ta('rules.description')}
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
            icon={<BellRingGlassIcon className="size-6" />}
            title={ta('rules.emptyTitle')}
            description={ta('rules.emptyDescription')}
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
