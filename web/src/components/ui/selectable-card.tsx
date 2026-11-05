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
