import { createFileRoute } from '@tanstack/react-router'
import { TracesPanel } from '../../../components/traces/TracesPanel'

// App-scoped traces: OTLP spans received for this app, a list with filters
// and a waterfall per trace (internal/api/traces.go).
export const Route = createFileRoute('/apps/$name/traces')({
  component: TracesSection,
})

function TracesSection() {
  const { name } = Route.useParams()
  return <TracesPanel app={name} />
}
