import { createFileRoute } from '@tanstack/react-router'
import { ScheduledTasksPanel } from '../../../components/ScheduledTasksPanel'

// Real, deep-linkable route for the scheduled-tasks section, the same
// nested-route shape every other app-detail section (alerts.tsx,
// domains.tsx, ...) already uses.
export const Route = createFileRoute('/apps/$name/scheduled-tasks')({
  component: ScheduledTasksSection,
})

function ScheduledTasksSection() {
  const { name } = Route.useParams()
  return <ScheduledTasksPanel appName={name} />
}
