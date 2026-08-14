import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { ResourceLimitsEditor } from '../../../components/ResourceLimitsEditor'

// Former "resources" tab, now a real deep-linkable route. Reads app data
// from the query cache the parent layout route's loader already primed.
export const Route = createFileRoute('/apps/$name/resources')({
  component: ResourcesSection,
})

function ResourcesSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return <ResourceLimitsEditor app={app} />
}
