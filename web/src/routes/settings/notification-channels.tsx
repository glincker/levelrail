import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { WebhookGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { notificationChannelListQueryOptions } from '../../queries/notificationChannels'
import { NotificationChannelTable } from '../../components/NotificationChannelTable'
import { CreateNotificationChannelDialog } from '../../components/CreateNotificationChannelDialog'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { SetupStepHint } from '@/components/getStarted/SetupStepHint'
import { HeartbeatCard } from '../../components/settings/HeartbeatCard'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'

// Account-level, mirroring routes/settings/backup-targets.tsx: connect
// once here, apps attach a channel by ID instead of retyping a URL.
export const Route = createFileRoute('/settings/notification-channels')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(notificationChannelListQueryOptions()),
  component: NotificationChannelsPage,
  pendingComponent: NotificationChannelsPending,
})

function NotificationChannelsPage() {
  const { t } = useTranslation('settings')
  const { data: channels } = useSuspenseQuery(
    notificationChannelListQueryOptions(),
  )
  const enabled = channels.filter((c) => c.enabled).length
  const disabled = channels.length - enabled

  let banner = null
  if (channels.length === 0) {
    banner = (
      <Banner variant="warning" title={t('channelsPage.banner.none')}>
        {t('channelsPage.banner.noneBody')}
      </Banner>
    )
  } else if (enabled === 0) {
    banner = (
      <Banner variant="warning" title={t('channelsPage.banner.allOff')}>
        {t('channelsPage.banner.allOffBody', { count: disabled })}
      </Banner>
    )
  }

  return (
    <PageTemplate
      icon={<WebhookGlassIcon size="1.4em" />}
      eyebrow={t('channelsPage.eyebrow')}
      title={t('channelsPage.title')}
      description={t('channelsPage.description')}
      helpPath="/email-notifications"
      helpLabel={t('channelsPage.help')}
      status={
        <Badge variant={enabled > 0 ? 'success' : 'muted'}>
          {t('channelsPage.enabledCount', { count: enabled })}
        </Badge>
      }
      actions={<CreateNotificationChannelDialog />}
      banners={banner}
    >
      <SetupStepHint step="notifications" />
      <NotificationChannelTable
        channels={channels}
        action={<CreateNotificationChannelDialog />}
      />
      <HeartbeatCard />
    </PageTemplate>
  )
}

// Route-level fallback for the loader's pending phase, matching
// NotificationChannelTable's own 5-column shape so the skeleton doesn't
// jump when real rows swap in.
function NotificationChannelsPending() {
  return (
    <div className="space-y-6">
      <SettingsPageTitle id="notification-channels" />
      <TableSkeleton columnCount={5} />
    </div>
  )
}
