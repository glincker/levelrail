import { createFileRoute } from '@tanstack/react-router'
import { ErrorsPanel } from '../../../components/errorTracking/ErrorsPanel'

// App-scoped errors: Sentry SDK events received for this app, grouped into
// issues (internal/api/errors_issues.go).
export const Route = createFileRoute('/apps/$name/errors')({
  component: ErrorsSection,
})

function ErrorsSection() {
  const { name } = Route.useParams()
  return <ErrorsPanel app={name} />
}
