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
  /** Decorative sample shown above the label. */
  sample?: ReactNode
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
      className={cn('grid grid-cols-2 gap-2 sm:grid-cols-4', className)}
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
              'flex min-w-0 flex-col items-start gap-1.5 rounded-lg p-2 text-left',
            )}
          >
            <SelectedMark selected={selected} />
            {o.sample}
            <span className="text-sm font-medium">{o.label}</span>
            {o.hint ? (
              <span className="text-xs text-muted-foreground">{o.hint}</span>
            ) : null}
            {o.isDefault || o.isRecommended ? (
              <span className="flex flex-wrap gap-1">
                {o.isDefault ? (
                  <Badge variant="muted">{defaultLabel}</Badge>
                ) : null}
                {o.isRecommended ? (
                  <Badge variant="accent">{recommendedLabel}</Badge>
                ) : null}
              </span>
            ) : null}
          </button>
        )
      })}
    </div>
  )
}
