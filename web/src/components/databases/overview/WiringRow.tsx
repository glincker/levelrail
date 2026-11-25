import type { ReactElement, ReactNode } from 'react'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { DOT, type WireTone } from '../../pages/tlsDot'

const LINE: Record<WireTone, string> = {
  ok: 'bg-tone-success-solid',
  warn: 'bg-tone-warning-solid',
  bad: 'bg-tone-danger-solid',
  off: 'bg-muted-foreground/35',
}
const INK: Record<WireTone, string> = {
  ok: 'text-tone-success',
  warn: 'text-tone-warning',
  bad: 'text-tone-danger',
  off: 'text-muted-foreground/50',
}

const ARROW =
  'absolute size-0 border-y-4 border-y-transparent border-l-[6px] border-l-current'

/** Edge into a node; glows and pulses while the owning group/row is hovered or focused. */
export function Edge({
  tone,
  side,
}: Readonly<{ tone: WireTone; side: 'left' | 'right' }>) {
  const left = side === 'left'
  return (
    <span aria-hidden="true" className="hidden @xl:block">
      <span
        className={cn(
          'absolute top-1/2 h-px w-5 -translate-y-1/2 transition-all duration-150 group-focus-within/row:h-0.5 group-hover/row:h-0.5 group-hover/row:animate-pulse motion-reduce:animate-none motion-reduce:transition-none',
          left ? '-left-5' : '-right-5',
          LINE[tone],
        )}
      />
      <span
        className={cn(
          ARROW,
          'top-1/2 -translate-y-1/2',
          left ? '-left-1' : '-right-px',
          INK[tone],
        )}
      />
    </span>
  )
}

/** Vertical spine piece joining the right-hand rows; first and last rows stop at their centre line. */
export function Spine() {
  return (
    <span
      aria-hidden="true"
      className="absolute -top-1 -bottom-1 -left-5 hidden w-px bg-muted-foreground/35 group-first/row:top-1/2 group-last/row:bottom-1/2 group-only/row:hidden @xl:block"
    />
  )
}

export function StatusDot({ tone }: Readonly<{ tone: WireTone }>) {
  return (
    <span
      aria-hidden="true"
      className={cn('size-2 shrink-0 rounded-full', DOT[tone])}
    />
  )
}

/** One node card: a main control (link or button) with a hover tooltip card, plus an optional CTA beside it. */
export function WireCard({
  control,
  icon,
  label,
  value,
  sub,
  tone,
  hint,
  next,
  cta,
  children,
}: Readonly<{
  control: ReactElement
  icon: ReactNode
  label: string
  value: string
  sub?: string
  tone: WireTone
  hint: string
  next: string
  cta?: ReactNode
  children?: ReactNode
}>) {
  return (
    <div className="rounded-lg border border-border bg-card/60 transition-colors duration-150 focus-within:border-ring/60 hover:border-ring/40 hover:bg-card motion-reduce:transition-none">
      <div className="flex items-center gap-2 px-2.5 py-2">
        <Tooltip>
          <TooltipTrigger
            render={control}
            className="flex min-w-0 flex-1 items-center gap-2 rounded-md text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <span className="text-muted-foreground [&_svg]:size-4">{icon}</span>
            <span className="min-w-0 flex-1">
              <span className="block text-xs leading-4 text-muted-foreground">
                {label}
              </span>
              <span className="block truncate text-sm leading-5 font-medium">
                {value}
              </span>
              {sub ? (
                <span className="block truncate text-xs leading-4 text-muted-foreground">
                  {sub}
                </span>
              ) : null}
            </span>
            <StatusDot tone={tone} />
          </TooltipTrigger>
          <TooltipContent variant="card" side="top" className="block">
            <span className="block font-medium">{label}</span>
            <span className="block text-muted-foreground">{hint}</span>
            <span className="mt-1 block">{next}</span>
          </TooltipContent>
        </Tooltip>
        {cta}
      </div>
      {children}
    </div>
  )
}
