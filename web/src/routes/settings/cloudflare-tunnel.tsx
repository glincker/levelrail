import { requireExperimental } from '../../lib/experimental'
import { createFileRoute } from '@tanstack/react-router'
import {
  cloudflareTunnelSettingsQueryOptions,
  useCloudflareTunnelSettings,
} from '../../queries/cloudflareTunnel'
import { CloudflareTunnelCard } from '../../components/CloudflareTunnelCard'
import { PageHeader } from '../../components/shell/PageHeader'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'

// Instance-level, not scoped to one app: lives under routes/settings/
// next to email.tsx and github-app.tsx, the same reasoning those files'
// own header comments give for their placement. Covers only what
// internal/api's cloudflare-tunnel routes actually do: enabling/
// disabling the cloudflared container and viewing its connection
// status. Per-app hostname routing stays entirely on Cloudflare's own
// dashboard, out of scope here (see the backend package doc comment,
// internal/reconcile/cloudflaretunnel).
export const Route = createFileRoute('/settings/cloudflare-tunnel')({
  beforeLoad: ({ context: { queryClient } }) =>
    requireExperimental(queryClient, 'cloudflare-tunnel'),
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(cloudflareTunnelSettingsQueryOptions()),
  component: CloudflareTunnelSettingsPage,
  pendingComponent: CloudflareTunnelSettingsSkeleton,
})

function CloudflareTunnelSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={1} />
    </div>
  )
}

function CloudflareTunnelSettingsPage() {
  const { data: settings } = useCloudflareTunnelSettings()

  return (
    <div className="space-y-6">
      <PageHeader
        title="Cloudflare Tunnel"
        description="Expose this control plane to the internet without opening an inbound port."
      />

      <CloudflareTunnelCard settings={settings} />
    </div>
  )
}
