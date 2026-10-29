import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { MoonGlassIcon } from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import { useAppSleepActions } from '../../queries/appSleep'
import { useSleepFleet, type SleepFleetApp } from '../../queries/sleepFleet'
import { formatSize } from '../../lib/format'
import { formatWakeMs, wakeEstimate } from '../../lib/sleepState'

const LIST_LIMIT = 5

function SleepingRow({ app }: { app: SleepFleetApp }) {
  const { t } = useTranslation('deploys')
  const { wake } = useAppSleepActions(app.name)
  const estimate = wakeEstimate(app.wake_timing)
  return (
    <li className="flex items-center justify-between gap-3 py-1.5">
      <span className="flex min-w-0 items-center gap-2">
        <span
          aria-hidden="true"
          className="size-1.5 shrink-0 rounded-full bg-primary motion-safe:animate-pulse"
        />
        <Link
          to="/apps/$name/deploys"
          params={{ name: app.name }}
          className="truncate text-sm text-foreground hover:underline"
        >
          {app.name}
        </Link>
      </span>
      <span className="flex shrink-0 items-center gap-3">
        <span className="text-xs tabular-nums text-muted-foreground">
          {estimate === null
            ? t('appSleep.wakeTiming.noneShort')
            : estimate.p95Ms === null
              ? t('appSleep.wakeTiming.short', {
                  typical: formatWakeMs(estimate.typicalMs),
                })
              : t('appSleep.wakeTiming.shortWithP95', {
                  typical: formatWakeMs(estimate.typicalMs),
                  p95: formatWakeMs(estimate.p95Ms),
                })}
        </span>
        {app.memory_limit_bytes > 0 ? (
          <span className="text-xs tabular-nums text-muted-foreground">
            {formatSize(app.memory_limit_bytes)}
          </span>
        ) : null}
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={wake.isPending}
          onClick={() =>
            wake.mutate(undefined, {
              onError: (error) =>
                toast.add({
                  title: t('appSleep.toast.errorTitle'),
                  description: error.message,
                  type: 'error',
                }),
            })
          }
        >
          {t('appSleep.wakeNow')}
        </Button>
      </span>
    </li>
  )
}

/** SleepFleetCard shows how many apps are asleep and the memory that frees up. */
export function SleepFleetCard() {
  const { t } = useTranslation('deploys')
  const { data } = useSleepFleet()
  if (!data) return null

  const sleeping = data.apps.filter((a) => a.sleeping)

  return (
    <Card id="sleeping-apps-card" className="overflow-hidden">
      <CardContent className="space-y-4 p-5">
        <div className="flex items-start justify-between gap-4">
          <div className="flex items-center gap-3">
            <span className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <MoonGlassIcon className="size-5.5" aria-hidden="true" />
            </span>
            <div>
              <h3 className="text-sm font-semibold text-foreground">
                {t('appSleep.fleetTitle')}
              </h3>
              <p className="text-xs text-muted-foreground">
                {data.enabled === 0
                  ? t('appSleep.fleetEmptyShort')
                  : t('appSleep.fleetSummary', {
                      sleeping: data.sleeping,
                      enabled: data.enabled,
                    })}
              </p>
            </div>
          </div>
          {data.reclaimed_bytes > 0 ? (
            <div className="text-right">
              <p className="text-2xl font-semibold tabular-nums text-foreground">
                {formatSize(data.reclaimed_bytes)}
              </p>
              <p className="text-xs text-muted-foreground">
                {t('appSleep.fleetReclaimedLabel')}
              </p>
            </div>
          ) : null}
        </div>

        {data.enabled > 0 ? (
          <div
            role="img"
            aria-label={t('appSleep.fleetSummary', {
              sleeping: data.sleeping,
              enabled: data.enabled,
            })}
            className="flex h-1.5 gap-0.5"
          >
            {data.apps.map((a) => (
              <span
                key={a.name}
                className={`h-full flex-1 rounded-full ${a.sleeping ? 'bg-primary' : 'bg-muted'}`}
              />
            ))}
          </div>
        ) : null}

        {data.sleeping > 0 && data.reclaimed_bytes === 0 ? (
          <p className="text-xs text-muted-foreground">
            {t('appSleep.fleetNoLimits')}
          </p>
        ) : null}

        {data.enabled === 0 ? (
          <div className="flex items-center justify-between gap-4 rounded-lg border border-dashed border-border p-3">
            <p className="text-xs text-muted-foreground">
              {t('appSleep.fleetEmpty')}
            </p>
            <Link
              to="/apps"
              className={buttonVariants({ size: 'sm', variant: 'outline' })}
            >
              {t('appSleep.fleetBrowseApps')}
            </Link>
          </div>
        ) : sleeping.length > 0 ? (
          <ul className="divide-y divide-border">
            {sleeping.slice(0, LIST_LIMIT).map((a) => (
              <SleepingRow key={a.name} app={a} />
            ))}
          </ul>
        ) : (
          <p className="text-xs text-muted-foreground">
            {t('appSleep.fleetAllAwake')}
          </p>
        )}
        {sleeping.length > LIST_LIMIT ? (
          <p className="text-xs text-muted-foreground">
            {t('appSleep.fleetMore', { count: sleeping.length - LIST_LIMIT })}
          </p>
        ) : null}
      </CardContent>
    </Card>
  )
}
