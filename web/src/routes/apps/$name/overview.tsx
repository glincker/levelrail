import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { useDeployStatus } from '../../../queries/deploys'
import { AppOverview } from '../../../components/AppOverview'
import { ConditionsPanel } from '../../../components/ConditionsPanel'

// Former "overview" tab of routes/apps/$name.tsx's Tabs component, now a
// real deep-linkable route. Reads app/conditions from the same query
// cache the parent layout route's loader already primed (queries/apps.ts,
// queries/deploys.ts), no fetch of its own.
export const Route = createFileRoute('/apps/$name/overview')({
  component: OverviewSection,
})

function OverviewSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)
  const { data: conditions } = useDeployStatus(name)

  return (
    <div className="space-y-6">
      <AppOverview app={app} />
      <ConditionsPanel conditions={conditions} />
    </div>
  )
}
