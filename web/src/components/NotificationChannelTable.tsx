import {
  PaperPlaneTiltIcon,
  WebhooksLogoIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import { BrandIcon } from './BrandIcon'
import { DeleteNotificationChannelDialog } from './DeleteNotificationChannelDialog'
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
}: {
  channels: NotificationChannel[]
}) {
  if (channels.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<WebhooksLogoIcon className="size-5" />}
        title="No notification channels connected"
        description="Connect Slack, Discord, Telegram, a generic webhook, or email once here, then attach it from any app instead of retyping a URL."
      />
    )
  }

  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Type</TableHead>
            <TableHead>Destination</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Connected</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {channels.map((channel) => {
            const brandIcon = CHANNEL_KIND_BRAND_ICON[channel.kind]
            return (
              <TableRow key={channel.id}>
                <TableCell className="font-medium text-foreground">
                  {channel.name}
                </TableCell>
                <TableCell>
                  <Badge variant="outline" className="gap-1.5">
                    {brandIcon ? (
                      <BrandIcon name={brandIcon} className="size-3.5" />
                    ) : null}
                    {CHANNEL_KIND_LABEL[channel.kind]}
                  </Badge>
                </TableCell>
                <TableCell
                  className="max-w-[16rem] truncate font-mono text-xs text-muted-foreground"
                  title={channel.notify_url}
                >
                  {maskNotifyUrl(channel.notify_url)}
                </TableCell>
                <TableCell>
                  {channel.enabled ? (
                    <Badge variant="success">Enabled</Badge>
                  ) : (
                    <Badge variant="muted">Disabled</Badge>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(channel.created_at)}
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex justify-end gap-2">
                    <TestButton channel={channel} />
                    <DeleteNotificationChannelDialog channel={channel} />
                  </div>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
