import { createFileRoute } from '@tanstack/react-router'
import { CloudIcon } from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { pickStringSearch } from '../../lib/urlDialog'
import { nodeProviderListQueryOptions } from '../../queries/nodeProvision'
import { NodeProviderCredentialsCard } from '../../components/NodeProviderCredentialsCard'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'

// Instance-level, not scoped to one app: lives under routes/settings/
// next to registry-credentials.tsx and backup-targets.tsx, the same
// "credential used by name/kind from elsewhere" reasoning those files'
// own header comments give. Reachable directly here and via a link from
// the Nodes page's own Add node wizard when a provider has no stored
// token yet.
export const Route = createFileRoute('/settings/node-providers')({
  validateSearch: (
    search: Record<string, unknown>,
  ): { connect?: string; method?: string } =>
    pickStringSearch(search, ['connect', 'method']),
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(nodeProviderListQueryOptions()),
  component: NodeProvidersSettingsPage,
  pendingComponent: NodeProvidersSettingsSkeleton,
})

// A fixed approximation of the real providers.map grid, matching
// OAuthSettingsSkeleton's own comment for the same reasoning.
function NodeProvidersSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <div className="grid gap-4 sm:grid-cols-2">
        <SettingsCardSkeleton description={false} rows={1} />
        <SettingsCardSkeleton description={false} rows={1} />
      </div>
    </div>
  )
}

function NodeProvidersSettingsPage() {
  const { t } = useTranslation('settings')

  return (
    <PageTemplate
      icon={<CloudIcon />}
      title={t('nodeProvidersPage.title')}
      description={t('nodeProvidersPage.description')}
    >
      <NodeProviderCredentialsCard />
    </PageTemplate>
  )
}
