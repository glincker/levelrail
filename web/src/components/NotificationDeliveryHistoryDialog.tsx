import { useState } from 'react'
import { ClockCounterClockwiseIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { useNotificationDeliveries } from '../queries/notificationChannels'
import type { NotificationChannel } from '../types/notificationChannel'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

const TRIGGER_LABEL: Record<string, string> = {
  test: 'Test',
  'deploy-succeeded': 'Deploy succeeded',
  'deploy-failed': 'Deploy failed',
  'alert-fired': 'Alert fired',
  'alert-resolved': 'Alert resolved',
}

function triggerLabel(trigger: string): string {
  return TRIGGER_LABEL[trigger] ?? trigger
}

const MAX_ERROR_LENGTH = 140

function truncateError(error: string): string {
  if (error.length <= MAX_ERROR_LENGTH) {
    return error
  }
  return error.slice(0, MAX_ERROR_LENGTH) + '...'
}

// Per-channel send history: deploy outcomes, alert rules, and test-send
// clicks alike, all funneled into the same delivery record on the
// backend (internal/alerting's recordDelivery), so this is the one place
// an operator answers "did this channel actually work last time."
export function NotificationDeliveryHistoryDialog({
  channel,
}: {
  channel: NotificationChannel
}) {
  const [open, setOpen] = useState(false)
  const { data, isLoading, isError, error } = useNotificationDeliveries(
    channel.id,
    open,
  )

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button type="button" variant="outline" size="sm" />}>
        <ClockCounterClockwiseIcon className="size-3.5" aria-hidden="true" />
        History
      </DialogTrigger>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>&ldquo;{channel.name}&rdquo; delivery history</DialogTitle>
          <DialogDescription>
            Recent send attempts through this channel, newest first.
          </DialogDescription>
        </DialogHeader>
        {isLoading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : isError ? (
          <p className="text-sm text-destructive">{error.message}</p>
        ) : data && data.length > 0 ? (
          <ul className="max-h-80 space-y-2 overflow-y-auto">
            {data.map((delivery) => (
              <li
                key={delivery.id}
                className="rounded-md border border-border p-2.5 text-sm"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-muted-foreground">
                    {formatDate(delivery.created_at)}
                  </span>
                  <Badge variant="outline">
                    {triggerLabel(delivery.trigger)}
                  </Badge>
                  {delivery.success ? (
                    <Badge variant="success">Delivered</Badge>
                  ) : (
                    <Badge variant="destructive">Failed</Badge>
                  )}
                </div>
                {!delivery.success && delivery.error ? (
                  <p
                    className="mt-1.5 truncate text-xs text-destructive"
                    title={delivery.error}
                  >
                    {truncateError(delivery.error)}
                  </p>
                ) : null}
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            No recorded deliveries yet. Use &ldquo;Test&rdquo; to send one.
          </p>
        )}
      </DialogContent>
    </Dialog>
  )
}
