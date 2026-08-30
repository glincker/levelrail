import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { WebhooksLogoIcon } from '@phosphor-icons/react/dist/ssr'
import { notificationChannelListQueryOptions } from '../../queries/notificationChannels'
import { NotificationChannelTable } from '../../components/NotificationChannelTable'
import { CreateNotificationChannelDialog } from '../../components/CreateNotificationChannelDialog'
import { TableSkeleton } from '@/components/ui/table-skeleton'

// Account-level, mirroring routes/settings/backup-targets.tsx: connect
// once here, apps attach a channel by ID instead of retyping a URL.
export const Route = createFileRoute('/settings/notification-channels')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(notificationChannelListQueryOptions()),
  component: NotificationChannelsPage,
  pendingComponent: NotificationChannelsPending,
})

function NotificationChannelsPage() {
  const { data: channels } = useSuspenseQuery(
    notificationChannelListQueryOptions(),
  )

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div className="flex items-start gap-3">
          <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
            <WebhooksLogoIcon className="size-4" />
          </div>
          <div>
            <h1 className="text-lg font-semibold text-foreground">
              Notification channels
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Connect Slack, Discord, Telegram, a generic webhook, or email
              once, then attach it from any app&apos;s deploy notifications.
            </p>
          </div>
        </div>
        <CreateNotificationChannelDialog />
      </div>
      <NotificationChannelTable channels={channels} />
    </div>
  )
}

// Route-level fallback for the loader's pending phase, matching
// NotificationChannelTable's own 6-column shape so the skeleton doesn't
// jump when real rows swap in.
function NotificationChannelsPending() {
  return (
    <div className="space-y-6">
      <h1 className="text-lg font-semibold text-foreground">
        Notification channels
      </h1>
      <TableSkeleton columnCount={6} />
    </div>
  )
}
