import { cn } from '@/lib/utils'

const TONES = {
  destructive: 'bg-destructive text-white',
  primary: 'bg-primary text-primary-foreground',
} as const

// Pill grows with its text so "9+" and "99+" never clip; the parent button names the count.
export function CountBadge({
  count,
  tone,
}: {
  count: number
  tone: keyof typeof TONES
}) {
  return (
    <span
      aria-hidden="true"
      data-slot="count-badge"
      className={cn(
        'pointer-events-none absolute -top-1.5 -right-1.5 flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-xs leading-none font-semibold tabular-nums',
        TONES[tone],
      )}
    >
      {count > 99 ? '99+' : count > 9 ? '9+' : count}
    </span>
  )
}
