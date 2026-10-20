import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { toast } from '@/components/ui/toast'
import {
  useRollbackTakeover,
  type TakeoverRun,
  type TakeoverState,
} from '../queries/portTakeover'

const stateVariant: Record<
  TakeoverState,
  'success' | 'warning' | 'destructive' | 'default'
> = {
  preparing: 'warning',
  applying: 'warning',
  verifying: 'warning',
  done: 'success',
  rolling_back: 'warning',
  rolled_back: 'default',
  failed: 'destructive',
}

const stepVariant = {
  pending: 'default',
  running: 'warning',
  done: 'success',
  failed: 'destructive',
  skipped: 'default',
} as const

export function TakeoverRunPanel({ run }: { run: TakeoverRun }) {
  const { t } = useTranslation('migration')
  const rollback = useRollbackTakeover()
  const [asking, setAsking] = useState(false)
  const failing = run.verify.filter((d) => !d.ok && d.domain)
  return (
    <section
      className="space-y-2 rounded-md border p-3"
      aria-live="polite"
      aria-label={t('takeover.runTitle')}
    >
      <div className="flex flex-wrap items-center gap-2">
        <h4 className="text-sm font-semibold">{t('takeover.runTitle')}</h4>
        <Badge variant={stateVariant[run.state]}>
          {t(`takeover.state.${run.state}`)}
        </Badge>
        {run.rollbackable ? (
          <Button
            className="ml-auto"
            size="sm"
            variant="outline"
            disabled={rollback.isPending}
            onClick={() => setAsking(true)}
          >
            {t('takeover.rollback.button')}
          </Button>
        ) : null}
      </div>
      {run.error ? (
        <Alert variant={run.state === 'failed' ? 'destructive' : 'default'}>
          <AlertDescription>{run.error}</AlertDescription>
        </Alert>
      ) : null}
      <ol className="space-y-1 text-sm">
        {run.steps.map((s) => (
          <li key={s.id} className="flex gap-2">
            <Badge variant={stepVariant[s.state]}>
              {t(`takeover.step.${s.id}`, { defaultValue: s.title })}
            </Badge>
            <span className="flex-1 text-muted-foreground">{s.detail}</span>
          </li>
        ))}
      </ol>
      {failing.length > 0 ? (
        <ul className="list-disc space-y-0.5 pl-5 text-xs text-muted-foreground">
          {failing.map((d) => (
            <li key={d.domain}>
              {d.domain}: {d.detail}
            </li>
          ))}
        </ul>
      ) : null}
      <p className="text-xs text-muted-foreground">
        {t(`takeover.hint.${run.state}`, {
          defaultValue: '',
          seconds: run.window_seconds,
        })}
      </p>
      <ConfirmDialog
        open={asking}
        onOpenChange={setAsking}
        title={t('takeover.rollback.title')}
        description={t('takeover.rollback.description')}
        confirmLabel={t('takeover.rollback.confirm')}
        pendingLabel={t('takeover.rollback.pending')}
        pending={rollback.isPending}
        error={rollback.isError ? rollback.error.message : null}
        onConfirm={() =>
          rollback.mutate(
            { run: run.id },
            {
              onSuccess: () => setAsking(false),
              onError: (e) => toast.add({ title: e.message, type: 'error' }),
            },
          )
        }
      />
    </section>
  )
}
