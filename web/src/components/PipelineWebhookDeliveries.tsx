import { useTranslation } from 'react-i18next'
import { RotateCcwGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  usePipelineWebhookDeliveries,
  useReplayWebhookDelivery,
} from '../queries/pipelineExtras'
import type { PipelineWebhookOutcome } from '../types/pipelines'

const OUTCOME_VARIANT: Record<
  PipelineWebhookOutcome,
  'success' | 'destructive' | 'muted'
> = {
  received: 'muted',
  started: 'success',
  rejected: 'destructive',
  failed: 'destructive',
}

export function PipelineWebhookDeliveries({
  app,
  name,
}: {
  app: string
  name: string
}) {
  const { t } = useTranslation('pipelines')
  const { data } = usePipelineWebhookDeliveries(app, name)
  const replay = useReplayWebhookDelivery(app, name)
  const list = data ?? []
  return (
    <section aria-labelledby="webhook-deliveries" className="space-y-2">
      <h3
        id="webhook-deliveries"
        className="text-sm font-semibold text-foreground"
      >
        {t('webhook.deliveries')}
      </h3>
      {list.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('webhook.noDeliveries')}
        </p>
      ) : (
        <ul className="max-h-60 divide-y divide-border overflow-y-auto rounded-lg border border-border">
          {list.map((d) => (
            <li
              key={d.id}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 p-2 text-xs"
            >
              <time
                dateTime={d.received_at}
                className="tabular-nums text-muted-foreground"
              >
                {new Date(d.received_at).toLocaleString()}
              </time>
              <Badge variant={OUTCOME_VARIANT[d.outcome]}>
                {t(`webhook.outcomes.${d.outcome}`)}
              </Badge>
              <span className="tabular-nums text-muted-foreground">
                {d.status_code}
              </span>
              {d.reason ? (
                <span className="min-w-24 flex-1 break-words text-muted-foreground">
                  {d.reason}
                </span>
              ) : (
                <span className="flex-1" />
              )}
              {d.replayable ? (
                <Button
                  size="xs"
                  variant="outline"
                  disabled={replay.isPending}
                  aria-label={t('webhook.replayLabel', {
                    id: d.id.slice(0, 8),
                  })}
                  onClick={() =>
                    replay.mutate(d.id, {
                      onSuccess: (run) =>
                        toast.add({
                          title: t('webhook.replayed', { number: run.number }),
                          type: 'success',
                        }),
                      onError: (e) =>
                        toast.add({ title: e.message, type: 'error' }),
                    })
                  }
                >
                  <RotateCcwGlassIcon size="1.4em" aria-hidden="true" />
                  {t('webhook.replay')}
                </Button>
              ) : null}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
