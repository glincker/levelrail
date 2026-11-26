import { createFileRoute } from '@tanstack/react-router'
import { MonitoringTabs } from '../../../components/databases/home/MonitoringTabs'
import { DatabaseSlowQueriesPanel } from '../../../components/DatabaseSlowQueriesPanel'

// Database-scoped slow query log section, the counterpart to logs.tsx/
// metrics.tsx: no loader needed, DatabaseSlowQueriesPanel owns its own
// data fetching (GET /api/v1/databases/{name}/slow-queries,
// internal/api/database_slow_queries.go).
export const Route = createFileRoute('/databases/$name/slow-queries')({
  component: SlowQueriesSection,
})

function SlowQueriesSection() {
  const { name } = Route.useParams()
  return (
    <>
      <MonitoringTabs name={name} />
      <DatabaseSlowQueriesPanel databaseName={name} />
    </>
  )
}
