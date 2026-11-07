import { cn } from '@/lib/utils'

export function NavBadge({
  count,
  tone,
  label,
}: {
  count: number
  tone: 'danger' | 'warning'
  label: string
}) {
  if (count <= 0) return null
  return (
    <span
      aria-label={label}
      className={cn(
        'ml-auto flex h-5 min-w-5 shrink-0 transition-[filter] duration-150 group-hover/menu-button:brightness-110 group-hover/header:brightness-110 motion-reduce:transition-none items-center justify-center rounded-full px-1.5 text-xs font-semibold tabular-nums group-data-[collapsible=icon]:hidden',
        tone === 'danger'
          ? 'bg-destructive text-white'
          : 'border border-tone-warning-border bg-tone-warning-soft text-tone-warning',
      )}
    >
      {count > 99 ? '99+' : count}
    </span>
  )
}
