import { createFileRoute } from '@tanstack/react-router'
import { LogSearchPanel } from '../../../components/LogSearchPanel'

// Former "logs" tab (searchable log history, distinct from the live
// build-log stream at /apps/$name/deploys/$deployId/logs), now a real
// deep-linkable route. LogSearchPanel only needs appName, no shared
// query-cache read required here.
export const Route = createFileRoute('/apps/$name/logs')({
  component: LogsSection,
})

function LogsSection() {
  const { name } = Route.useParams()

  return <LogSearchPanel appName={name} />
}
