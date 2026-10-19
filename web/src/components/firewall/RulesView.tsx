import { useTranslation } from 'react-i18next'
import { useFirewallRules, useFirewallStatus } from '../../queries/firewallEdge'
import { AttackModeCard } from './AttackModeCard'
import { BansTable } from './BansTable'
import { BotManagementCard } from './BotManagementCard'
import { CustomRulesCard } from './CustomRulesCard'
import { DomainLimitsCard } from './DomainLimitsCard'
import { isIpListRule } from './firewallRuleModel'
import { IpListCard } from './IpListCard'
import { LoadError } from './LoadError'
import { TopListSkeleton } from './TopList'
import type { FirewallActions } from './useFirewallActions'

/** RulesView holds everything that changes what the firewall does: IP lists, custom rules, bot handling and attack mode. */
export function RulesView({
  scope,
  actions,
  onNewRule,
  onEditRule,
}: {
  scope: string
  actions: FirewallActions
  onNewRule: () => void
  onEditRule: (id: string) => void
}) {
  const { t } = useTranslation('firewall')
  const { data: rules, error, refetch } = useFirewallRules()
  const status = useFirewallStatus('', '1h')

  if (error) return <LoadError error={error} onRetry={() => void refetch()} />
  if (!rules) {
    return (
      <div className="grid gap-(--glinr-page-gap) xl:grid-cols-2">
        <TopListSkeleton rows={4} />
        <TopListSkeleton rows={4} />
      </div>
    )
  }

  const applies = (r: { scope?: string }) =>
    !scope || !r.scope || r.scope === scope
  const visible = rules.filter(applies)
  const blocks = visible.filter((r) => isIpListRule(r) === 'deny')
  const bypass = visible.filter((r) => isIpListRule(r) === 'allow')
  const custom = visible.filter((r) => isIpListRule(r) === null)
  const ban = status.data
    ? t('bots.bansDetail', {
        strikes: status.data.retention.ban_strikes,
        window: status.data.retention.ban_window,
        duration: status.data.retention.ban_duration,
      })
    : undefined

  return (
    <div className="grid items-start gap-(--glinr-page-gap) xl:grid-cols-2">
      <div className="min-w-0 space-y-(--glinr-page-gap)">
        <CustomRulesCard
          all={rules}
          custom={custom}
          onNew={onNewRule}
          onEdit={onEditRule}
        />
        <BansTable retention={ban} />
        {scope ? <DomainLimitsCard domain={scope} /> : null}
      </div>
      <div className="min-w-0 space-y-(--glinr-page-gap)">
        <IpListCard
          kind="block"
          rules={blocks}
          scope={scope}
          actions={actions}
        />
        <IpListCard
          kind="bypass"
          rules={bypass}
          scope={scope}
          actions={actions}
        />
        <BotManagementCard banNote={ban} />
        <AttackModeCard />
      </div>
      {actions.dialog}
    </div>
  )
}
