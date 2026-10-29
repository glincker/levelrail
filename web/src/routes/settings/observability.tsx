import { createFileRoute } from '@tanstack/react-router'
import {
  observabilitySettingsQueryOptions,
  useObservabilitySettings,
} from '../../queries/observabilitySettings'
import {
  ObservabilitySettingsCard,
  RemoteReadConnectionCard,
} from '../../components/ObservabilitySettingsCard'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'
import { useTranslation } from 'react-i18next'
import { ChartLineGlassIcon } from '@thesvg/ui-react'
import { LogVolumeCard } from '../../components/logBudgets/LogVolumeCard'
import { PageTemplate } from '@/components/shell/PageTemplate'

export const Route = createFileRoute('/settings/observability')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(observabilitySettingsQueryOptions()),
  component: ObservabilitySettingsPage,
  pendingComponent: ObservabilitySettingsSkeleton,
})

function ObservabilitySettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={3} />
      <SettingsCardSkeleton rows={2} />
    </div>
  )
}

function ObservabilitySettingsPage() {
  const { t } = useTranslation('settings')
  const { data: settings } = useObservabilitySettings()

  return (
    <PageTemplate
      icon={<ChartLineGlassIcon size="1.4em" />}
      title={t('observabilityPage.title')}
      description={t('observabilityPage.description')}
    >
      <RemoteReadConnectionCard settings={settings} />
      <ObservabilitySettingsCard settings={settings} />
      <LogVolumeCard />
    </PageTemplate>
  )
}
