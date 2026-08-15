import { createFileRoute } from '@tanstack/react-router'
import { AlertRulesPanel } from '../../../components/AlertRulesPanel'
import { DeployNotifyTargetsPanel } from '../../../components/DeployNotifyTargetsPanel'

// Former "alerts" tab, now a real deep-linkable route. Both panels only
// need appName, no shared query-cache read required here.
//
// DeployNotifyTargetsPanel (wave-2 roadmap item #5, deploy-outcome
// notifications) lives on this same route alongside AlertRulesPanel
// rather than a separate nav tab: see that panel's own doc comment for
// why. The two are backed by entirely separate backend concepts
// (internal/alerting/deploy_notify.go's own doc comment on why a deploy
// outcome doesn't reuse alert_rules), but from this page an operator
// just sees "two kinds of notification I can configure for this app."
export const Route = createFileRoute('/apps/$name/alerts')({
  component: AlertsSection,
})

function AlertsSection() {
  const { name } = Route.useParams()

  return (
    <div className="flex flex-col gap-4">
      <AlertRulesPanel appName={name} />
      <DeployNotifyTargetsPanel appName={name} />
    </div>
  )
}
