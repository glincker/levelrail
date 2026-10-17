import { createFileRoute } from '@tanstack/react-router'
import { RuleEditorPage } from '../../../components/firewall/RuleEditorPage'
import { useFirewallNav } from '../../../components/firewall/useFirewallNav'

export const Route = createFileRoute('/firewall/rules/$id')({
  component: EditRuleRoute,
})

function EditRuleRoute() {
  const { id } = Route.useParams()
  const nav = useFirewallNav()
  return <RuleEditorPage ruleId={id} scope={nav.scope} onDone={nav.openRules} />
}
