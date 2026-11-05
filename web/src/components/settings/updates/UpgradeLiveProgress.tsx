import { useTranslation } from 'react-i18next'
import {
  LoaderCircleGlassIcon,
  TriangleAlertGlassIcon,
  WifiOffGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import {
  downloadPercent,
  formatBytes,
  type ActionState,
} from '@/lib/upgradeControl'
import { formatElapsed } from '@/lib/upgradeTime'

export interface UpgradeLiveProgressProps {
  step: string
  startedAtMs: number
  lastProgressMs: number
  now: number
  downloadedBytes?: number
  totalBytes?: number
  stalled: boolean
  reconnecting: boolean
  actions: ActionState
  busy: boolean
  onCancel: () => void
  onRetry: () => void
  onRestart: () => void
  onRetryNow: () => void
}

// The active step with real numbers. A stalled run swaps the spinner for a
// callout with ways out, so the page never spins without end.
export function UpgradeLiveProgress({
  step,
  startedAtMs,
  lastProgressMs,
  now,
  downloadedBytes,
  totalBytes,
  stalled,
  reconnecting,
  actions,
  busy,
  onCancel,
  onRetry,
  onRestart,
  onRetryNow,
}: UpgradeLiveProgressProps) {
  const { t } = useTranslation('updates')
  const percent = downloadPercent(downloadedBytes, totalBytes)
  const stepName = step
    ? t(`selfUpgrade.step.${step}`, { defaultValue: step })
    : t('control.starting')
  return (
    <section aria-label={t('control.liveLabel')} className="space-y-2">
      {reconnecting ? (
        <div
          role="alert"
          className="flex flex-wrap items-center gap-2 rounded-md border border-tone-warning-border bg-tone-warning-soft px-3 py-2 text-sm text-tone-warning"
        >
          <WifiOffGlassIcon size="1.4em" aria-hidden="true" />
          <span className="min-w-0 flex-1">{t('control.reconnecting')}</span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={onRetryNow}
          >
            {t('control.retryNow')}
          </Button>
        </div>
      ) : null}
      {stalled ? (
        <div
          role="alert"
          className="space-y-3 rounded-md border border-tone-warning-border bg-tone-warning-soft p-3 text-sm"
        >
          <p className="flex items-center gap-2 font-medium text-tone-warning">
            <TriangleAlertGlassIcon size="1.4em" aria-hidden="true" />
            {t('control.stuckTitle', { step: stepName })}
          </p>
          <p className="text-muted-foreground">
            {t('control.stuckBody', {
              time: formatElapsed(now - lastProgressMs),
            })}
          </p>
          <ul className="space-y-1 text-xs text-muted-foreground">
            {actions.showCancel ? <li>{t('control.cancelHint')}</li> : null}
            {actions.showRetry ? <li>{t('control.retryHint')}</li> : null}
            <li>{t('control.restartHint')}</li>
          </ul>
          <div className="flex flex-wrap gap-2">
            {actions.showCancel ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={busy || actions.cancelling}
                onClick={onCancel}
              >
                {actions.cancelling
                  ? t('control.cancelling')
                  : t('control.cancel')}
              </Button>
            ) : null}
            {actions.showRetry ? (
              <Button type="button" size="sm" disabled={busy} onClick={onRetry}>
                {t('control.retry')}
              </Button>
            ) : null}
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={busy}
              onClick={onRestart}
            >
              {t('control.restartServer')}
            </Button>
          </div>
        </div>
      ) : (
        <div className="space-y-1.5 rounded-md border border-border p-3">
          <div className="flex items-center justify-between gap-2 text-sm">
            <span className="flex min-w-0 items-center gap-2 font-medium">
              <LoaderCircleGlassIcon
                size="1.4em"
                className="shrink-0 text-primary motion-safe:animate-spin"
                aria-hidden="true"
              />
              <span className="truncate">{stepName}</span>
            </span>
            <span className="shrink-0 font-mono text-xs tabular-nums text-muted-foreground">
              {t('control.elapsed', {
                time: formatElapsed(now - startedAtMs),
              })}
            </span>
          </div>
          {percent !== null ? (
            <Progress
              value={percent}
              aria-label={t('control.progressLabel', { step: stepName })}
            />
          ) : (
            <div
              role="progressbar"
              aria-label={t('control.progressLabel', { step: stepName })}
              className="h-1 w-full overflow-hidden rounded-full bg-muted"
            >
              <div className="h-full w-1/3 rounded-full bg-primary motion-safe:animate-pulse" />
            </div>
          )}
          <p className="flex flex-wrap justify-between gap-x-3 text-xs text-muted-foreground">
            <span>
              {percent !== null && downloadedBytes !== undefined && totalBytes
                ? t('control.bytes', {
                    done: formatBytes(downloadedBytes),
                    total: formatBytes(totalBytes),
                    percent,
                  })
                : t('control.sizeUnknown')}
            </span>
            <span>
              {t('control.lastProgress', {
                time: formatElapsed(now - lastProgressMs),
              })}
            </span>
          </p>
        </div>
      )}
    </section>
  )
}
