import { createFileRoute } from '@tanstack/react-router'
import { FirewallPage } from '../../components/firewall/FirewallPage'
import { TrafficView } from '../../components/firewall/TrafficView'
import { useFirewallActions } from '../../components/firewall/useFirewallActions'
import { useFirewallNav } from '../../components/firewall/useFirewallNav'

export const Route = createFileRoute('/firewall/traffic')({
  component: FirewallTrafficRoute,
})

function FirewallTrafficRoute() {
  const nav = useFirewallNav()
  const actions = useFirewallActions({
    scope: nav.scope,
    onCreateRule: nav.createRuleFrom,
  })
  return (
    <FirewallPage
      tab="traffic"
      scope={nav.scope}
      range={nav.range}
      onScope={nav.setScope}
      onRange={nav.setRange}
    >
      <TrafficView scope={nav.scope} actions={actions} />
    </FirewallPage>
  )
}
