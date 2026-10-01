import { createFileRoute } from '@tanstack/react-router'
import {
  emailSettingsQueryOptions,
  useEmailSettings,
} from '../../queries/emailSettings'
import { EmailSettingsCard } from '../../components/EmailSettingsCard'
import { PageHeader } from '../../components/shell/PageHeader'
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
    <div className="space-y-6">
      <PageHeader
        title="Email"
        description="The backend that sends alert notifications and password-reset links."
      />

      <EmailSettingsCard settings={settings} />
    </div>
  )
}
