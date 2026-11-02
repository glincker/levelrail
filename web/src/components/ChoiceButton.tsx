import { selectableCardClass } from './ui/selectable-card'
import type { ReactNode } from 'react'

export function ChoiceButton({
  selected,
  onClick,
  children,
}: {
  selected: boolean
  onClick: () => void
  children: ReactNode
}) {
  return (
    <button
      type="button"
      aria-pressed={selected}
      onClick={onClick}
      className={selectableCardClass(
        selected,
        `rounded-md px-2.5 py-1 text-sm ${selected ? '' : 'text-muted-foreground'}`,
      )}
    >
      {children}
    </button>
  )
}
