import { createFileRoute } from '@tanstack/react-router'
import {
  emailSettingsQueryOptions,
  useEmailSettings,
} from '../../queries/emailSettings'
import { EmailSettingsCard } from '../../components/EmailSettingsCard'
import { EmailPageShell } from '../../components/settings/EmailPageShell'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'

export const Route = createFileRoute('/settings/email')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(emailSettingsQueryOptions()),
  component: EmailSettingsPage,
  pendingComponent: EmailSettingsSkeleton,
})

function EmailSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={5} />
    </div>
  )
}

function EmailSettingsPage() {
  const { data: settings } = useEmailSettings()

  return (
    <EmailPageShell backend={settings.backend}>
      <EmailSettingsCard settings={settings} />
    </EmailPageShell>
  )
}
