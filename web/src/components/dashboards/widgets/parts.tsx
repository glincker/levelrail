import type { ReactNode } from 'react'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { widthClass } from '@/components/dashboard/widthClass'
import { TONE, type Tone } from '@/components/kit/tone'

export function Muted({ children }: { children: ReactNode }) {
  return <p className="text-sm text-muted-foreground">{children}</p>
}

export function WidgetSkeleton() {
  return <Skeleton className="h-16 w-full" />
}

export function BigStat({
  value,
  label,
  tone,
}: {
  value: ReactNode
  label: string
  tone?: Tone
}) {
  return (
    <div className="min-w-0">
      <p
        className={cn(
          'text-2xl font-semibold tabular-nums',
          tone && TONE[tone].text,
        )}
      >
        {value}
      </p>
      <p className="text-xs text-muted-foreground">{label}</p>
    </div>
  )
}

/** Thin meter; the caller always prints the number beside it. */
export function Meter({
  percent,
  label,
  tone = 'info',
}: {
  percent: number
  label: string
  tone?: Tone
}) {
  return (
    <div
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(percent)}
      className="h-1.5 w-full overflow-hidden rounded-full bg-muted"
    >
      <div
        className={cn(
          'h-full rounded-full',
          TONE[tone].solid,
          widthClass(percent),
        )}
      />
    </div>
  )
}
