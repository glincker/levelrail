import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { CleanupMode } from '../../types/cleanup'

const MODES: CleanupMode[] = ['off', 'preview', 'on']

/** Segmented Off, Preview, On picker; a radiogroup so arrows and focus behave as users expect. */
export function ModeControl({
  value,
  disabled,
  ariaLabel,
  onChange,
}: {
  value: CleanupMode
  disabled?: boolean
  ariaLabel: string
  onChange: (mode: CleanupMode) => void
}) {
  const { t } = useTranslation('cleanup')
  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      className="inline-flex h-7 items-center rounded-md border border-border bg-muted/40 p-0.5 pointer-coarse:h-11"
    >
      {MODES.map((m) => {
        const checked = m === value
        return (
          <button
            key={m}
            type="button"
            role="radio"
            aria-checked={checked}
            disabled={disabled}
            onClick={() => {
              if (!checked) onChange(m)
            }}
            className={cn(
              'h-full rounded px-2.5 text-xs font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
              checked
                ? 'bg-background text-foreground shadow-sm'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {t(`target.modes.${m}`)}
          </button>
        )
      })}
    </div>
  )
}
