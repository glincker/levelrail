import { createFileRoute } from '@tanstack/react-router'
import { GitConnectionsPage } from '@/components/gitConnections/GitConnectionsPage'

// Account-level: every connected account across hosts, one table.
export const Route = createFileRoute('/settings/git-connections')({
  component: GitConnectionsPage,
})
