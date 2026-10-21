import { createFileRoute } from '@tanstack/react-router'
import { StaticRoutesPanel } from '../../../components/static/StaticRoutesPanel'

// Routing rules of a build.type: static site (internal/api/static_routes.go).
export const Route = createFileRoute('/apps/$name/static-routes')({
  component: StaticRoutesSection,
})

function StaticRoutesSection() {
  const { name } = Route.useParams()
  return <StaticRoutesPanel app={name} />
}
