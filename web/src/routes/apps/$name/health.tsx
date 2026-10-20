import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { HealthCheckEditor } from '../../../components/HealthCheckEditor'

// Former "health" tab, now a real deep-linkable route. Reads app data
// from the query cache the parent layout route's loader already primed.
export const Route = createFileRoute('/apps/$name/health')({
  component: HealthSection,
})

function HealthSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return <HealthCheckEditor app={app} />
}
