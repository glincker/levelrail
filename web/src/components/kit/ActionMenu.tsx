import { isValidElement, type ReactNode } from 'react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

export interface ActionMenuItem {
  id: string
  label: string
  icon?: ReactNode
  onSelect: () => void
  tone?: 'danger'
  description?: string
  disabled?: boolean
}

export interface ActionMenuProps {
  trigger: ReactNode
  items: ActionMenuItem[]
}

function Row({ item }: { item: ActionMenuItem }) {
  return (
    <DropdownMenuItem
      disabled={item.disabled}
      onClick={item.onSelect}
      className={cn(
        'rounded-lg px-2.5 py-2',
        item.tone === 'danger' &&
          'text-tone-danger data-highlighted:bg-tone-danger-soft data-highlighted:text-tone-danger [&_svg]:text-tone-danger',
      )}
    >
      {item.icon}
      <span className="flex min-w-0 flex-col">
        <span>{item.label}</span>
        {item.description && (
          <span className="text-xs text-muted-foreground">
            {item.description}
          </span>
        )}
      </span>
    </DropdownMenuItem>
  )
}

export function ActionMenu({ trigger, items }: ActionMenuProps) {
  const normal = items.filter((i) => i.tone !== 'danger')
  const danger = items.filter((i) => i.tone === 'danger')
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={isValidElement(trigger) ? trigger : <button type="button" />}
      >
        {isValidElement(trigger) ? undefined : trigger}
      </DropdownMenuTrigger>
      <DropdownMenuContent className="min-w-52 rounded-xl p-1.5 shadow-floating">
        {normal.map((i) => (
          <Row key={i.id} item={i} />
        ))}
        {normal.length > 0 && danger.length > 0 && <DropdownMenuSeparator />}
        {danger.map((i) => (
          <Row key={i.id} item={i} />
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
