import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from '@/components/ui/progress'
import type { Canary } from '../queries/canary'

function percent(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

function millis(value: number): string {
  return `${Math.round(value)} ms`
}

/** CanaryProgressPanel shows a staged canary's stage bar and its live comparison with the stable release. */
export function CanaryProgressPanel({ canary }: { canary: Canary }) {
  const { t } = useTranslation('deploys')
  const stages = canary.stages ?? []
  const stage = canary.stage ?? 1
  const progress = canary.progress
  const breaching = progress?.breaching ?? false

  return (
    <div className="space-y-3">
      <Progress value={(stage / Math.max(stages.length, 1)) * 100}>
        <ProgressLabel>
          {t('canary.stageLabel', { stage, count: stages.length })}
        </ProgressLabel>
        <ProgressValue>
          {() => t('canary.stageWeights', { stages: stages.join(' / ') })}
        </ProgressValue>
      </Progress>
      {canary.status_reason ? (
        <Alert variant="destructive">
          <AlertDescription>{canary.status_reason}</AlertDescription>
        </Alert>
      ) : null}
      {progress ? (
        <>
          <table
            className="w-full text-sm"
            aria-label={t('canary.compareLabel')}
          >
            <thead>
              <tr className="text-left text-xs text-muted-foreground">
                <th className="font-normal" />
                <th className="font-normal">{t('canary.canaryColumn')}</th>
                <th className="font-normal">{t('canary.stableColumn')}</th>
              </tr>
            </thead>
            <tbody className="tabular-nums">
              <tr>
                <th scope="row" className="font-normal text-muted-foreground">
                  {t('canary.requests')}
                </th>
                <td>{progress.canary_requests}</td>
                <td>{progress.stable_requests}</td>
              </tr>
              <tr>
                <th scope="row" className="font-normal text-muted-foreground">
                  {t('canary.errorRate')}
                </th>
                <td className={breaching ? 'text-destructive' : undefined}>
                  {percent(progress.canary_error_rate)}
                </td>
                <td>{percent(progress.stable_error_rate)}</td>
              </tr>
              <tr>
                <th scope="row" className="font-normal text-muted-foreground">
                  {t('canary.p95')}
                </th>
                <td className={breaching ? 'text-destructive' : undefined}>
                  {millis(progress.canary_p95_ms)}
                </td>
                <td>{millis(progress.stable_p95_ms)}</td>
              </tr>
            </tbody>
          </table>
          <p className="text-xs text-muted-foreground" role="status">
            {breaching
              ? t('canary.breaching', { reason: progress.next })
              : t('canary.next', {
                  reason: progress.next,
                  elapsed: progress.elapsed_secs,
                  interval: progress.interval_secs,
                })}
          </p>
        </>
      ) : (
        <p className="text-xs text-muted-foreground" role="status">
          {t('canary.waitingForTraffic')}
        </p>
      )}
    </div>
  )
}

/** CanaryOutcomeNotice says how the last canary ended, and why when it was rolled back. */
export function CanaryOutcomeNotice({ canary }: { canary: Canary }) {
  const { t } = useTranslation('deploys')
  const last = canary.last_outcome
  if (!last) return null
  const rolledBack = last.outcome === 'rolled_back'
  return (
    <Alert variant={rolledBack ? 'destructive' : 'default'}>
      <AlertTitle>{t(`canary.outcome.${last.outcome}`)}</AlertTitle>
      <AlertDescription>
        {t('canary.outcomeDetail', {
          image: last.image ?? '',
          reason: last.reason ?? '',
        })}
      </AlertDescription>
    </Alert>
  )
}
