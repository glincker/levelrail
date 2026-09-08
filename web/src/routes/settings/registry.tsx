import { createFileRoute } from '@tanstack/react-router'
import {
  registrySettingsQueryOptions,
  useRegistrySettings,
} from '../../queries/registry'
import { RegistrySettingsCard } from '../../components/RegistrySettingsCard'
import { PageSpinner } from '../../components/ui/page-spinner'

// Instance-level, not scoped to one app: lives under routes/settings/
// next to cloudflare-tunnel.tsx, the same reasoning that file's own
// header comment gives for its placement. Covers only what internal/api's
// registry routes actually do: enabling/disabling the built-in registry
// container and viewing its status (see the backend package doc comment,
// internal/reconcile/registry). Distinct from
// routes/settings/registry-credentials.tsx, which manages credentials to
// an external registry an operator already runs elsewhere.
export const Route = createFileRoute('/settings/registry')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(registrySettingsQueryOptions()),
  component: RegistrySettingsPage,
  pendingComponent: PageSpinner,
})

function RegistrySettingsPage() {
  const { data: settings } = useRegistrySettings()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">
          Container registry
        </h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Levelrail&apos;s own built-in image registry for multi-node build
          caching and image distribution.
        </p>
      </div>

      <RegistrySettingsCard settings={settings} />
    </div>
  )
}
