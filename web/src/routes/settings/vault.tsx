import { createFileRoute } from '@tanstack/react-router'
import { vaultSettingsQueryOptions, useVaultSettings } from '../../queries/vault'
import { VaultSettingsCard } from '../../components/VaultSettingsCard'
import { PageSpinner } from '../../components/ui/page-spinner'

// Instance-level, not scoped to one app: lives under routes/settings/
// next to cloudflare-tunnel.tsx and registry.tsx, the same reasoning
// those files' own header comments give for their placement. Covers
// only what internal/api's vault routes actually do: configuring the
// external Vault connection and credential used to resolve app.yaml's
// own { vault: { path, key } } env vars. Declaring which env var on
// which app resolves from Vault happens on that app's own Environment
// tab (VaultEnvEditor), not here.
export const Route = createFileRoute('/settings/vault')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(vaultSettingsQueryOptions()),
  component: VaultSettingsPage,
  pendingComponent: PageSpinner,
})

function VaultSettingsPage() {
  const { data: settings } = useVaultSettings()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">Vault</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Resolve app secrets live from an external HashiCorp Vault instance.
        </p>
      </div>

      <VaultSettingsCard settings={settings} />
    </div>
  )
}
