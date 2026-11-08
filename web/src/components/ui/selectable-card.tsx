import type { ReactNode } from 'react'
import { CheckGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'

// Keyboard-only focus ring so a mouse click never leaves a bright outline.
const FOCUS =
  'focus:outline-none focus-visible:ring-2 focus-visible:ring-state-accent focus-visible:ring-offset-2 focus-visible:ring-offset-background motion-reduce:transition-none'

// Shared look for any pick-one card or option. Tokens live in index.css.
export function selectableCardClass(selected: boolean, className?: string) {
  return cn(
    'relative border transition-colors',
    FOCUS,
    selected
      ? 'border-state-selected-border bg-state-selected text-foreground'
      : 'border-border hover:bg-muted/60',
    className,
  )
}

// Non-color cue for the selected state; place inside a relative parent.
export function SelectedMark({ selected }: { selected: boolean }) {
  if (!selected) return null
  return (
    <CheckGlassIcon
      aria-hidden="true"
      className="pointer-events-none absolute top-1.5 right-1.5 size-3.5 text-state-accent"
    />
  )
}

// Pick-one card: layer edge, aria-pressed and a check mark, so selection never relies on colour alone.
export function SelectableCard({
  selected,
  onSelect,
  children,
  className,
  disabled,
}: {
  selected: boolean
  onSelect: () => void
  children: ReactNode
  className?: string
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      disabled={disabled}
      onClick={onSelect}
      className={selectableCardClass(
        selected,
        cn(
          'banner-edge rounded-glinr-card p-4 text-left disabled:opacity-50',
          className,
        ),
      )}
    >
      {children}
      <SelectedMark selected={selected} />
    </button>
  )
}
