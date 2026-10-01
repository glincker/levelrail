import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { PlugsConnectedIcon } from '@phosphor-icons/react/dist/ssr'
import { tokenListQueryOptions } from '../../queries/tokens'
import { useBrand } from '../../hooks/useBrand'
import { AgentConnectCard } from '../../components/agents/AgentConnectCard'
import { AgentTokensCard } from '../../components/agents/AgentTokensCard'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageHeader } from '@/components/shell/PageHeader'

export const Route = createFileRoute('/settings/agents')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(tokenListQueryOptions()),
  component: AgentsPage,
  pendingComponent: AgentsPending,
})

function AgentsPage() {
  const { data: tokens } = useSuspenseQuery(tokenListQueryOptions())
  const brand = useBrand()
  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <PlugsConnectedIcon className="size-4" />
        </div>
        <PageHeader
          title="Agents"
          description="Connect an AI agent to this instance through MCP, and manage the tokens it uses."
        />
      </div>
      <AgentConnectCard
        serverKey={brand.ShortName.toLowerCase()}
        binary={`${brand.BinaryName}-mcp`}
        apiURL={window.location.origin}
      />
      <AgentTokensCard tokens={tokens} />
    </div>
  )
}

function AgentsPending() {
  return (
    <div className="space-y-6">
      <PageHeader title="Agents" />
      <TableSkeleton columnCount={6} />
    </div>
  )
}
