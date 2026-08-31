import { ArrowClockwiseIcon, WebhooksLogoIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useReplayWebhookDelivery,
  useWebhookDeliveries,
} from '../queries/webhookDeliveries'
import type { WebhookDelivery } from '../types/webhookDelivery'
import type { AppDetail } from '../types/appDetail'

// Recent inbound webhook deliveries (internal/api/webhook_deliveries.go):
// what a git provider actually sent to this app's webhook URL, whether
// or not it verified or resolved to a real deploy, plus a manual replay
// of any stored delivery, the same "recent deliveries" feature GitHub's
// own webhook settings page offers for its own webhooks.

function DeliveryRow({ appName, delivery }: { appName: string; delivery: WebhookDelivery }) {
  const replay = useReplayWebhookDelivery(appName)

  function handleReplay() {
    replay.mutate(delivery.id, {
      onSuccess: (result) => {
        toast.add({
          title: 'Delivery replayed.',
          description: result.message.trim(),
          type: result.status < 400 ? 'success' : 'error',
        })
      },
      onError: (error) => {
        toast.add({ title: 'Could not replay delivery.', description: error.message, type: 'error' })
      },
    })
  }

  return (
    <li className="rounded-lg border border-border p-2.5">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="font-mono text-xs text-muted-foreground">{delivery.id}</span>
          <Badge variant="muted" className="rounded-full">{delivery.provider}</Badge>
          <span className="text-xs text-muted-foreground">{delivery.event_type}</span>
          <Badge variant={delivery.signature_valid ? 'success' : 'destructive'} className="rounded-full">
            {delivery.signature_valid ? 'verified' : 'unverified'}
          </Badge>
          {!delivery.matched ? (
            <Badge variant="warning" className="rounded-full">no git source</Badge>
          ) : null}
          <Badge variant={delivery.status_code < 400 ? 'success' : 'destructive'} className="rounded-full">
            {delivery.status_code}
          </Badge>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">
            {new Date(delivery.received_at).toLocaleString()}
          </span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={replay.isPending}
            onClick={handleReplay}
          >
            <ArrowClockwiseIcon aria-hidden="true" />
            {replay.isPending ? 'Replaying...' : 'Replay'}
          </Button>
        </div>
      </div>
      {delivery.error ? (
        <p className="mt-1.5 text-xs text-destructive">{delivery.error}</p>
      ) : null}
      <details className="mt-1.5 text-xs">
        <summary className="cursor-pointer text-muted-foreground underline underline-offset-2">
          Payload{delivery.payload_truncated ? ' (truncated)' : ''}
        </summary>
        <pre className="mt-1 max-h-60 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted p-2 font-mono text-[11px] text-foreground">
          {delivery.payload || '(empty)'}
        </pre>
      </details>
    </li>
  )
}

export function WebhookDeliveriesPanel({ app }: { app: AppDetail }) {
  const deliveries = useWebhookDeliveries(app.name)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <WebhooksLogoIcon className="size-4 text-muted-foreground" aria-hidden="true" />
          Recent deliveries
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="text-sm text-muted-foreground">
          Every inbound webhook request this app&apos;s webhook URL has received, verified or
          not, deployed or not. Replay re-runs a stored delivery&apos;s exact payload.
        </p>

        {deliveries.isLoading ? (
          <p className="text-sm text-muted-foreground">Loading deliveries...</p>
        ) : deliveries.error ? (
          <p className="text-sm text-destructive">{deliveries.error.message}</p>
        ) : deliveries.data && deliveries.data.length > 0 ? (
          <ul className="space-y-2">
            {deliveries.data.map((delivery) => (
              <DeliveryRow key={delivery.id} appName={app.name} delivery={delivery} />
            ))}
          </ul>
        ) : (
          <p className="text-sm text-muted-foreground">
            No webhook deliveries recorded yet. They&apos;ll show up here the first time the
            connected provider sends one.
          </p>
        )}
      </CardContent>
    </Card>
  )
}
