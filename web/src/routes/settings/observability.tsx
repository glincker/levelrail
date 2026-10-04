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
import { PageHeader } from '@/components/shell/PageHeader'

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
  const { data: settings } = useObservabilitySettings()

  return (
    <div className="space-y-6">
      <PageHeader
        title="Observability"
        description="Connect your own Grafana to this control plane's metrics."
      />

      <RemoteReadConnectionCard settings={settings} />
      <ObservabilitySettingsCard settings={settings} />
    </div>
  )
}
