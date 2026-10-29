import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { PlugGlassIcon } from '@thesvg/ui-react'
import { tokenListQueryOptions } from '../../queries/tokens'
import { useBrand } from '../../hooks/useBrand'
import { AgentConnectCard } from '../../components/agents/AgentConnectCard'
import { AgentTokensCard } from '../../components/agents/AgentTokensCard'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'

export const Route = createFileRoute('/settings/agents')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(tokenListQueryOptions()),
  component: AgentsPage,
  pendingComponent: AgentsPending,
})

function AgentsPage() {
  const { t } = useTranslation('settings')
  const { data: tokens } = useSuspenseQuery(tokenListQueryOptions())
  const brand = useBrand()
  return (
    <PageTemplate
      icon={<PlugGlassIcon size="1.2em" />}
      title={t('agentsPage.title')}
      description={t('agentsPage.description')}
    >
      <AgentConnectCard
        serverKey={brand.ShortName.toLowerCase()}
        binary={`${brand.BinaryName}-mcp`}
        apiURL={window.location.origin}
      />
      <AgentTokensCard tokens={tokens} />
    </PageTemplate>
  )
}

function AgentsPending() {
  return (
    <div className="space-y-6">
      <SettingsPageTitle id="agents" />
      <TableSkeleton columnCount={6} />
    </div>
  )
}
