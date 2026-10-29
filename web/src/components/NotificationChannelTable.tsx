import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { SendGlassIcon, WebhookGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import { BrandTile } from './BrandLogoBadge'
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
  const { t } = useTranslation('settings')
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
            toast.add({
              title: t('channelsPage.table.testOk'),
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: t('channelsPage.table.testFailed'),
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <SendGlassIcon className="size-4" aria-hidden="true" />
      {testChannel.isPending
        ? t('channelsPage.table.sending')
        : t('channelsPage.table.test')}
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
        className="gap-2 border-0 bg-transparent py-4"
        icon={<WebhookGlassIcon className="size-5.5" />}
        title={t('channelsPage.empty.title')}
        description={t('channelsPage.empty.body')}
        action={action}
      />
    )
  }

  const columns: ResponsiveColumn<NotificationChannel>[] = [
    {
      id: 'name',
      primary: true,
      header: t('channelsPage.table.name'),
      cell: (channel) => (
        <span className="font-medium text-foreground">{channel.name}</span>
      ),
    },
    {
      id: 'type',
      header: t('channelsPage.table.type'),
      priority: 2,
      cell: (channel) => {
        const brandIcon = CHANNEL_KIND_BRAND_ICON[channel.kind]
        return (
          <Badge variant="outline" className="gap-1.5">
            <BrandTile
              name={brandIcon ?? ''}
              label={CHANNEL_KIND_LABEL[channel.kind]}
              size="xs"
            />
            {CHANNEL_KIND_LABEL[channel.kind]}
          </Badge>
        )
      },
    },
    {
      id: 'destination',
      header: t('channelsPage.table.destination'),
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
      header: t('channelsPage.table.status'),
      cell: (channel) =>
        channel.enabled ? (
          <Badge variant="success">{t('channelsPage.table.enabled')}</Badge>
        ) : (
          <Badge variant="muted">{t('channelsPage.table.disabled')}</Badge>
        ),
    },
    {
      id: 'connected',
      header: t('channelsPage.table.connected'),
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
      ariaLabel={t('channelsPage.table.label')}
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
