import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Button } from '@/components/ui/button'
import { ConfirmAction } from '@/components/ui/confirm-action'
import { Progress } from '@/components/ui/progress'
import { toast } from '@/components/ui/toast'
import { useRollbackToDeploy } from '../queries/deployControl'
import { useRollbackTargets, useStopWatch } from '../queries/deployGate'
import type { WatchStatus } from '../types/deployGate'

const SECONDS_PER_MINUTE = 60

function duration(seconds: number, t: TFunction<'deploySafety'>): string {
  return seconds >= SECONDS_PER_MINUTE
    ? t('watch.min', { count: Math.round(seconds / SECONDS_PER_MINUTE) })
    : t('watch.sec', { count: seconds })
}

/** WatchSummary says where a release's post-deploy watch stands, with progress while it runs. */
export function WatchSummary({ watch }: { watch: WatchStatus }) {
  const { t } = useTranslation('deploySafety')
  const message = watch.message ?? ''
  const reason = watch.reason ?? ''
  const elapsed = Math.max(0, watch.window_seconds - watch.remaining_seconds)
  const pct =
    watch.window_seconds > 0
      ? Math.min(100, Math.round((elapsed / watch.window_seconds) * 100))
      : 0

  return (
    <div className="space-y-2">
      {watch.state === 'watching' ? (
        <Progress value={pct} aria-label={t('watch.progress')}>
          <p className="text-sm text-foreground">
            {t('watch.watching', {
              window: duration(watch.window_seconds, t),
              left: duration(watch.remaining_seconds, t),
            })}
          </p>
        </Progress>
      ) : null}
      {watch.state === 'tripped' ? (
        <p className="text-sm text-foreground">
          {t('watch.tripped', { message, reason })}
        </p>
      ) : null}
      {watch.state === 'rolled_back' && reason ? (
        <p className="text-sm text-foreground">
          {t('watch.rolledBack', { message, reason })}
        </p>
      ) : null}
      {watch.state === 'stopped' ? (
        <p className="text-sm text-muted-foreground">{t('watch.stopped')}</p>
      ) : null}
      {watch.state === 'off' ? (
        <p className="text-sm text-muted-foreground">{t('watch.off')}</p>
      ) : null}
      {watch.notice ? (
        <p className="text-sm text-muted-foreground">
          {t('watch.notice', { message: watch.notice })}
        </p>
      ) : null}
    </div>
  )
}

/** WatchActions offers Stop watching and Roll back now while a watch is open. */
export function WatchActions({ appName }: { appName: string }) {
  const { t } = useTranslation('deploySafety')
  const stop = useStopWatch(appName)
  const rollback = useRollbackToDeploy(appName)
  const { data: targets } = useRollbackTargets(appName)
  const target = targets?.find((x) => !x.live)

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={stop.isPending}
        onClick={() =>
          stop.mutate(undefined, {
            onSuccess: () =>
              toast.add({ title: t('watch.stopped_toast'), type: 'success' }),
            onError: (e) =>
              toast.add({
                title: t('watch.stopError'),
                description: e.message,
                type: 'error',
              }),
          })
        }
      >
        {t('watch.stop')}
      </Button>
      <ConfirmAction
        trigger={(open) => (
          <Button
            type="button"
            size="sm"
            variant="destructive"
            disabled={!target}
            onClick={open}
          >
            {t('watch.rollbackNow')}
          </Button>
        )}
        title={t('watch.rollbackTitle')}
        description={t('watch.rollbackDescription')}
        consequences={[
          t('watch.rollbackConsequence', { ref: target?.image ?? '' }),
        ]}
        confirmLabel={t('watch.rollbackConfirm')}
        pendingLabel={t('watch.rollbackPending')}
        pending={rollback.isPending}
        error={rollback.error?.message ?? null}
        onConfirm={(close) => {
          if (!target) return
          rollback.mutate(
            { deployId: target.deploy_id, confirm: false },
            {
              onSuccess: () => {
                toast.add({ title: t('watch.rollbackToast'), type: 'success' })
                close()
              },
            },
          )
        }}
      />
      {!target ? (
        <p className="text-xs text-muted-foreground">{t('watch.noTarget')}</p>
      ) : null}
    </div>
  )
}
