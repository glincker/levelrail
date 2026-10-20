import { createFileRoute } from '@tanstack/react-router'
import { ScheduledTasksPanel } from '../../../components/ScheduledTasksPanel'

// Real deep-linkable nested route, matching alerts.tsx/domains.tsx's own
// shape: routes/apps/$name.tsx's layout owns the loader/header, this
// file only renders the section itself. No shared query-cache read
// needed here beyond appName.
export const Route = createFileRoute('/apps/$name/scheduled-tasks')({
  component: ScheduledTasksSection,
})

function ScheduledTasksSection() {
  const { name } = Route.useParams()
  return <ScheduledTasksPanel appName={name} />
}
