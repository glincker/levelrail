import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '../hooks/useIsRoot'
import {
  takeoverSettled,
  useStartTakeover,
  useTakeoverStatus,
  type TakeoverVerdict,
} from '../queries/portTakeover'
import { CheckRow, DomainList, PlanSteps } from './PortTakeoverChecks'
import { TakeoverRunPanel } from './PortTakeoverRun'

export const TAKEOVER_CONFIRM_WORD = 'takeover'

const verdictVariant: Record<
  TakeoverVerdict,
  'success' | 'warning' | 'destructive'
> = {
  ready: 'success',
  warnings: 'warning',
  blocked: 'destructive',
  done: 'success',
}

export function PortTakeoverPanel({
  onBack,
  onNext,
}: {
  onBack?: () => void
  onNext?: () => void
}) {
  const { t } = useTranslation('migration')
  const isRoot = useIsRoot()
  const [accept, setAccept] = useState(false)
  const [asking, setAsking] = useState(false)
  const status = useTakeoverStatus(accept)
  const start = useStartTakeover()
  const plan = status.data?.plan
  const run = status.data?.run ?? null
  const busy = start.isPending || !takeoverSettled(run)
  const unmanaged = plan?.checks.find((c) => c.id === 'unmanaged_domains')
  const canRun =
    isRoot &&
    !!plan &&
    plan.verdict !== 'blocked' &&
    plan.verdict !== 'done' &&
    !busy

  if (status.isPending) {
    return <Skeleton className="h-48 w-full rounded-lg" />
  }
  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">{t('takeover.title')}</h3>
        <p className="text-sm text-muted-foreground">
          {t('takeover.description')}
        </p>
      </div>
      {!isRoot ? (
        <Alert>
          <AlertDescription>{t('takeover.rootOnly')}</AlertDescription>
        </Alert>
      ) : null}
      {status.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('takeover.loadError')} {status.error.message}
          </AlertDescription>
        </Alert>
      ) : null}
      {run ? <TakeoverRunPanel run={run} /> : null}
      {plan ? (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={verdictVariant[plan.verdict]}>
              {t(`takeover.verdict.${plan.verdict}`)}
            </Badge>
            <span className="text-xs text-muted-foreground">
              {t('takeover.ports', {
                from: `${plan.current_http} ${plan.current_https}`,
                to: `${plan.target_http} ${plan.target_https}`,
              })}
            </span>
            <Button
              className="ml-auto"
              size="sm"
              variant="outline"
              disabled={status.isFetching}
              onClick={() => void status.refetch()}
            >
              {status.isFetching
                ? t('takeover.rechecking')
                : t('takeover.recheck')}
            </Button>
          </div>
          <ul className="space-y-2">
            {plan.checks.map((c) => (
              <CheckRow key={c.id} check={c} />
            ))}
          </ul>
          {unmanaged && unmanaged.status !== 'pass' ? (
            <label className="flex items-center gap-2 text-sm">
              <Checkbox
                checked={accept}
                onCheckedChange={(v) => setAccept(v === true)}
              />
              {t('takeover.acceptUnmanaged')}
            </label>
          ) : null}
          <DomainList domains={plan.domains} />
          {plan.verdict !== 'done' ? (
            <PlanSteps steps={plan.steps} rollback={plan.rollback} />
          ) : (
            <p className="text-sm text-muted-foreground">
              {t('takeover.alreadyDone')}
            </p>
          )}
        </>
      ) : null}
      <div className="flex flex-wrap items-center gap-2 border-t pt-3">
        {onBack ? (
          <Button variant="outline" onClick={onBack}>
            {t('appImport.back')}
          </Button>
        ) : null}
        <Button disabled={!canRun} onClick={() => setAsking(true)}>
          {t('takeover.run.button')}
        </Button>
        <span className="text-xs text-muted-foreground">
          {t('takeover.run.note', { seconds: plan?.window_seconds ?? 300 })}
        </span>
        {onNext ? (
          <Button variant="outline" className="ml-auto" onClick={onNext}>
            {t('appImport.retire.open')}
          </Button>
        ) : null}
      </div>
      <ConfirmDialog
        open={asking}
        onOpenChange={setAsking}
        title={t('takeover.run.dialogTitle')}
        description={t('takeover.run.dialogDescription')}
        consequences={[
          t('takeover.run.consequenceStop'),
          t('takeover.run.consequenceRestart'),
          t('takeover.run.consequenceRollback'),
        ]}
        confirmLabel={t('takeover.run.confirm')}
        pendingLabel={t('takeover.run.pending')}
        requireTyped={TAKEOVER_CONFIRM_WORD}
        tone="destructive"
        pending={start.isPending}
        error={start.isError ? start.error.message : null}
        onConfirm={() =>
          start.mutate(
            { confirm: true, acceptUnmanaged: accept },
            {
              onSuccess: () => setAsking(false),
              onError: (e) => toast.add({ title: e.message, type: 'error' }),
            },
          )
        }
      />
    </div>
  )
}
