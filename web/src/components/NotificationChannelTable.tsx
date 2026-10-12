import type { ReactNode } from 'react'
import {
  PaperPlaneTiltIcon,
  WebhooksLogoIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import { BrandIcon } from './BrandIcon'
import { DeleteNotificationChannelDialog } from './DeleteNotificationChannelDialog'
import { EditNotificationChannelDialog } from './EditNotificationChannelDialog'
import { NotificationDeliveryHistoryDialog } from './NotificationDeliveryHistoryDialog'
import {
  CHANNEL_KIND_BRAND_ICON,
  CHANNEL_KIND_LABEL,
} from './notificationChannelKind'
import { useTestExistingNotificationChannel } from '../queries/notificationChannels'
import type { NotificationChannel } from '../types/notificationChannel'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

// Same "mask the destination, never print a webhook URL in full" choice
// DeployNotifyTargetsPanel's own maskNotifyUrl already makes.
function maskNotifyUrl(url: string): string {
  try {
    const parsed = new URL(url)
    return `${parsed.host}/••••`
  } catch {
    return url
  }
}

function TestButton({ channel }: { channel: NotificationChannel }) {
  const testChannel = useTestExistingNotificationChannel()
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={testChannel.isPending}
      onClick={() => {
        testChannel.mutate(channel.id, {
          onSuccess: () => {
            toast.add({ title: 'Test notification sent.', type: 'success' })
          },
          onError: (error) => {
            toast.add({
              title: 'Test notification failed.',
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <PaperPlaneTiltIcon className="size-3.5" aria-hidden="true" />
      {testChannel.isPending ? 'Sending...' : 'Test'}
    </Button>
  )
}

export function NotificationChannelTable({
  channels,
  action,
}: {
  channels: NotificationChannel[]
  action?: ReactNode
}) {
  const { t } = useTranslation('settings')
  if (channels.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<WebhooksLogoIcon className="size-5" />}
        title="No notification channels connected"
        description="Connect Slack, Discord, Telegram, a generic webhook, or email once here, then attach it from any app instead of retyping a URL."
        action={action}
      />
    )
  }

  const columns: ResponsiveColumn<NotificationChannel>[] = [
    {
      id: 'name',
      primary: true,
      header: 'Name',
      cell: (channel) => (
        <span className="font-medium text-foreground">{channel.name}</span>
      ),
    },
    {
      id: 'type',
      header: 'Type',
      priority: 2,
      cell: (channel) => {
        const brandIcon = CHANNEL_KIND_BRAND_ICON[channel.kind]
        return (
          <Badge variant="outline" className="gap-1.5">
            {brandIcon ? (
              <BrandIcon name={brandIcon} className="size-3.5" />
            ) : null}
            {CHANNEL_KIND_LABEL[channel.kind]}
          </Badge>
        )
      },
    },
    {
      id: 'destination',
      header: 'Destination',
      priority: 3,
      cell: (channel) => (
        <span
          className="block truncate font-mono text-xs text-muted-foreground"
          title={channel.notify_url}
        >
          {maskNotifyUrl(channel.notify_url)}
        </span>
      ),
    },
    {
      id: 'status',
      header: 'Status',
      cell: (channel) =>
        channel.enabled ? (
          <Badge variant="success">Enabled</Badge>
        ) : (
          <Badge variant="muted">Disabled</Badge>
        ),
    },
    {
      id: 'connected',
      header: 'Connected',
      priority: 3,
      cell: (channel) => (
        <span className="text-muted-foreground">
          {formatDate(channel.created_at)}
        </span>
      ),
    },
  ]

  return (
    <ResponsiveTable
      rows={channels}
      columns={columns}
      getRowId={(channel) => channel.id}
      ariaLabel={t('tables.notificationChannels')}
      renderActions={(channel) => (
        <div className="flex flex-wrap items-center justify-end gap-2">
          <TestButton channel={channel} />
          <NotificationDeliveryHistoryDialog channel={channel} />
          <EditNotificationChannelDialog channel={channel} />
          <DeleteNotificationChannelDialog channel={channel} />
        </div>
      )}
    />
  )
}
