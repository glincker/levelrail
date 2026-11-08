import { cn } from '@/lib/utils'
import { STATUS, TONE, type StatusState } from './tone'

/** StatusShape draws the state glyph; always aria-hidden because a label sits beside it. */
export function StatusShape({
  state,
  className,
}: Readonly<{ state: StatusState; className?: string }>) {
  const { tone, shape, live } = STATUS[state]
  const t = TONE[tone]
  const stroke = 'fill-none stroke-current stroke-[1.5]'
  return (
    <span
      aria-hidden="true"
      data-shape={shape}
      className={cn(
        'relative inline-flex size-2.5 shrink-0',
        t.text,
        className,
      )}
    >
      {live && (
        <span
          data-testid="status-ping"
          className={cn(
            'status-ping absolute inset-0 rounded-full motion-reduce:hidden',
            t.solid,
          )}
        />
      )}
      <svg viewBox="0 0 10 10" className="relative size-full">
        {shape === 'circle' && (
          <circle cx="5" cy="5" r="4" className="fill-current" />
        )}
        {shape === 'ring' && (
          <circle cx="5" cy="5" r="3.5" className={stroke} />
        )}
        {shape === 'dashed' && (
          <circle
            cx="5"
            cy="5"
            r="3.5"
            strokeDasharray="2 1.6"
            className={stroke}
          />
        )}
        {shape === 'triangle' && (
          <path d="M5 1 9 9H1Z" className="fill-current" />
        )}
        {shape === 'diamond' && (
          <path d="M5 .5 9.5 5 5 9.5.5 5Z" className="fill-current" />
        )}
      </svg>
    </span>
  )
}
