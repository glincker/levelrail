import { createFileRoute } from '@tanstack/react-router'
import { VaultIcon } from '@phosphor-icons/react/dist/ssr'
import {
  vaultSettingsQueryOptions,
  useVaultSettings,
} from '../../queries/vault'
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
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <VaultIcon className="size-4" />
        </div>
        <div>
          <h1 className="text-lg font-semibold text-foreground">Vault</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Resolve app secrets live from an external HashiCorp Vault instance.
          </p>
        </div>
      </div>

      <VaultSettingsCard settings={settings} />
    </div>
  )
}
