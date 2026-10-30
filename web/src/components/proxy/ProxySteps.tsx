import { useTranslation } from 'react-i18next'
import {
  BanGlassIcon,
  CircleCheckGlassIcon,
  CircleGlassIcon,
  CircleXGlassIcon,
  ClockGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import type { ProxyStep, ProxyStepState } from '../../queries/proxyIntegration'

const ICONS = {
  done: {
    Icon: CircleCheckGlassIcon,
    tone: 'text-green-600 dark:text-green-400',
  },
  todo: { Icon: CircleGlassIcon, tone: 'text-muted-foreground' },
  blocked: { Icon: BanGlassIcon, tone: 'text-amber-600 dark:text-amber-400' },
  error: { Icon: CircleXGlassIcon, tone: 'text-destructive' },
  waiting: { Icon: ClockGlassIcon, tone: 'text-muted-foreground' },
} as const satisfies Record<
  ProxyStepState | 'waiting',
  { Icon: unknown; tone: string }
>

// Steps after an unfinished "detect" cannot start yet, so they wait instead
// of looking like a dead, undone to-do.
function displayState(
  steps: ProxyStep[],
  step: ProxyStep,
): ProxyStepState | 'waiting' {
  const detect = steps.find((s) => s.id === 'detect')
  const stuck = detect?.state === 'blocked' || detect?.state === 'error'
  return stuck && step.id !== 'detect' && step.state === 'todo'
    ? 'waiting'
    : step.state
}

export function ProxySteps({ steps }: { steps: ProxyStep[] }) {
  const { t } = useTranslation('domains')
  return (
    <ol className="min-w-0 space-y-0" aria-label={t('proxySetup.title')}>
      {steps.map((step, i) => {
        const state = displayState(steps, step)
        const { Icon, tone } = ICONS[state]
        const last = i === steps.length - 1
        return (
          <li key={step.id} className="relative flex gap-3 pb-4 last:pb-0">
            {last ? null : (
              <span
                aria-hidden="true"
                className="absolute top-6 bottom-0 left-2.75 w-px bg-border"
              />
            )}
            <Icon
              className={cn('relative size-6 shrink-0 bg-card', tone)}
              aria-hidden="true"
            />
            <div className="min-w-0">
              <p className="text-sm font-medium text-foreground">
                {t(`proxySetup.steps.${step.id}`)}
                <span className="sr-only">
                  {' '}
                  ({t(`proxySetup.stepState.${state}`)})
                </span>
              </p>
              {state === 'waiting' ? (
                <p className="text-xs text-muted-foreground">
                  {t('proxySetup.waitingDetail')}
                </p>
              ) : step.detail ? (
                <p className="text-xs break-words text-muted-foreground">
                  {step.detail}
                </p>
              ) : null}
            </div>
          </li>
        )
      })}
    </ol>
  )
}
