import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { PlugsConnectedIcon } from '@phosphor-icons/react/dist/ssr'
import { tokenListQueryOptions } from '../../queries/tokens'
import { useBrand } from '../../hooks/useBrand'
import { AgentConnectCard } from '../../components/agents/AgentConnectCard'
import { AgentTokensCard } from '../../components/agents/AgentTokensCard'
import { TableSkeleton } from '@/components/ui/table-skeleton'

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
        <div>
          <h1 className="text-lg font-semibold text-foreground">Agents</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Connect an AI agent to this instance through MCP, and manage the
            tokens it uses.
          </p>
        </div>
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
      <h1 className="text-lg font-semibold text-foreground">Agents</h1>
      <TableSkeleton columnCount={6} />
    </div>
  )
}
