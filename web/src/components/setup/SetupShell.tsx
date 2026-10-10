import { useState } from 'react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { CaretDownIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { useBrand } from '../../hooks/useBrand'
import { BrandMarkGlyph } from '../BrandMarkGlyph'
import { SETUP_STEPS } from '../../lib/setupWizard'
import type { SetupStepId } from '../../lib/setupWizard'
import { SaveIndicator } from './SaveIndicator'

/** SetupShell is the onboarding frame: brand header, a progress rail beside one focused card, and the save indicator. */
export function SetupShell({
  current,
  settled,
  rail,
  saving,
  saveError,
  onDismiss,
  dismissable,
  dismissDisabled,
  children,
}: {
  current: SetupStepId
  settled: number
  rail: ReactNode
  saving: boolean
  saveError?: string
  onDismiss: () => void
  dismissable: boolean
  dismissDisabled: boolean
  children: ReactNode
}) {
  const { t } = useTranslation('setup')
  const brand = useBrand()
  const [railOpen, setRailOpen] = useState(false)
  const index = SETUP_STEPS.indexOf(current)

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <header className="flex items-start justify-between gap-4">
        <div className="flex min-w-0 items-center gap-3">
          <span className="flex size-11 shrink-0 items-center justify-center overflow-hidden rounded-xl border border-border bg-card p-2 text-base font-bold text-foreground">
            <BrandMarkGlyph svgWrapperClassName="flex size-full items-center justify-center [&_svg]:size-full" />
          </span>
          <div className="min-w-0">
            <h1 className="truncate text-xl font-semibold tracking-tight text-foreground">
              {t('shell.title', { name: brand.Name })}
            </h1>
            <p className="text-sm text-muted-foreground">
              {t('shell.subtitle')}
            </p>
          </div>
        </div>
        {dismissable ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onDismiss}
            disabled={dismissDisabled}
          >
            {t('shell.dismiss')}
          </Button>
        ) : null}
      </header>

      <div className="grid gap-6 lg:grid-cols-[15rem_minmax(0,1fr)]">
        <aside className="space-y-4 lg:sticky lg:top-4 lg:self-start">
          <div className="rounded-xl border border-border bg-card p-2">
            <button
              type="button"
              aria-expanded={railOpen}
              onClick={() => setRailOpen((v) => !v)}
              className="flex w-full items-center justify-between gap-2 rounded-lg px-2 py-2 text-left text-sm outline-none focus-visible:ring-3 focus-visible:ring-ring/50 lg:hidden"
            >
              <span className="font-medium text-foreground">
                {t(`steps.${current}.title`)}
              </span>
              <span className="flex items-center gap-2 text-xs text-muted-foreground">
                {t('shell.progress', {
                  done: settled,
                  total: SETUP_STEPS.length,
                })}
                <CaretDownIcon
                  className={cn(
                    'size-4 transition-transform motion-reduce:transition-none',
                    railOpen && 'rotate-180',
                  )}
                  aria-hidden="true"
                />
              </span>
            </button>
            <div className={cn(railOpen ? 'block' : 'hidden', 'lg:block')}>
              {rail}
            </div>
          </div>
          <div className="hidden px-1 lg:block">
            <SaveIndicator saving={saving} errorMessage={saveError} />
          </div>
        </aside>

        <section
          aria-labelledby="setup-step-title"
          className="min-w-0 rounded-xl border border-border bg-card p-5 sm:p-6"
        >
          <div className="mb-5 space-y-1">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              {index + 1} / {SETUP_STEPS.length}
            </p>
            <h2
              id="setup-step-title"
              className="text-lg font-semibold tracking-tight text-foreground"
            >
              {t(`steps.${current}.title`)}
            </h2>
          </div>
          <div key={current} className="kit-enter">
            {children}
          </div>
          <div className="mt-6 border-t border-border pt-3 lg:hidden">
            <SaveIndicator saving={saving} errorMessage={saveError} />
          </div>
        </section>
      </div>
    </div>
  )
}
