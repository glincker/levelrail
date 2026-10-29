import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { STEP_IDS, type StepId } from '../../lib/domainStepper'

// Horizontal progress list; finished steps are buttons so a step can be revisited.
export function StepperNav({
  current,
  reached,
  onGo,
}: {
  current: StepId
  reached: number
  onGo: (id: StepId) => void
}) {
  const { t } = useTranslation('domains')
  return (
    <ol className="flex flex-wrap gap-1.5" aria-label={t('stepper.title')}>
      {STEP_IDS.map((id, i) => {
        const done = i < reached
        const active = id === current
        return (
          <li key={id}>
            <button
              type="button"
              disabled={i > reached}
              aria-current={active ? 'step' : undefined}
              onClick={() => {
                onGo(id)
              }}
              className={cn(
                'flex min-h-11 items-center gap-1.5 rounded-md border px-2.5 text-xs transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring motion-reduce:transition-none sm:min-h-8',
                active
                  ? 'border-primary bg-primary/10 text-foreground'
                  : 'border-border text-muted-foreground',
                i > reached && 'opacity-50',
              )}
            >
              {done && !active ? (
                <CircleCheckGlassIcon
                  className="size-4 text-emerald-600 dark:text-emerald-400"
                  aria-hidden="true"
                />
              ) : (
                <span aria-hidden="true" className="font-mono">
                  {i + 1}
                </span>
              )}
              {t(`stepper.step.${id}`)}
            </button>
          </li>
        )
      })}
    </ol>
  )
}

export function StepBody({ children }: { children: ReactNode }) {
  return <div className="space-y-2">{children}</div>
}
