import { createFileRoute } from '@tanstack/react-router'
import { AuditView } from '../../components/firewall/AuditView'
import { FirewallPage } from '../../components/firewall/FirewallPage'
import { useFirewallNav } from '../../components/firewall/useFirewallNav'

export const Route = createFileRoute('/firewall/audit')({
  component: FirewallAuditRoute,
})

function FirewallAuditRoute() {
  const nav = useFirewallNav()
  return (
    <FirewallPage
      tab="audit"
      scope={nav.scope}
      range={nav.range}
      onScope={nav.setScope}
      onRange={nav.setRange}
    >
      <AuditView />
    </FirewallPage>
  )
}
