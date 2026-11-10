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
  tone: keyof typeof TONES | 'neutral'
}) {
  const text = count > 99 ? '99+' : count > 9 ? '9+' : count
  if (tone === 'neutral') {
    // Inline muted text inside the control, so it can never be clipped.
    return (
      <span
        aria-hidden="true"
        data-slot="count-badge"
        className="text-xs leading-none font-medium text-muted-foreground tabular-nums"
      >
        {text}
      </span>
    )
  }
  return (
    <span
      aria-hidden="true"
      data-slot="count-badge"
      className={cn(
        'pointer-events-none absolute -top-1.5 -right-1.5 flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-xs leading-none font-semibold tabular-nums',
        TONES[tone],
      )}
    >
      {text}
    </span>
  )
}
