import { createFileRoute } from '@tanstack/react-router'
import { CloudIcon } from '@phosphor-icons/react/dist/ssr'
import { PageHeader } from '../../components/shell/PageHeader'
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
  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <CloudIcon className="size-4" />
        </div>
        <PageHeader
          title="Cloud node providers"
          description="API tokens for creating servers automatically from the Nodes page. Used only to create and inspect VMs, day to day operation never touches SSH."
        />
      </div>
      <NodeProviderCredentialsCard />
    </div>
  )
}
