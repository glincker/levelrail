import { createFileRoute } from '@tanstack/react-router'
import { FirewallPage } from '../../components/firewall/FirewallPage'
import { OverviewView } from '../../components/firewall/OverviewView'
import { useFirewallActions } from '../../components/firewall/useFirewallActions'
import { useFirewallNav } from '../../components/firewall/useFirewallNav'

export const Route = createFileRoute('/firewall/')({
  component: FirewallOverviewRoute,
})

function FirewallOverviewRoute() {
  const nav = useFirewallNav()
  const actions = useFirewallActions({
    scope: nav.scope,
    onCreateRule: nav.createRuleFrom,
  })
  return (
    <FirewallPage
      tab="overview"
      scope={nav.scope}
      range={nav.range}
      onScope={nav.setScope}
      onRange={nav.setRange}
    >
      <OverviewView
        scope={nav.scope}
        range={nav.range}
        actions={actions}
        onOpenRules={nav.openRules}
        onNewRule={nav.newRule}
      />
    </FirewallPage>
  )
}
