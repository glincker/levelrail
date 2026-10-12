import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Display, Lede } from '@/components/ui/typography'
import { cn } from '@/lib/utils'

export interface FormSectionProps {
  title: ReactNode
  description?: ReactNode
  /** Step number: renders a small marker aligned to the title. */
  step?: number
  /** Small element beside the title, e.g. a connected badge or a BrandTile. */
  badge?: ReactNode
  /** Section-level validation or server error, announced to screen readers. */
  error?: ReactNode
  /** Docs link or secondary note rendered under the description. */
  hint?: ReactNode
  className?: string
  children: ReactNode
}

/** FormSection puts the description left and the fields right from lg up, one column below. */
export function FormSection({
  title,
  description,
  step,
  badge,
  error,
  hint,
  className,
  children,
}: FormSectionProps) {
  return (
    <section
      className={cn(
        'grid gap-x-8 gap-y-3 border-t border-border py-(--glinr-section-pad) first:border-t-0 first:pt-0 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]',
        className,
      )}
    >
      <div className="space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          {step === undefined ? null : (
            <span
              aria-hidden="true"
              className="flex size-5 shrink-0 items-center justify-center rounded-full bg-muted font-mono text-xs font-medium text-muted-foreground"
            >
              {step}
            </span>
          )}
          <Display as="h2" size="section">
            {title}
          </Display>
          {badge}
        </div>
        {description ? <Lede>{description}</Lede> : null}
        {hint ? <div className="text-sm">{hint}</div> : null}
      </div>
      <div className="min-w-0 max-w-2xl space-y-3">
        {children}
        {error ? (
          <p role="alert" className="text-sm text-tone-danger">
            {error}
          </p>
        ) : null}
      </div>
    </section>
  )
}

export interface StickySaveBarProps {
  dirty: boolean
  saving?: boolean
  onSave: () => void
  onReset?: () => void
  /** Slot for a Test connection button; stays enabled while the form is dirty. */
  testSlot?: ReactNode
  /** Short validation or save summary shown at the left of the bar. */
  message?: ReactNode
  saveLabel?: string
  className?: string
}

/** StickySaveBar slides in at the bottom of the form once it has unsaved changes. */
export function StickySaveBar({
  dirty,
  saving = false,
  onSave,
  onReset,
  testSlot,
  message,
  saveLabel,
  className,
}: StickySaveBarProps) {
  const { t } = useTranslation('common')
  if (!dirty && !saving) return null
  return (
    <div
      role="region"
      aria-label={t('foundation.saveBar.label')}
      className={cn(
        'sticky bottom-4 z-20 flex flex-wrap items-center gap-3 rounded-xl border border-border bg-card px-4 py-3 shadow-floating motion-safe:animate-in motion-safe:fade-in motion-safe:slide-in-from-bottom-2',
        className,
      )}
    >
      <p
        aria-live="polite"
        className="min-w-0 flex-1 basis-40 text-sm text-muted-foreground"
      >
        {message ?? t('foundation.saveBar.unsaved')}
      </p>
      <div className="flex flex-wrap items-center gap-2">
        {testSlot}
        {onReset ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            disabled={saving}
            onClick={onReset}
          >
            {t('foundation.saveBar.discard')}
          </Button>
        ) : null}
        <Button type="button" size="sm" disabled={saving} onClick={onSave}>
          {saving
            ? t('foundation.saveBar.saving')
            : (saveLabel ?? t('foundation.saveBar.save'))}
        </Button>
      </div>
    </div>
  )
}
