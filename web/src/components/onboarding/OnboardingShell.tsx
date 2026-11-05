import { useEffect, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { ONBOARDING_STEPS } from './onboardingSteps'

export interface OnboardingShellProps {
  index: number
  children: ReactNode
  busy?: boolean
  /** Label of the primary button; defaults to Continue. */
  continueLabel?: string
  onBack: () => void
  onSkip: () => void
  onContinue: () => void
}

/** OnboardingShell frames one step: a rail (compact stepper under lg), the step body and a footer bar. */
export function OnboardingShell({
  index,
  children,
  busy = false,
  continueLabel,
  onBack,
  onSkip,
  onContinue,
}: OnboardingShellProps) {
  const { t } = useTranslation('onboarding')
  const headingRef = useRef<HTMLHeadingElement>(null)
  const first = useRef(true)
  const id = ONBOARDING_STEPS[index] ?? ONBOARDING_STEPS[0]

  // Move focus to the heading on a step change so screen readers announce it; never on first paint.
  useEffect(() => {
    if (first.current) {
      first.current = false
      return
    }
    headingRef.current?.focus()
  }, [index])

  return (
    <div className="grid gap-6 lg:grid-cols-[13rem_minmax(0,1fr)]">
      <nav aria-label={t('rail.label')} className="hidden lg:block">
        <ol className="space-y-1">
          {ONBOARDING_STEPS.map((step, i) => {
            const done = i < index
            const current = i === index
            return (
              <li
                key={step}
                aria-current={current ? 'step' : undefined}
                className={cn(
                  'flex items-center gap-2.5 rounded-md px-2 py-1.5 text-sm',
                  current
                    ? 'bg-muted font-medium text-foreground'
                    : 'text-muted-foreground',
                )}
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    'flex size-5 shrink-0 items-center justify-center rounded-full border text-[0.65rem] tabular-nums',
                    done
                      ? 'border-transparent bg-state-accent text-background'
                      : 'border-border',
                  )}
                >
                  {done ? <CheckGlassIcon className="size-3.5" /> : i + 1}
                </span>
                {t(`steps.${step}.label`)}
                {done ? (
                  <span className="sr-only">{t('rail.done')}</span>
                ) : null}
              </li>
            )
          })}
        </ol>
      </nav>

      <div className="min-w-0 space-y-5">
        <p className="text-xs text-muted-foreground tabular-nums lg:hidden">
          {t('rail.progress', {
            current: index + 1,
            total: ONBOARDING_STEPS.length,
            name: t(`steps.${id}.label`),
          })}
        </p>
        <div className="max-w-2xl space-y-1">
          <h2
            ref={headingRef}
            tabIndex={-1}
            className="text-lg font-semibold text-foreground outline-none"
          >
            {t(`steps.${id}.title`)}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t(`steps.${id}.description`)}
          </p>
        </div>
        <div className="max-w-2xl">{children}</div>
        <div className="flex max-w-2xl flex-wrap items-center gap-2 border-t border-border pt-4">
          {index > 0 ? (
            <Button
              type="button"
              variant="outline"
              disabled={busy}
              onClick={onBack}
            >
              {t('footer.back')}
            </Button>
          ) : null}
          <Button
            type="button"
            variant="ghost"
            className="ml-auto"
            disabled={busy}
            onClick={onSkip}
          >
            {t('footer.skip')}
          </Button>
          <Button type="button" disabled={busy} onClick={onContinue}>
            {continueLabel ?? t('footer.continue')}
          </Button>
        </div>
      </div>
    </div>
  )
}
