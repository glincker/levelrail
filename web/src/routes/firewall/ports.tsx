import { createFileRoute } from '@tanstack/react-router'
import { FirewallPage } from '../../components/firewall/FirewallPage'
import { PortsView } from '../../components/firewall/PortsView'
import { useFirewallNav } from '../../components/firewall/useFirewallNav'

export const Route = createFileRoute('/firewall/ports')({
  component: FirewallPortsRoute,
})

function FirewallPortsRoute() {
  const nav = useFirewallNav()
  return (
    <FirewallPage
      tab="ports"
      scope={nav.scope}
      range={nav.range}
      onScope={nav.setScope}
      onRange={nav.setRange}
    >
      <PortsView />
    </FirewallPage>
  )
}
