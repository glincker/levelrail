import { useTranslation } from 'react-i18next'
import { RotateCwGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useEventWebhookDeliveries,
  useReplayEventDelivery,
} from '../../queries/eventWebhooks'
import type { EventDeliveryState } from '../../types/eventWebhooks'

const STATE_VARIANT = {
  pending: 'warning',
  delivered: 'success',
  dead: 'destructive',
} as const satisfies Record<EventDeliveryState, string>

const STATE_KEYS = {
  pending: 'deliveries.state.pending',
  delivered: 'deliveries.state.delivered',
  dead: 'deliveries.state.dead',
} as const satisfies Record<EventDeliveryState, string>

export function EventWebhookDeliveries({ webhookId }: { webhookId: string }) {
  const { t } = useTranslation('eventWebhooks')
  const { data, isPending, error } = useEventWebhookDeliveries(webhookId, true)
  const replay = useReplayEventDelivery()

  if (isPending) {
    return (
      <p className="text-sm text-muted-foreground">{t('deliveries.loading')}</p>
    )
  }
  if (error) {
    return (
      <p className="text-sm text-tone-danger">
        {t('deliveries.loadError', { message: error.message })}
      </p>
    )
  }
  if (data.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('deliveries.empty')}</p>
    )
  }

  return (
    <ul className="divide-y divide-border rounded-md border border-border">
      {data.map((d) => (
        <li
          key={d.id}
          className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
        >
          <div className="min-w-0 space-y-0.5">
            <p className="flex flex-wrap items-center gap-2 text-sm text-foreground">
              <span className="font-mono">{d.event_type}</span>
              <Badge variant={STATE_VARIANT[d.state]}>
                {t(STATE_KEYS[d.state])}
              </Badge>
            </p>
            <p className="break-words text-xs text-muted-foreground">
              {t('deliveries.summary', {
                attempts: d.attempts,
                status: d.response_status || t('deliveries.noStatus'),
                when: d.updated_at,
              })}
              {d.last_error ? ` · ${d.last_error}` : ''}
            </p>
          </div>
          {d.state !== 'pending' ? (
            <Button
              variant="outline"
              size="sm"
              disabled={replay.isPending}
              onClick={() => {
                replay.mutate(d.id, {
                  onSuccess: () => {
                    toast.add({
                      title: t('deliveries.replayed'),
                      type: 'success',
                    })
                  },
                  onError: (err) => {
                    toast.add({
                      title: t('deliveries.replayFailed', {
                        message: err.message,
                      }),
                      type: 'error',
                    })
                  },
                })
              }}
            >
              <RotateCwGlassIcon className="size-5" aria-hidden="true" />
              {t('deliveries.replay')}
            </Button>
          ) : null}
        </li>
      ))}
    </ul>
  )
}
