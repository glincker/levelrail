import { createFileRoute } from '@tanstack/react-router'
import {
  emailSettingsQueryOptions,
  useEmailSettings,
} from '../../queries/emailSettings'
import { EmailSettingsCard } from '../../components/EmailSettingsCard'
import { EmailDomainCheckCard } from '../../components/email/EmailDomainCheckCard'
import { EmailStatusHeader } from '../../components/email/EmailStatusHeader'
import { EmailTestCard } from '../../components/email/EmailTestCard'
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
    <EmailPageShell settings={settings}>
      <div className="space-y-6">
        <EmailStatusHeader />
        <EmailSettingsCard settings={settings} />
        <EmailTestCard provider={settings.backend} />
        <EmailDomainCheckCard provider={settings.backend} />
      </div>
    </EmailPageShell>
  )
}
