import { useTranslation } from 'react-i18next'
import {
  ShieldAlertGlassIcon,
  ShieldCheckGlassIcon,
  SirenGlassIcon,
} from '@thesvg/ui-react'
import {
  useFirewallRules,
  useFirewallSeries,
  useFirewallStatus,
  useFirewallTraffic,
  type FwRange,
} from '../../queries/firewallEdge'
import { ActionChart, ActionChartSkeleton } from './ActionChart'
import { AlertsCard } from './AlertsCard'
import { BansTable } from './BansTable'
import { LoadError } from './LoadError'
import { StatusCard, StatusCardSkeleton } from './StatusCard'
import { TopList, TopListSkeleton } from './TopList'
import { TopRulesCard } from './TopRulesCard'
import { useFirewallLabels } from './useFirewallLabels'
import type { FirewallActions } from './useFirewallActions'

/** OverviewView is the firewall at a glance: state, outcomes over time, alerts, busiest rules and clients. */
export function OverviewView({
  scope,
  range,
  actions,
  onOpenRules,
  onNewRule,
}: {
  scope: string
  range: FwRange
  actions: FirewallActions
  onOpenRules: () => void
  onNewRule: () => void
}) {
  const { t } = useTranslation('firewall')
  const labels = useFirewallLabels()
  const status = useFirewallStatus(scope, range)
  const series = useFirewallSeries(scope, range)
  const rules = useFirewallRules()
  const traffic = useFirewallTraffic(scope)

  if (status.error) {
    return (
      <LoadError error={status.error} onRetry={() => void status.refetch()} />
    )
  }
  const st = status.data
  const attack = st?.attack_mode
  const spiking = st?.attack_suggestion.suggested
  const tone = attack?.active ? 'warning' : spiking ? 'warning' : 'success'

  return (
    <div className="space-y-(--glinr-page-gap)">
      <div className="grid gap-(--glinr-page-gap) lg:grid-cols-[minmax(16rem,20rem)_minmax(0,1fr)]">
        {st ? (
          <StatusCard
            tone={tone}
            icon={
              attack?.active ? (
                <SirenGlassIcon size="1.4em" />
              ) : spiking ? (
                <ShieldAlertGlassIcon
                  className="text-tone-warning"
                  size="1.4em"
                />
              ) : (
                <ShieldCheckGlassIcon
                  className="text-tone-success"
                  size="1.4em"
                />
              )
            }
            title={
              attack?.active
                ? t('status.attackTitle')
                : spiking
                  ? t('status.spikeTitle')
                  : t('status.activeTitle')
            }
            subtitle={
              attack?.active && attack.until
                ? t('status.attackUntil', {
                    time: new Date(attack.until).toLocaleTimeString(),
                  })
                : spiking
                  ? t('status.spikeSubtitle')
                  : t('status.activeSubtitle')
            }
            rows={[
              {
                label: t('status.attackMode'),
                value: attack?.active ? t('status.on') : t('status.off'),
              },
              {
                label: t('status.customRules'),
                value: `${st.rules_enabled} / ${st.rules_total}`,
              },
              {
                label: t('status.botProtection'),
                value: labels.mode[st.bot_mode],
              },
              { label: t('status.aiBots'), value: labels.mode[st.ai_bot_mode] },
            ]}
          />
        ) : (
          <StatusCardSkeleton rows={4} />
        )}
        {st && series.data ? (
          <ActionChart
            points={series.data}
            totals={st.totals}
            outcomeLabels={labels.outcome}
            chartLabel={t('chart.label', { range })}
            emptyText={t('chart.empty')}
          />
        ) : series.error ? (
          <LoadError
            error={series.error}
            onRetry={() => void series.refetch()}
          />
        ) : (
          <ActionChartSkeleton />
        )}
      </div>

      <div className="grid items-start gap-(--glinr-page-gap) lg:grid-cols-2">
        <div className="space-y-(--glinr-page-gap)">
          {st ? <AlertsCard status={st} /> : <TopListSkeleton rows={1} />}
          <BansTable
            limit={5}
            retention={
              st
                ? t('bans.retention', {
                    duration: st.retention.ban_duration,
                    strikes: st.retention.ban_strikes,
                    window: st.retention.ban_window,
                  })
                : undefined
            }
          />
        </div>
        <div className="space-y-(--glinr-page-gap)">
          {rules.data ? (
            <TopRulesCard
              rules={rules.data}
              onOpenRules={onOpenRules}
              onNewRule={onNewRule}
            />
          ) : rules.error ? (
            <LoadError
              error={rules.error}
              compact
              onRetry={() => void rules.refetch()}
            />
          ) : (
            <TopListSkeleton />
          )}
          {traffic.data ? (
            <TopList
              title={t('traffic.deniedIps')}
              rows={traffic.data.tables.denied_ip}
              mono
              empty={t('traffic.deniedEmpty')}
              menuFor={(row) => actions.menuFor('denied_ip', row.key)}
              sparkLabel={(row) => t('topList.spark', { key: row.key })}
              action={
                <span className="text-xs text-muted-foreground">
                  {t('traffic.lastHour')}
                </span>
              }
            />
          ) : traffic.error ? (
            <LoadError
              error={traffic.error}
              compact
              onRetry={() => void traffic.refetch()}
            />
          ) : (
            <TopListSkeleton />
          )}
        </div>
      </div>
      {actions.dialog}
    </div>
  )
}
