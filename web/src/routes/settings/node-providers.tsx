import { createFileRoute } from '@tanstack/react-router'
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

function NodeProvidersSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={3} />
    </div>
  )
}

function NodeProvidersSettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">
          Cloud node providers
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Store an API token per provider so the Nodes page can create servers
          automatically.
        </p>
      </div>
      <NodeProviderCredentialsCard />
    </div>
  )
}
