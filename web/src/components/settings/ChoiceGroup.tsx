import { useRef, type KeyboardEvent, type ReactNode } from 'react'
import { Badge } from '@/components/ui/badge'
import {
  SelectedMark,
  selectableCardClass,
} from '@/components/ui/selectable-card'
import { cn } from '@/lib/utils'

export interface Choice<T extends string> {
  value: T
  label: string
  hint?: string
  /** Decorative sample, shown beside the label (above it when block). */
  sample?: ReactNode
  /** The sample is a full-width preview. */
  block?: boolean
  isDefault?: boolean
  isRecommended?: boolean
}

interface ChoiceGroupProps<T extends string> {
  label: string
  value: T
  options: readonly Choice<T>[]
  onChange: (next: T) => void
  defaultLabel: string
  recommendedLabel: string
  /** Narrower tiles, for a group that shares a row with another. */
  compact?: boolean
  className?: string
}

/** ChoiceGroup is a roving-tabindex radio group of selectable cards with Default and Recommended badges. */
export function ChoiceGroup<T extends string>({
  label,
  value,
  options,
  onChange,
  defaultLabel,
  recommendedLabel,
  compact,
  className,
}: ChoiceGroupProps<T>) {
  const refs = useRef<(HTMLButtonElement | null)[]>([])

  const onKeyDown = (e: KeyboardEvent, index: number) => {
    const step =
      e.key === 'ArrowRight' || e.key === 'ArrowDown'
        ? 1
        : e.key === 'ArrowLeft' || e.key === 'ArrowUp'
          ? -1
          : 0
    if (step === 0) return
    e.preventDefault()
    const next = (index + step + options.length) % options.length
    const target = options[next]
    if (target) onChange(target.value)
    refs.current[next]?.focus()
  }

  return (
    <div
      role="radiogroup"
      aria-label={label}
      className={cn(
        'grid gap-2',
        compact
          ? 'grid-cols-[repeat(auto-fit,minmax(7.5rem,1fr))]'
          : 'grid-cols-[repeat(auto-fit,minmax(9.5rem,1fr))]',
        className,
      )}
    >
      {options.map((o, i) => {
        const selected = o.value === value
        return (
          <button
            key={o.value}
            ref={(el) => {
              refs.current[i] = el
            }}
            type="button"
            role="radio"
            aria-checked={selected}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(o.value)}
            onKeyDown={(e) => onKeyDown(e, i)}
            className={selectableCardClass(
              selected,
              'flex min-w-0 flex-col gap-1.5 rounded-lg px-2.5 py-2 text-left',
            )}
          >
            <SelectedMark selected={selected} />
            {o.block ? o.sample : null}
            <span className="flex min-w-0 items-start gap-2">
              {o.block ? null : o.sample ? (
                <span className="shrink-0">{o.sample}</span>
              ) : null}
              <span className="min-w-0 flex-1">
                <span className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 pr-3 text-sm font-medium">
                  {o.label}
                  {o.isDefault || o.isRecommended ? (
                    <span className="flex min-w-0 flex-wrap gap-1">
                      {o.isDefault ? (
                        <Badge
                          variant="muted"
                          className="px-1 py-0 font-normal"
                        >
                          {defaultLabel}
                        </Badge>
                      ) : null}
                      {o.isRecommended ? (
                        <Badge
                          variant="accent"
                          className="px-1 py-0 font-normal"
                        >
                          {recommendedLabel}
                        </Badge>
                      ) : null}
                    </span>
                  ) : null}
                </span>
                {o.hint ? (
                  <span
                    title={o.hint}
                    className="block truncate text-xs text-muted-foreground"
                  >
                    {o.hint}
                  </span>
                ) : null}
              </span>
            </span>
          </button>
        )
      })}
    </div>
  )
}
