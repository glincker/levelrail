import { createFileRoute } from '@tanstack/react-router'
import { VisitorsSection } from '../../../components/trafficDetail/VisitorsSection'
import { TrafficDetailPanel } from '../../../components/trafficDetail/TrafficDetailPanel'

// App-scoped RED traffic view (internal/api/traffic_app.go).
export const Route = createFileRoute('/apps/$name/traffic')({
  component: TrafficSection,
})

function TrafficSection() {
  const { name } = Route.useParams()
  return (
    <div className="space-y-4">
      <VisitorsSection app={name} />
      <TrafficDetailPanel app={name} />
    </div>
  )
}
