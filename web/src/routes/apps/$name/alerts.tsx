import { createFileRoute } from '@tanstack/react-router'
import { AlertRulesPanel } from '../../../components/AlertRulesPanel'

// Former "alerts" tab, now a real deep-linkable route. AlertRulesPanel
// only needs appName, no shared query-cache read required here.
export const Route = createFileRoute('/apps/$name/alerts')({
  component: AlertsSection,
})

function AlertsSection() {
  const { name } = Route.useParams()

  return <AlertRulesPanel appName={name} />
}
