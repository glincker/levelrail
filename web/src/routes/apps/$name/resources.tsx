import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { ResourceLimitsEditor } from '../../../components/ResourceLimitsEditor'
import { OptimisePanel } from '../../../components/resourceAdvisor/OptimisePanel'
import { LabelsEditor } from '../../../components/LabelsEditor'
import { AppCostChip } from '../../../components/usage/AppCostChip'

// Former "resources" tab, now a real deep-linkable route. Reads app data
// from the query cache the parent layout route's loader already primed.
export const Route = createFileRoute('/apps/$name/resources')({
  component: ResourcesSection,
})

function ResourcesSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="space-y-6">
      <AppCostChip appName={name} />
      <OptimisePanel appName={name} />
      <ResourceLimitsEditor app={app} />
      <LabelsEditor app={app} />
    </div>
  )
}
