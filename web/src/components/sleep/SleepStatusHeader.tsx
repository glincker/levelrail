import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  MoonIcon,
  ProhibitIcon,
  SunIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { formatBytes } from '../../lib/format'
import { formatSleepDuration } from '../../lib/sleepTimeline'
import type { SleepUiState } from '../../lib/sleepState'
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
      <MoonIcon
        className={`${swap} size-5 ${asleep ? 'opacity-100' : 'rotate-[-30deg] opacity-0'}`}
      />
      <SunIcon
        className={`${swap} size-5 ${asleep ? 'rotate-[30deg] opacity-0' : 'opacity-100'}`}
      />
    </span>
  )
}

function StatusIcon({ state }: { state: SleepUiState }) {
  if (state === 'blocked') return <ProhibitIcon className="size-3" />
  if (state === 'ready') return <CheckCircleIcon className="size-3" />
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
  const coldStart = sleep.cold_start_ms_p50
  const explain =
    coldStart === null
      ? t('appSleep.explain.generic')
      : t('appSleep.explain.withCold', {
          duration: formatSleepDuration(coldStart / 1000),
        })
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
      <p className="text-sm text-muted-foreground">{explain}</p>
      {freed !== null && freed > 0 ? (
        <p className="text-sm text-foreground">
          {t('appSleep.frees', { size: formatBytes(freed) })}
        </p>
      ) : null}
    </div>
  )
}
