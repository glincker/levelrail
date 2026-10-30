import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PackageGlassIcon } from '@thesvg/ui-react'
import {
  registrySettingsQueryOptions,
  useRegistrySettings,
} from '../../queries/registry'
import {
  RegistrySettingsCard,
  RegistryStatusBadge,
} from '../../components/RegistrySettingsCard'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { PageBanner } from '../../components/PageBanner'
import { LazyRegistryDiagram } from '../../components/banners/LazyPass3Diagrams'
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

  const size = settings.enabled ? 'compact' : 'hero'

  return (
    <PageTemplate
      title={t('builtInRegistryPage.title')}
      banner={
        <PageBanner
          icon={<PackageGlassIcon size="1.4em" />}
          titleKey="settings:builtInRegistryPage.title"
          descriptionKey={
            settings.enabled
              ? 'settings:builtInRegistryPage.description'
              : 'settings:builtInRegistryPage.banner.description'
          }
          action={<RegistryStatusBadge status={settings.status} />}
          illustration={<LazyRegistryDiagram size={size} />}
          size={size}
        />
      }
    >
      <RegistrySettingsCard settings={settings} />
    </PageTemplate>
  )
}
