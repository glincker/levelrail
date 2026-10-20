import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface RankedBarItem {
  id: string
  label: string
  /** Muted second line, for example a region label. */
  detail?: string
  /** 0 to 1 share of the whole. */
  share: number
  valueLabel: ReactNode
}

/** RankedBarList is a ranked list where each row carries a share bar. */
export function RankedBarList({
  items,
  ariaLabel,
  className,
}: Readonly<{
  items: RankedBarItem[]
  ariaLabel: string
  className?: string
}>) {
  return (
    <ul
      aria-label={ariaLabel}
      className={cn('divide-y divide-border', className)}
    >
      {items.map((item) => {
        const pct = Math.max(0, Math.min(1, item.share)) * 100
        return (
          <li key={item.id} className="space-y-1.5 py-3 first:pt-0 last:pb-0">
            <div className="flex items-baseline justify-between gap-3">
              <span className="min-w-0 truncate text-sm" title={item.label}>
                {item.label}
                {item.detail ? (
                  <span className="ml-2 text-xs text-muted-foreground">
                    {item.detail}
                  </span>
                ) : null}
              </span>
              <span className="shrink-0 text-sm tabular-nums text-muted-foreground">
                {item.valueLabel}
              </span>
            </div>
            <svg
              viewBox="0 0 100 4"
              preserveAspectRatio="none"
              className="block h-1 w-full"
              aria-hidden="true"
            >
              <rect width={100} height={4} rx={2} className="fill-muted" />
              <rect
                width={pct}
                height={4}
                rx={2}
                className="fill-tone-accent-solid"
              />
            </svg>
          </li>
        )
      })}
    </ul>
  )
}
