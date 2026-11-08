import { useId } from 'react'

import { cn } from '@/lib/utils'

export interface SegmentedOption<T extends string> {
  value: T
  label: string
}

interface SegmentedControlProps<T extends string> {
  value: T
  onValueChange: (value: T) => void
  options: readonly SegmentedOption<T>[]
  'aria-label': string
  className?: string
}

const COLS = [
  '',
  '',
  'grid-cols-2',
  'grid-cols-3',
  'grid-cols-4',
  'grid-cols-5',
]
const THUMB = [
  '',
  '',
  'w-[calc((100%-4px)/2)]',
  'w-[calc((100%-4px)/3)]',
  'w-[calc((100%-4px)/4)]',
  'w-[calc((100%-4px)/5)]',
]
const SLIDE = [
  'translate-x-0',
  'translate-x-full',
  'translate-x-[200%]',
  'translate-x-[300%]',
  'translate-x-[400%]',
]

// Two to five segments; the thumb slides with transform only.
export function SegmentedControl<T extends string>({
  value,
  onValueChange,
  options,
  className,
  'aria-label': ariaLabel,
}: SegmentedControlProps<T>) {
  const name = useId()
  const index = Math.max(
    0,
    options.findIndex((o) => o.value === value),
  )
  const count = Math.min(5, Math.max(2, options.length))
  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      data-slot="segmented-control"
      className={cn(
        'relative inline-grid h-8 items-stretch rounded-full bg-muted p-0.5',
        COLS[count],
        className,
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          'pointer-events-none absolute inset-y-0.5 left-0.5 rounded-full bg-background shadow-[var(--glinr-elev-1)] transition-transform duration-200 motion-reduce:transition-none',
          THUMB[count],
          SLIDE[index],
        )}
      />
      {options.map((o) => (
        <label
          key={o.value}
          className={cn(
            'relative z-10 flex cursor-pointer items-center justify-center rounded-full px-3 text-sm font-medium transition-colors has-focus-visible:outline-2 has-focus-visible:outline-offset-1 has-focus-visible:outline-ring',
            o.value === value ? 'text-foreground' : 'text-muted-foreground',
          )}
        >
          <input
            type="radio"
            name={name}
            value={o.value}
            checked={o.value === value}
            onChange={() => onValueChange(o.value)}
            className="sr-only"
          />
          {o.label}
        </label>
      ))}
    </div>
  )
}
