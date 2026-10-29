import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { VaultGlassIcon } from '@thesvg/ui-react'
import {
  vaultSettingsQueryOptions,
  useVaultSettings,
} from '../../queries/vault'
import { VaultSettingsCard } from '../../components/VaultSettingsCard'
import { VaultRotateCredentialCard } from '../../components/VaultRotateCredentialCard'
import { PageTemplate } from '@/components/shell/PageTemplate'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'

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
  pendingComponent: VaultSettingsSkeleton,
})

function VaultSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton rows={3} />
    </div>
  )
}

function VaultSettingsPage() {
  const { t } = useTranslation('settings')
  const { data: settings } = useVaultSettings()

  return (
    <PageTemplate
      icon={<VaultGlassIcon size="1.4em" />}
      title={t('vaultPage.title')}
      description={t('vaultPage.description')}
    >
      <div className="space-y-6">
        <VaultSettingsCard settings={settings} />
        <VaultRotateCredentialCard settings={settings} />
      </div>
    </PageTemplate>
  )
}
