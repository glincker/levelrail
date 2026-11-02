import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

export type ThemeValue = 'system' | 'light' | 'dark'

const CHOICES: readonly {
  value: ThemeValue
  key:
    | 'preferences.themeBrowser'
    | 'preferences.themeLight'
    | 'preferences.themeDark'
  swatch: string
}[] = [
  {
    value: 'system',
    key: 'preferences.themeBrowser',
    swatch: 'bg-linear-to-r from-neutral-100 from-50% to-neutral-900 to-50%',
  },
  { value: 'light', key: 'preferences.themeLight', swatch: 'bg-neutral-100' },
  { value: 'dark', key: 'preferences.themeDark', swatch: 'bg-neutral-900' },
]

/** ThemeChoice is a three-option radio group with a live swatch per theme. */
export function ThemeChoice({
  value,
  onChange,
  labelledBy,
}: {
  value: ThemeValue
  onChange: (next: ThemeValue) => void
  labelledBy: string
}) {
  const { t } = useTranslation('account')
  const refs = useRef<(HTMLButtonElement | null)[]>([])

  const onKeyDown = (e: React.KeyboardEvent, index: number) => {
    const step =
      e.key === 'ArrowRight' || e.key === 'ArrowDown'
        ? 1
        : e.key === 'ArrowLeft' || e.key === 'ArrowUp'
          ? -1
          : 0
    if (step === 0) return
    e.preventDefault()
    const next = (index + step + CHOICES.length) % CHOICES.length
    onChange(CHOICES[next]?.value ?? value)
    refs.current[next]?.focus()
  }

  return (
    <div
      role="radiogroup"
      aria-labelledby={labelledBy}
      className="grid grid-cols-[1.7fr_1fr_1fr] gap-1 rounded-lg border border-input p-1"
    >
      {CHOICES.map((c, i) => {
        const active = c.value === value
        return (
          <button
            key={c.value}
            ref={(el) => {
              refs.current[i] = el
            }}
            type="button"
            role="radio"
            aria-checked={active}
            tabIndex={active ? 0 : -1}
            onClick={() => onChange(c.value)}
            onKeyDown={(e) => onKeyDown(e, i)}
            className={cn(
              'flex h-8 items-center justify-center gap-1.5 rounded-md px-1.5 text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring motion-reduce:transition-none',
              active
                ? 'bg-muted font-medium text-foreground ring-1 ring-foreground/20'
                : 'text-muted-foreground hover:bg-muted/60',
            )}
          >
            <span
              aria-hidden="true"
              className={cn(
                'size-3.5 shrink-0 rounded-full ring-1 ring-foreground/30',
                c.swatch,
              )}
            />
            <span className="truncate">{t(c.key)}</span>
          </button>
        )
      })}
    </div>
  )
}
