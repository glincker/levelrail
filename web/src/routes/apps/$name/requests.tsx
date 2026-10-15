import { createFileRoute } from '@tanstack/react-router'
import { EdgeAccessPanel } from '../../../components/edgeAccess/EdgeAccessPanel'

// App-scoped edge access records (internal/api/access_log.go).
export const Route = createFileRoute('/apps/$name/requests')({
  component: RequestsSection,
})

function RequestsSection() {
  const { name } = Route.useParams()
  return <EdgeAccessPanel app={name} />
}
