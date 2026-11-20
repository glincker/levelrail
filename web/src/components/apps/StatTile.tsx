import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { StatusShape, type StatusState } from '@/components/kit'

/** Compact label, value and optional status dot; one primitive for every summary strip. */
export function StatTile({
  label,
  value,
  state,
  hint,
  className,
}: Readonly<{
  label: string
  value: ReactNode
  state?: StatusState
  hint?: ReactNode
  className?: string
}>) {
  return (
    <div
      className={cn(
        'min-w-0 rounded-lg border border-border bg-card p-3',
        className,
      )}
    >
      <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
        {state ? <StatusShape state={state} /> : null}
        <span className="truncate">{label}</span>
      </p>
      <p className="mt-1 truncate text-xl font-semibold tabular-nums">
        {value}
      </p>
      {hint ? (
        <p className="truncate text-xs text-muted-foreground">{hint}</p>
      ) : null}
    </div>
  )
}
