import type { ReactNode } from 'react'
import { SelectableCard } from './ui/selectable-card'
import { cn } from '@/lib/utils'

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
    <SelectableCard
      selected={selected}
      onSelect={onClick}
      className={cn(
        'rounded-md py-1 pr-6 pl-2.5 text-sm',
        !selected && 'text-muted-foreground',
      )}
    >
      {children}
    </SelectableCard>
  )
}
