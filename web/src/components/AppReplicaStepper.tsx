import { CircleMinusGlassIcon, CirclePlusGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useUpdateApp } from '../queries/apps'
import type { AppDetail } from '../types/appDetail'

// One-step replica control. The server owns the upper bound and rejects a
// step past it; an active autoscale policy owns the count, so we disable.
export function AppReplicaStepper({
  app,
  autoscaled,
}: {
  app: AppDetail
  autoscaled: boolean
}) {
  const { t } = useTranslation('resources', { keyPrefix: 'scale.stepper' })
  const update = useUpdateApp(app.name)
  const step = (delta: number) =>
    update.mutate({ ...app, replicas: app.replicas + delta })
  const busy = update.isPending

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label={t('less')}
          disabled={autoscaled || busy || app.replicas <= 1}
          onClick={() => step(-1)}
        >
          <CircleMinusGlassIcon className="size-4" />
        </Button>
        <output className="min-w-8 text-center text-sm tabular-nums">
          {app.replicas}
        </output>
        <Button
          type="button"
          variant="outline"
          size="icon"
          aria-label={t('more')}
          disabled={autoscaled || busy}
          onClick={() => step(1)}
        >
          <CirclePlusGlassIcon className="size-4" />
        </Button>
      </div>
      {autoscaled ? (
        <p className="text-sm text-muted-foreground">{t('autoscaleOwns')}</p>
      ) : null}
      {update.error ? (
        <p role="alert" className="text-sm text-destructive">
          {update.error.message}
        </p>
      ) : null}
    </div>
  )
}
