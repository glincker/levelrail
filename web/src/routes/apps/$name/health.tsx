import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { useIsStaticSite } from '../../../queries/staticSites'
import { HealthCheckEditor } from '../../../components/HealthCheckEditor'
import { AppHealthSummary } from '../../../components/apps/AppHealthSummary'
import { AppUptimeReport } from '../../../components/apps/AppUptimeReport'
import { StaticHealth } from '../../../components/apps/StaticHealth'

// Former "health" tab, now a real deep-linkable route. Reads app data
// from the query cache the parent layout route's loader already primed.
export const Route = createFileRoute('/apps/$name/health')({
  component: HealthSection,
})

function HealthSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  const isStatic = useIsStaticSite(name)

  if (isStatic) return <StaticHealth name={name} />

  return (
    <div className="space-y-6">
      <AppHealthSummary app={app} />
      <AppUptimeReport app={name} />
      <HealthCheckEditor app={app} />
    </div>
  )
}
