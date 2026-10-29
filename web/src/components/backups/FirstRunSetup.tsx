import { useTranslation } from 'react-i18next'
import { UploadGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { TargetSetupForm } from './TargetSetupForm'

const STEPS = ['destination', 'schedule', 'verify'] as const

/** FirstRunSetup walks a fresh install from no destination to a verified backup. */
export function FirstRunSetup({ unprotected }: { unprotected: number }) {
  const { t } = useTranslation('backups')
  return (
    <section
      className="space-y-5 rounded-lg border border-border p-4 sm:p-6"
      aria-labelledby="first-run-title"
    >
      <div className="flex items-start gap-3">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <UploadGlassIcon className="size-5.5" aria-hidden="true" />
        </div>
        <div>
          <h2 id="first-run-title" className="text-lg font-semibold">
            {t('firstRun.title')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {unprotected > 0
              ? t('firstRun.descriptionWithResources', { count: unprotected })
              : t('firstRun.description')}
          </p>
        </div>
      </div>
      <ol className="grid gap-2 sm:grid-cols-3">
        {STEPS.map((step, i) => (
          <li
            key={step}
            aria-current={i === 0 ? 'step' : undefined}
            className={cn(
              'rounded-md border px-3 py-2 text-sm',
              i === 0 ? 'border-primary bg-primary/5' : 'border-border',
            )}
          >
            <span className="block font-medium">
              {i + 1}. {t(`firstRun.step.${step}.title`)}
            </span>
            <span className="block text-xs text-muted-foreground">
              {t(`firstRun.step.${step}.hint`)}
            </span>
          </li>
        ))}
      </ol>
      <TargetSetupForm onCreated={() => undefined} />
    </section>
  )
}
