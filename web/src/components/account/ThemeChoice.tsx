import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

export type ThemeValue = 'system' | 'light' | 'dark'
export type ThemeChoiceValue = ThemeValue | 'oled'

const CHOICES: readonly {
  value: ThemeChoiceValue
  key:
    | 'preferences.themeBrowser'
    | 'preferences.themeLight'
    | 'preferences.themeDark'
    | 'preferences.themeOled'
  swatch: string
}[] = [
  {
    value: 'system',
    key: 'preferences.themeBrowser',
    swatch: 'bg-linear-to-r from-neutral-100 from-50% to-neutral-900 to-50%',
  },
  { value: 'light', key: 'preferences.themeLight', swatch: 'bg-neutral-100' },
  { value: 'dark', key: 'preferences.themeDark', swatch: 'bg-neutral-900' },
  { value: 'oled', key: 'preferences.themeOled', swatch: 'bg-black' },
]

/** ThemeChoice is a three-option radio group with a live swatch per theme. */
export function ThemeChoice<V extends ThemeChoiceValue = ThemeValue>({
  value,
  onChange,
  labelledBy,
  includeOled = false,
}: {
  value: V
  onChange: (next: V) => void
  labelledBy: string
  /** OLED is a local display mode, so only surfaces that do not sync the choice to the server offer it. */
  includeOled?: boolean
}) {
  const { t } = useTranslation('account')
  const choices = includeOled
    ? CHOICES
    : CHOICES.filter((c) => c.value !== 'oled')
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
    const next = (index + step + choices.length) % choices.length
    onChange((choices[next]?.value as V | undefined) ?? value)
    refs.current[next]?.focus()
  }

  return (
    <div
      role="radiogroup"
      aria-labelledby={labelledBy}
      className={cn(
        'grid gap-1',
        includeOled
          ? 'grid-cols-[1.7fr_1fr_1fr_1.2fr]'
          : 'grid-cols-[1.7fr_1fr_1fr]',
        'rounded-lg border border-input p-1',
      )}
    >
      {choices.map((c, i) => {
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
            onClick={() => onChange(c.value as V)}
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
