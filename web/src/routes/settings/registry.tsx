import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { HardDrivesIcon } from '@phosphor-icons/react/dist/ssr'
import {
  registrySettingsQueryOptions,
  useRegistrySettings,
} from '../../queries/registry'
import {
  RegistrySettingsCard,
  RegistryStatusBadge,
} from '../../components/RegistrySettingsCard'
import { PageTemplate } from '@/components/shell/PageTemplate'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'

// Instance-level: the built-in registry container's enable/disable and
// status. Distinct from registry-credentials.tsx, which stores credentials
// for an external registry.
export const Route = createFileRoute('/settings/registry')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(registrySettingsQueryOptions()),
  component: RegistrySettingsPage,
  pendingComponent: RegistrySettingsSkeleton,
})

function RegistrySettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={1} />
    </div>
  )
}

function RegistrySettingsPage() {
  const { t } = useTranslation('settings')
  const { data: settings } = useRegistrySettings()

  return (
    <PageTemplate
      icon={<HardDrivesIcon />}
      title={t('builtInRegistryPage.title')}
      description={t('builtInRegistryPage.description')}
      status={<RegistryStatusBadge status={settings.status} />}
    >
      <RegistrySettingsCard settings={settings} />
    </PageTemplate>
  )
}
