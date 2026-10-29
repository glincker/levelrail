import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { FirewallPage } from '../../../components/firewall/FirewallPage'
import { RulesView } from '../../../components/firewall/RulesView'
import { useFirewallActions } from '../../../components/firewall/useFirewallActions'
import { useFirewallNav } from '../../../components/firewall/useFirewallNav'

export const Route = createFileRoute('/firewall/rules/')({
  component: FirewallRulesRoute,
})

function FirewallRulesRoute() {
  const { t } = useTranslation('firewall')
  const nav = useFirewallNav()
  const actions = useFirewallActions({
    scope: nav.scope,
    onCreateRule: nav.createRuleFrom,
  })
  return (
    <FirewallPage
      tab="rules"
      scope={nav.scope}
      range={nav.range}
      onScope={nav.setScope}
      onRange={nav.setRange}
      actions={
        <Button type="button" onClick={nav.newRule}>
          <PlusGlassIcon size="1.4em" />
          {t('rules.new')}
        </Button>
      }
    >
      <RulesView
        scope={nav.scope}
        actions={actions}
        onNewRule={nav.newRule}
        onEditRule={nav.editRule}
      />
    </FirewallPage>
  )
}
