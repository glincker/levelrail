import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { TONE } from '../kit/tone'
import type { Tone } from '../kit/tone'

// Same visual language as the setup wizard's capacity readouts.
export function Readout({
  label,
  value,
  hint,
  tone = 'neutral',
}: {
  label: string
  value: ReactNode
  hint?: ReactNode
  tone?: Tone
}) {
  return (
    <div className="relative overflow-hidden rounded-xl border border-border bg-card p-3">
      <span
        className={cn('absolute inset-x-0 top-0 h-0.5', TONE[tone].solid)}
        aria-hidden="true"
      />
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="mt-1 text-lg font-semibold tabular-nums text-foreground">
        {value}
      </dd>
      {hint ? (
        <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>
      ) : null}
    </div>
  )
}
