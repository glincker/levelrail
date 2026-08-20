import { createFileRoute } from '@tanstack/react-router'
import {
  cloudflareTunnelSettingsQueryOptions,
  useCloudflareTunnelSettings,
} from '../../queries/cloudflareTunnel'
import { CloudflareTunnelCard } from '../../components/CloudflareTunnelCard'

// Instance-level, not scoped to one app: lives under routes/settings/
// next to email.tsx and github-app.tsx, the same reasoning those files'
// own header comments give for their placement. Covers only what
// internal/api's cloudflare-tunnel routes actually do: enabling/
// disabling the cloudflared container and viewing its connection
// status. Per-app hostname routing stays entirely on Cloudflare's own
// dashboard, out of scope here (see the backend package doc comment,
// internal/reconcile/cloudflaretunnel).
export const Route = createFileRoute('/settings/cloudflare-tunnel')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(cloudflareTunnelSettingsQueryOptions()),
  component: CloudflareTunnelSettingsPage,
})

function CloudflareTunnelSettingsPage() {
  const { data: settings } = useCloudflareTunnelSettings()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">
          Cloudflare Tunnel
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Expose this control plane to the internet without opening an
          inbound port.
        </p>
      </div>

      <CloudflareTunnelCard settings={settings} />
    </div>
  )
}
