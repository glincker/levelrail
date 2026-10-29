import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BanGlassIcon,
  CircleCheckGlassIcon,
  MoonGlassIcon,
  SunGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { formatBytes } from '../../lib/format'
import {
  formatWakeMs,
  wakeEstimate,
  type SleepUiState,
} from '../../lib/sleepState'
import type { AppSleep } from '../../queries/appSleep'

const BADGE_VARIANT = {
  ready: 'success',
  blocked: 'destructive',
  awake: 'success',
  sleeping: 'info',
  waking: 'warning',
} as const

function MoonSunIcon({ asleep, waking }: { asleep: boolean; waking: boolean }) {
  const swap =
    'absolute inset-0 motion-safe:transition-[opacity,transform] motion-safe:duration-300'
  return (
    <span
      aria-hidden="true"
      className={`relative size-5 shrink-0 ${waking ? 'motion-safe:animate-pulse' : ''}`}
    >
      <MoonGlassIcon
        className={`${swap} size-6 ${asleep ? 'opacity-100' : 'rotate-[-30deg] opacity-0'}`}
      />
      <SunGlassIcon
        className={`${swap} size-6 ${asleep ? 'rotate-[30deg] opacity-0' : 'opacity-100'}`}
      />
    </span>
  )
}

function StatusIcon({ state }: { state: SleepUiState }) {
  if (state === 'blocked') return <BanGlassIcon className="size-4.5" />
  if (state === 'ready')
    return <CircleCheckGlassIcon className="size-4.5 text-tone-success" />
  return null
}

/** SleepStatusHeader names the current sleep state and says what sleeping does for this app. */
export function SleepStatusHeader({
  state,
  sleep,
  actions,
}: {
  state: SleepUiState
  sleep: AppSleep
  actions?: ReactNode
}) {
  const { t } = useTranslation('deploys')
  const wake = wakeEstimate(sleep.wake_timing)
  const freed = sleep.estimated_memory_freed_bytes
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <MoonSunIcon
            asleep={state === 'sleeping'}
            waking={state === 'waking'}
          />
          <Badge
            variant={BADGE_VARIANT[state]}
            role="status"
            aria-live="polite"
            data-state={state}
          >
            <StatusIcon state={state} />
            {t(`appSleep.status.${state}`)}
          </Badge>
        </div>
        {actions}
      </div>
      <p className="text-sm text-muted-foreground">
        {t('appSleep.explain.generic')}
      </p>
      <p className="text-sm text-foreground" data-testid="sleep-wake-timing">
        {wake === null
          ? t('appSleep.wakeTiming.none')
          : wake.p95Ms === null
            ? t('appSleep.wakeTiming.typical', {
                typical: formatWakeMs(wake.typicalMs),
              })
            : t('appSleep.wakeTiming.typicalWithP95', {
                typical: formatWakeMs(wake.typicalMs),
                p95: formatWakeMs(wake.p95Ms),
              })}
      </p>
      {freed !== null && freed > 0 ? (
        <p className="text-sm text-foreground">
          {t('appSleep.frees', { size: formatBytes(freed) })}
        </p>
      ) : null}
    </div>
  )
}
