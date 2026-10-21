import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  useAttemptAction,
  type AttemptAction,
  type SelfUpgradeAttempt,
} from '@/queries/selfUpgrade'

export function FailureActions({ attempt }: { attempt: SelfUpgradeAttempt }) {
  const { t } = useTranslation('updates')
  const act = useAttemptAction()
  const f = attempt.failure
  if (!f) return null
  const buttons: { action: AttemptAction; label: string; primary: boolean }[] =
    []
  if (f.primary === 'cleanup' && f.fix_id) {
    buttons.push({
      action: 'fix',
      label: t('selfUpgrade.failure.fix'),
      primary: true,
    })
  }
  if (f.retryable) {
    buttons.push({
      action: 'retry',
      label: t('selfUpgrade.failure.retry'),
      primary: f.primary === 'retry',
    })
  }
  if (f.rollbackable) {
    buttons.push({
      action: 'rollback',
      label: t('selfUpgrade.failure.rollback'),
      primary: f.primary === 'rollback',
    })
  }
  const outcome = act.data?.outcome
  return (
    <div className="space-y-2 rounded-md border border-border bg-muted/30 p-3">
      <p className="text-sm font-medium">{t('selfUpgrade.failure.title')}</p>
      <p className="text-sm text-muted-foreground">
        {t(`selfUpgrade.failure.cause.${f.cause}`, {
          defaultValue: t('selfUpgrade.failure.cause.unknown'),
        })}
      </p>
      {buttons.length > 0 ? (
        <div className="flex flex-wrap gap-2">
          {buttons.map((b) => (
            <Button
              key={b.action}
              type="button"
              size="sm"
              variant={b.primary ? 'default' : 'outline'}
              disabled={act.isPending}
              onClick={() => act.mutate({ id: attempt.id, action: b.action })}
            >
              {b.label}
            </Button>
          ))}
        </div>
      ) : null}
      {act.isError ? (
        <p role="alert" className="text-xs text-destructive">
          {act.error.message}
        </p>
      ) : null}
      {act.isSuccess ? (
        <p role="status" className="text-xs text-muted-foreground">
          {outcome === 'fixed'
            ? t('selfUpgrade.failure.fixed')
            : outcome
              ? t('selfUpgrade.failure.stillFailing')
              : t('selfUpgrade.failure.started')}
        </p>
      ) : null}
      {f.cli ? (
        <details className="text-xs text-muted-foreground">
          <summary className="cursor-pointer">
            {t('selfUpgrade.failure.cliSummary')}
          </summary>
          <p className="mt-1">{t('selfUpgrade.failure.cliHint')}</p>
          <code className="mt-1 block rounded bg-muted px-2 py-1 font-mono">
            {`levelrail-cli ${f.cli} ${attempt.id}`}
          </code>
        </details>
      ) : null}
    </div>
  )
}
