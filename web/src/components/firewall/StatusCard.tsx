import type { ReactNode } from 'react'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { TONE, type Tone } from '@/components/kit/tone'
import { cn } from '@/lib/utils'

export interface StatusRow {
  label: string
  value: ReactNode
}

/** StatusCard is the at-a-glance state of a protection: an icon, one sentence and a few label/value rows. */
export function StatusCard({
  tone,
  icon,
  title,
  subtitle,
  rows,
  className,
}: {
  tone: Tone
  icon: ReactNode
  title: string
  subtitle: string
  rows: StatusRow[]
  className?: string
}) {
  return (
    <Card className={cn('justify-between', className)}>
      <div className="flex flex-col items-center gap-2 px-(--card-spacing) text-center">
        <span
          aria-hidden="true"
          className={cn(
            'flex size-12 items-center justify-center rounded-xl [&_svg]:size-7',
            TONE[tone].soft,
            TONE[tone].text,
          )}
        >
          {icon}
        </span>
        <p className="text-sm font-semibold text-foreground">{title}</p>
        <p className="text-xs text-muted-foreground">{subtitle}</p>
      </div>
      <dl className="divide-y divide-border px-(--card-spacing)">
        {rows.map((r) => (
          <div
            key={r.label}
            className="flex items-center justify-between gap-3 py-2 text-sm"
          >
            <dt className="text-muted-foreground">{r.label}</dt>
            <dd className="font-medium text-foreground tabular-nums">
              {r.value}
            </dd>
          </div>
        ))}
      </dl>
    </Card>
  )
}

/** Same footprint as StatusCard so the page does not jump when data arrives. */
export function StatusCardSkeleton({ rows = 3 }: { rows?: number }) {
  return (
    <Card aria-hidden="true" className="justify-between">
      <div className="flex flex-col items-center gap-2 px-(--card-spacing)">
        <Skeleton className="size-12 rounded-xl" />
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-3 w-24" />
      </div>
      <div className="divide-y divide-border px-(--card-spacing)">
        {Array.from({ length: rows }, (_, i) => (
          <div key={i} className="flex items-center justify-between py-2">
            <Skeleton className="h-4 w-24" />
            <Skeleton className="h-4 w-12" />
          </div>
        ))}
      </div>
    </Card>
  )
}
