import { createFileRoute } from '@tanstack/react-router'
import { AlertRulesPanel } from '../../../components/AlertRulesPanel'
import { AlertHistoryTable } from '../../../components/AlertHistoryTable'
import { DeployNotifyTargetsPanel } from '../../../components/DeployNotifyTargetsPanel'
import { useApp } from '../../../queries/apps'

// Former "alerts" tab, now a real deep-linkable route.
//
// DeployNotifyTargetsPanel (wave-2 roadmap item #5, deploy-outcome
// notifications) lives on this same route alongside AlertRulesPanel
// rather than a separate nav tab: see that panel's own doc comment for
// why. The two are backed by entirely separate backend concepts
// (internal/alerting/deploy_notify.go's own doc comment on why a deploy
// outcome doesn't reuse alert_rules), but from this page an operator
// just sees "two kinds of notification I can configure for this app."
//
// useApp is read here (not just inside AlertRulesPanel) so its own
// volumes list can feed a kind=backup_missing rule's volume picker: the
// create/edit dialogs need to offer a real choice of this app's declared
// volumes, not a free-text field.
export const Route = createFileRoute('/apps/$name/alerts')({
  component: AlertsSection,
})

function AlertsSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="flex flex-col gap-4">
      <AlertRulesPanel appName={name} volumes={app?.volumes} />
      <DeployNotifyTargetsPanel appName={name} />
      <section className="rounded-lg border border-border p-4">
        <h2 className="mb-1 text-sm font-semibold text-foreground">
          Alert history
        </h2>
        <p className="mb-3 text-xs text-muted-foreground">
          Every firing and resolution for this app, and whether its notification
          was sent, silenced, grouped, inhibited or failed.
        </p>
        <AlertHistoryTable app={name} />
      </section>
    </div>
  )
}
