import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'

// Capsule chip with a visible, keyboard reachable remove button.
export function RemovableChip({
  label,
  onRemove,
}: {
  label: string
  onRemove: () => void
}) {
  const { t } = useTranslation('uiShared')
  return (
    <span className="inline-flex h-7 items-center gap-1 rounded-glinr-pill border border-border bg-muted/60 pr-1 pl-3 text-sm text-foreground">
      {label}
      <button
        type="button"
        aria-label={t('chip.remove', { label })}
        onClick={onRemove}
        className="flex size-5 items-center justify-center rounded-full text-muted-foreground hover:bg-muted hover:text-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-state-accent"
      >
        <XGlassIcon aria-hidden="true" className="size-3.5" />
      </button>
    </span>
  )
}
