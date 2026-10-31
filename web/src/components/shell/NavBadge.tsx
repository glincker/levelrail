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
        'ml-auto flex h-5 min-w-5 items-center justify-center rounded-full px-1.5 text-[11px] font-semibold tabular-nums group-data-[collapsible=icon]:hidden',
        tone === 'danger'
          ? 'bg-destructive text-white'
          : 'border border-tone-warning-border bg-tone-warning-soft text-tone-warning',
      )}
    >
      {count > 99 ? '99+' : count}
    </span>
  )
}
