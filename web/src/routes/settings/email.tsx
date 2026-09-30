import { createFileRoute } from '@tanstack/react-router'
import {
  emailSettingsQueryOptions,
  useEmailSettings,
} from '../../queries/emailSettings'
import { EmailSettingsCard } from '../../components/EmailSettingsCard'
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
      <div>
        <h1 className="text-lg font-semibold text-foreground">Email</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          The backend that sends alert notifications and password-reset links.
        </p>
      </div>

      <EmailSettingsCard settings={settings} />
    </div>
  )
}
