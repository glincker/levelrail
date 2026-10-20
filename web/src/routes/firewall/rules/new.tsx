import { createFileRoute } from '@tanstack/react-router'
import { RuleEditorPage } from '../../../components/firewall/RuleEditorPage'
import {
  useFirewallNav,
  validateRuleSearch,
} from '../../../components/firewall/useFirewallNav'

export const Route = createFileRoute('/firewall/rules/new')({
  validateSearch: validateRuleSearch,
  component: NewRuleRoute,
})

function NewRuleRoute() {
  const nav = useFirewallNav()
  return (
    <RuleEditorPage seed={nav.seed} scope={nav.scope} onDone={nav.openRules} />
  )
}
