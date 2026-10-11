import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  CircleIcon,
  CircleNotchIcon,
  MinusCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'

export type StepState = 'todo' | 'active' | 'done' | 'failed' | 'skipped'

export interface Step {
  id: string
  title: string
  detail?: string
  state: StepState
  action?: ReactNode
}

const STATE_ICON: Record<StepState, Icon> = {
  todo: CircleIcon,
  active: CircleNotchIcon,
  done: CheckCircleIcon,
  failed: XCircleIcon,
  skipped: MinusCircleIcon,
}

const STATE_TONE: Record<StepState, string> = {
  todo: 'text-muted-foreground',
  active: 'text-status-info-foreground',
  done: 'text-green-700 dark:text-green-300',
  failed: 'text-destructive',
  skipped: 'text-muted-foreground',
}

export interface StepListProps {
  steps: readonly Step[]
  orientation?: 'vertical' | 'horizontal'
  compact?: boolean
  className?: string
}

// Announces the step that is moving, not every step on every render.
function liveMessage(
  steps: readonly Step[],
  label: (state: StepState) => string,
): string {
  const moving =
    steps.find((s) => s.state === 'failed') ??
    steps.find((s) => s.state === 'active')
  return moving ? `${moving.title}: ${label(moving.state)}` : ''
}

/** Ordered progress steps shared by add domain, go live, wizards and setup. */
export function StepList({
  steps,
  orientation = 'vertical',
  compact = false,
  className,
}: Readonly<StepListProps>) {
  const { t } = useTranslation('traffic')
  const stateLabel = (state: StepState) => t(`steps.state.${state}`)

  return (
    <div className={className}>
      <ol
        className={cn(
          'flex gap-3',
          orientation === 'horizontal'
            ? 'flex-col sm:flex-row sm:items-start'
            : 'flex-col',
          compact && 'gap-2',
        )}
      >
        {steps.map((step, index) => {
          const StepIcon = STATE_ICON[step.state]
          return (
            <li
              key={step.id}
              aria-current={step.state === 'active' ? 'step' : undefined}
              data-state={step.state}
              className={cn(
                'flex min-w-0 items-start gap-2.5',
                orientation === 'horizontal' && 'sm:flex-1',
              )}
            >
              <StepIcon
                aria-hidden="true"
                weight={step.state === 'todo' ? 'regular' : 'fill'}
                className={cn(
                  'mt-0.5 size-4 shrink-0',
                  STATE_TONE[step.state],
                  step.state === 'active' &&
                    'animate-spin motion-reduce:animate-none',
                )}
              />
              <div className="min-w-0 flex-1 space-y-0.5">
                <p
                  className={cn(
                    'text-sm',
                    step.state === 'todo' || step.state === 'skipped'
                      ? 'text-muted-foreground'
                      : 'font-medium text-foreground',
                  )}
                >
                  <span className="sr-only">
                    {t('steps.position', {
                      current: index + 1,
                      total: steps.length,
                      state: stateLabel(step.state),
                    })}
                    {'. '}
                  </span>
                  {step.title}
                </p>
                {step.detail && !compact ? (
                  <p
                    className={cn(
                      'text-xs',
                      step.state === 'failed'
                        ? 'text-destructive'
                        : 'text-muted-foreground',
                    )}
                  >
                    {step.detail}
                  </p>
                ) : null}
                {step.state === 'failed' && step.action ? (
                  <div className="pt-1">{step.action}</div>
                ) : null}
              </div>
            </li>
          )
        })}
      </ol>
      <p className="sr-only" role="status" aria-live="polite">
        {liveMessage(steps, stateLabel)}
      </p>
    </div>
  )
}
