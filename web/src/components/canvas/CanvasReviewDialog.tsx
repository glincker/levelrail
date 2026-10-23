import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Skeleton } from '@/components/ui/skeleton'
import { IacPlanView, IacResultView } from '../IacPlanView'
import { IacIssuesError, type IacPlan } from '../../queries/iac'
import type { ApiError } from '../../lib/apiError'
import type { CanvasOutcome } from '../../hooks/useCanvasReview'
import type { ConnectionStep, ResolvedSteps } from '../../lib/canvasDraft'

export interface CanvasReviewDialogProps {
  open: boolean
  hasServerPlan: boolean
  plan: IacPlan | null
  planning: boolean
  planError: ApiError | null
  applyError: ApiError | null
  applying: boolean
  outcome: CanvasOutcome | null
  resolved: ResolvedSteps
  lookupsLoading: boolean
  onClose: () => void
  onRetry: () => void
  onApply: () => void
}

export function CanvasReviewDialog(props: CanvasReviewDialogProps) {
  const { t } = useTranslation('canvas')
  const {
    open,
    hasServerPlan,
    plan,
    planning,
    planError,
    applyError,
    applying,
    outcome,
    resolved,
    lookupsLoading,
    onClose,
    onRetry,
    onApply,
  } = props

  const serverPending = plan
    ? plan.summary.create + plan.summary.update + plan.summary.delete > 0
    : false
  const nothing =
    !planError && !planning && !serverPending && resolved.steps.length === 0
  const canApply =
    !planning &&
    !planError &&
    !applying &&
    !lookupsLoading &&
    (!hasServerPlan || (plan !== null && plan.summary.error === 0)) &&
    (serverPending || resolved.steps.length > 0)

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('review.title')}</DialogTitle>
          <DialogDescription>{t('review.description')}</DialogDescription>
        </DialogHeader>

        {planning ? (
          <div role="status" aria-live="polite" className="space-y-2">
            <p className="text-sm text-muted-foreground">
              {t('review.planning')}
            </p>
            <Skeleton className="h-16 w-full" />
          </div>
        ) : null}

        {planError ? (
          <Alert variant="destructive">
            <AlertDescription className="space-y-2">
              <p className="font-medium">{t('review.planError.title')}</p>
              {planError instanceof IacIssuesError ? (
                <>
                  <ul className="list-disc space-y-1 pl-5">
                    {planError.issues.map((i) => (
                      <li key={`${i.file ?? ''}:${i.line}:${i.message}`}>
                        {i.file ? `${i.file}:${i.line} ` : ''}
                        {i.message}
                      </li>
                    ))}
                  </ul>
                  <p>{t('review.planError.issuesNext')}</p>
                </>
              ) : (
                <>
                  <p>{planError.message}</p>
                  <p>{t('review.planError.generic')}</p>
                </>
              )}
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={onRetry}
              >
                {t('review.planError.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}

        {plan && !outcome ? (
          <section aria-labelledby="canvas-plan-heading" className="space-y-2">
            <h3 id="canvas-plan-heading" className="text-sm font-medium">
              {t('review.serverChanges')}
            </h3>
            <IacPlanView plan={plan} />
          </section>
        ) : null}

        {!outcome &&
        !planError &&
        (resolved.steps.length > 0 || lookupsLoading) ? (
          <section aria-labelledby="canvas-steps-heading" className="space-y-2">
            <h3 id="canvas-steps-heading" className="text-sm font-medium">
              {t('review.steps.title')}
            </h3>
            <p className="text-xs text-muted-foreground">
              {t('review.steps.note')}
            </p>
            {lookupsLoading ? (
              <p role="status" className="text-sm text-muted-foreground">
                {t('review.steps.loading')}
              </p>
            ) : (
              <ul className="space-y-1 text-sm">
                {resolved.steps.map((s) => (
                  <li key={`${s.kind}:${s.app}:${s.database}:${s.envVar}`}>
                    {stepText(t, s)}
                  </li>
                ))}
              </ul>
            )}
          </section>
        ) : null}

        {!outcome && resolved.unsupported.length > 0 ? (
          <Alert>
            <AlertDescription>
              <ul className="space-y-1">
                {resolved.unsupported.map((u) => (
                  <li key={`${u.app}:${u.database}`}>
                    {t('review.steps.unsupported', u)}
                  </li>
                ))}
              </ul>
            </AlertDescription>
          </Alert>
        ) : null}

        {nothing && !outcome ? (
          <p className="text-sm text-muted-foreground">{t('review.nothing')}</p>
        ) : null}

        {applyError && !outcome ? (
          <Alert variant="destructive">
            <AlertDescription>
              <p>{applyError.message}</p>
              <p>{t('review.planError.generic')}</p>
            </AlertDescription>
          </Alert>
        ) : null}

        {outcome ? (
          <section aria-live="polite" className="space-y-3">
            <h3 className="text-sm font-medium">
              {outcome.ok
                ? t('review.applied.title')
                : t('review.applied.failed')}
            </h3>
            {!outcome.ok ? (
              <p className="text-sm text-muted-foreground">
                {t('review.applied.failedNext')}
              </p>
            ) : null}
            {outcome.iac ? (
              <IacResultView results={outcome.iac.results} />
            ) : null}
            {outcome.steps.length > 0 ? (
              <ul className="space-y-1 text-sm">
                {outcome.steps.map((r) => (
                  <li key={`${r.step.kind}:${r.step.app}:${r.step.envVar}`}>
                    {stepText(t, r.step)}: {statusText(t, r.status, r.error)}
                  </li>
                ))}
              </ul>
            ) : null}
          </section>
        ) : null}

        <DialogFooter>
          {outcome?.ok ? (
            <Button type="button" onClick={onClose}>
              {t('review.applied.done')}
            </Button>
          ) : (
            <>
              <Button type="button" variant="outline" onClick={onClose}>
                {t('review.back')}
              </Button>
              {outcome ? null : (
                <Button type="button" disabled={!canApply} onClick={onApply}>
                  {applying ? t('review.applying') : t('review.apply')}
                </Button>
              )}
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function statusText(
  t: TFunction<'canvas'>,
  status: 'applied' | 'failed' | 'skipped',
  error: string | undefined,
): string {
  if (status === 'failed') {
    return t('review.stepStatus.failed', { error: error ?? '' })
  }
  return status === 'applied'
    ? t('review.stepStatus.applied')
    : t('review.stepStatus.skipped')
}

function stepText(t: TFunction<'canvas'>, s: ConnectionStep): string {
  if (s.kind === 'disconnect') return t('review.steps.disconnect', s)
  return s.envVar
    ? t('review.steps.connect', s)
    : t('review.steps.connectAuto', s)
}
