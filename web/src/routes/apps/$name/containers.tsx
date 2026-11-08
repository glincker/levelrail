import { createFileRoute } from '@tanstack/react-router'
import { AppContainersPanel } from '../../../components/apps/AppContainersPanel'

// Per-replica container details; route code splitting keeps it out of the first load.
export const Route = createFileRoute('/apps/$name/containers')({
  component: ContainersSection,
})

function ContainersSection() {
  const { name } = Route.useParams()
  return <AppContainersPanel name={name} />
}
