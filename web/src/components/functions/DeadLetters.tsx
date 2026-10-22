import { useTranslation } from 'react-i18next'
import { ArrowClockwiseIcon, TrashIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useDeadLetters,
  useDiscardInvocation,
  useRetryInvocation,
} from '../../queries/functionTriggers'

export function DeadLetters() {
  const { t } = useTranslation('functions')
  const { data, error } = useDeadLetters()
  const retry = useRetryInvocation()
  const discard = useDiscardInvocation()

  if (error) {
    return (
      <p className="text-sm text-destructive">
        {t('triggers.dead.error', { message: error.message })}
      </p>
    )
  }

  const onError = (err: Error) => {
    toast.add({ title: err.message, type: 'error' })
  }

  return (
    <section className="space-y-3" aria-labelledby="dead-letters-title">
      <div>
        <h2 id="dead-letters-title" className="text-base font-semibold">
          {t('triggers.dead.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('triggers.dead.description')}
        </p>
      </div>
      {!data || data.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('triggers.dead.empty')}
        </p>
      ) : (
        <ul className="divide-y rounded-lg border">
          {data.map((inv) => (
            <li
              key={inv.id}
              className="flex flex-wrap items-center gap-3 p-3 text-sm"
            >
              <div className="min-w-0 flex-1">
                <p className="font-medium">
                  {inv.function}{' '}
                  <span className="font-mono text-xs text-muted-foreground">
                    {inv.method} {inv.path}
                  </span>
                </p>
                <p className="truncate text-xs text-muted-foreground">
                  {t('triggers.dead.detail', {
                    attempts: inv.attempts,
                    max: inv.max_attempts,
                    code: inv.last_status_code ?? 0,
                    error: inv.last_error ?? '',
                  })}
                </p>
              </div>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={retry.isPending}
                onClick={() => {
                  retry.mutate(inv.id, { onError })
                }}
              >
                <ArrowClockwiseIcon />
                {t('triggers.dead.retry')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={discard.isPending}
                onClick={() => {
                  discard.mutate(inv.id, { onError })
                }}
              >
                <TrashIcon />
                {t('triggers.dead.discard')}
              </Button>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
