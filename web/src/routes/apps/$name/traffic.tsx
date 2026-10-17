import { createFileRoute } from '@tanstack/react-router'
import { TrafficDetailPanel } from '../../../components/trafficDetail/TrafficDetailPanel'

// App-scoped RED traffic view (internal/api/traffic_app.go).
export const Route = createFileRoute('/apps/$name/traffic')({
  component: TrafficSection,
})

function TrafficSection() {
  const { name } = Route.useParams()
  return <TrafficDetailPanel app={name} />
}
