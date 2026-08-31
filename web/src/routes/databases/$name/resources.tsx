import { createFileRoute } from '@tanstack/react-router'
import { useDatabase } from '../../../queries/databases'
import { DatabaseResourceLimitsEditor } from '../../../components/DatabaseResourceLimitsEditor'
import { ResourceRecommendationCard } from '../../../components/ResourceRecommendationCard'

// Second real section for Databases, alongside overview.tsx: mirrors
// routes/apps/$name/resources.tsx exactly. Reads database data from the
// query cache the parent layout route's loader already primed.
export const Route = createFileRoute('/databases/$name/resources')({
  component: ResourcesSection,
})

function ResourcesSection() {
  const { name } = Route.useParams()
  const { data: database } = useDatabase(name)

  return (
    <div className="space-y-6">
      <ResourceRecommendationCard databaseName={name} />
      <DatabaseResourceLimitsEditor database={database} />
    </div>
  )
}
