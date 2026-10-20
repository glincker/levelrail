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
        'ml-auto flex h-5 min-w-5 items-center justify-center rounded-full px-1.5 text-[10px] font-semibold tabular-nums text-white group-data-[collapsible=icon]:hidden',
        tone === 'danger' ? 'bg-destructive' : 'bg-amber-500',
      )}
    >
      {count > 99 ? '99+' : count}
    </span>
  )
}
