import { createFileRoute } from '@tanstack/react-router'
import {
  emailSettingsQueryOptions,
  useEmailSettings,
} from '../../queries/emailSettings'
import { EmailSettingsCard } from '../../components/EmailSettingsCard'
import { PageSpinner } from '../../components/ui/page-spinner'

export const Route = createFileRoute('/settings/email')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(emailSettingsQueryOptions()),
  component: EmailSettingsPage,
  pendingComponent: PageSpinner,
})

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
