import { createFileRoute } from '@tanstack/react-router'
import { useDatabase } from '../../../queries/databases'
import { DatabaseResourceLimitsEditor } from '../../../components/DatabaseResourceLimitsEditor'

// Second real section for Databases, alongside overview.tsx: mirrors
// routes/apps/$name/resources.tsx exactly. Reads database data from the
// query cache the parent layout route's loader already primed.
export const Route = createFileRoute('/databases/$name/resources')({
  component: ResourcesSection,
})

function ResourcesSection() {
  const { name } = Route.useParams()
  const { data: database } = useDatabase(name)

  return <DatabaseResourceLimitsEditor database={database} />
}
