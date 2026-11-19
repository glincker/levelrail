import { useRef, type KeyboardEvent } from 'react'
import { BrandIcon, hasMonoVariant } from '@/components/BrandIcon'
import { brandForCIProvider } from '@/lib/ciProviderBrand'
import { cn } from '@/lib/utils'

export interface CIProviderOption {
  id: string
  label: string
}

interface CIProviderPickerProps {
  options: readonly CIProviderOption[]
  value: string
  onChange: (id: string) => void
  label: string
}

/** Segmented radio group of CI providers with their brand marks; arrow keys move and select. */
export function CIProviderPicker({
  options,
  value,
  onChange,
  label,
}: CIProviderPickerProps) {
  const refs = useRef<(HTMLButtonElement | null)[]>([])

  function move(index: number) {
    const next = (index + options.length) % options.length
    const opt = options[next]
    if (!opt) return
    onChange(opt.id)
    refs.current[next]?.focus()
  }

  function onKeyDown(e: KeyboardEvent, index: number) {
    const target: Record<string, number> = {
      ArrowRight: index + 1,
      ArrowDown: index + 1,
      ArrowLeft: index - 1,
      ArrowUp: index - 1,
      Home: 0,
      End: options.length - 1,
    }
    const to = target[e.key]
    if (to === undefined) return
    e.preventDefault()
    move(to)
  }

  return (
    <div
      role="radiogroup"
      aria-label={label}
      className="flex flex-wrap gap-1.5 rounded-lg bg-muted p-1"
    >
      {options.map((opt, i) => {
        const checked = opt.id === value
        const brand = brandForCIProvider(opt.id)
        return (
          <button
            key={opt.id}
            ref={(el) => {
              refs.current[i] = el
            }}
            type="button"
            role="radio"
            aria-checked={checked}
            tabIndex={checked ? 0 : -1}
            onClick={() => {
              onChange(opt.id)
            }}
            onKeyDown={(e) => {
              onKeyDown(e, i)
            }}
            className={cn(
              'inline-flex min-h-11 flex-1 items-center justify-center gap-2 rounded-md border px-3 text-sm font-medium whitespace-nowrap transition-colors duration-150 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none active:scale-[0.97] motion-reduce:transition-none motion-reduce:active:scale-100',
              checked
                ? 'border-border bg-card text-foreground shadow-sm'
                : 'border-transparent text-muted-foreground hover:text-foreground',
            )}
          >
            {brand ? (
              <BrandIcon
                name={brand}
                mono={hasMonoVariant(brand)}
                className="size-5 shrink-0"
              />
            ) : null}
            {opt.label}
          </button>
        )
      })}
    </div>
  )
}
