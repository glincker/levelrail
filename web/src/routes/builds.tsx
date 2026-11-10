import { createFileRoute } from '@tanstack/react-router'
import { BuildsPage } from '../components/builds/BuildsPage'
import { parseBuildStatusFilter } from '../components/builds/buildFormat'

export const Route = createFileRoute('/builds')({
  validateSearch: (search: Record<string, unknown>): { status?: string } => {
    const status = parseBuildStatusFilter(search.status)
    return status ? { status } : {}
  },
  component: BuildsPage,
})
