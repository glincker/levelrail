import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { WebhookGlassIcon } from '@thesvg/ui-react'
import { Banner } from '@/components/ui/banner'
import { buttonVariants } from '@/components/ui/button'
import {
  emailProvidersQueryOptions,
  useEmailProviders,
} from '../../queries/emailProviders'
import {
  emailSettingsQueryOptions,
  useEmailSettings,
} from '../../queries/emailSettings'
import { EmailBrandingCard } from '../../components/email/EmailBrandingCard'
import { EmailDeliveryCard } from '../../components/email/EmailDeliveryCard'
import { EmailProvidersCard } from '../../components/email/EmailProvidersCard'
import { EmailSenderCard } from '../../components/email/EmailSenderCard'
import { EmailStatusHeader } from '../../components/email/EmailStatusHeader'
import { EmailPageShell } from '../../components/settings/EmailPageShell'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'

export const Route = createFileRoute('/settings/email')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(emailSettingsQueryOptions()),
      queryClient.ensureQueryData(emailProvidersQueryOptions()),
    ]),
  component: EmailSettingsPage,
  pendingComponent: EmailSettingsSkeleton,
})

function EmailSettingsSkeleton() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={3} />
    </div>
  )
}

function EmailSettingsPage() {
  const { t } = useTranslation('settings')
  const { data: settings } = useEmailSettings()
  const { data: providers } = useEmailProviders()
  const hasProviders = (providers ?? []).length > 0

  return (
    <EmailPageShell settings={settings}>
      <div className="space-y-4">
        <EmailStatusHeader />
        <EmailProvidersCard
          sharedFrom={settings.from_address ?? ''}
          sesRegions={settings.ses_regions ?? []}
        />
        <div
          className={`grid items-start gap-4 ${hasProviders ? 'xl:grid-cols-2' : ''}`}
        >
          <EmailSenderCard settings={settings} />
          <EmailDeliveryCard />
        </div>
        <EmailBrandingCard />
        <Banner
          variant="info"
          icon={<WebhookGlassIcon size="1.4em" />}
          title={t('email.channelsCta')}
          action={
            <Link
              to="/settings/notification-channels"
              className={buttonVariants({ variant: 'outline', size: 'sm' })}
            >
              {t('email.channelsLink')}
            </Link>
          }
        />
      </div>
    </EmailPageShell>
  )
}
