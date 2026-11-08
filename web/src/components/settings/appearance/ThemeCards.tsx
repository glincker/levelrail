import { useRef, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  SelectedMark,
  selectableCardClass,
} from '@/components/ui/selectable-card'
import type { Theme } from '@/components/ThemeProvider'
import { cn } from '@/lib/utils'

type Tone = 'light' | 'dark' | 'oled'

// Static samples mirror the base blocks: page, sidebar, border, text line.
const TONE: Record<Tone, string> = {
  light:
    'bg-[oklch(1_0_0)] [--s:oklch(0.97_0_0)] [--b:oklch(0.922_0_0)] [--l:oklch(0.708_0_0)]',
  dark: 'bg-[oklch(0.145_0_0)] [--s:oklch(0.205_0_0)] [--b:oklch(0.3_0_0)] [--l:oklch(0.55_0_0)]',
  oled: 'bg-[oklch(0_0_0)] [--s:oklch(0.15_0_0)] [--b:oklch(0.32_0_0)] [--l:oklch(0.5_0_0)]',
}

function Mini({ tone, half }: { tone: Tone; half?: 'left' | 'right' }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        'flex h-full min-w-0 flex-1 gap-1.5 p-1.5',
        TONE[tone],
        half === 'right' && 'justify-end',
      )}
    >
      {half === 'right' ? null : (
        <span className="w-1/4 rounded-sm border border-(--b) bg-(--s)" />
      )}
      <span className="flex flex-1 flex-col gap-1">
        <span className="h-1.5 w-2/3 rounded-full bg-(--l)" />
        <span className="h-1.5 w-1/2 rounded-full bg-(--l) opacity-60" />
        <span className="mt-auto h-2.5 w-1/3 rounded-full bg-state-accent" />
      </span>
    </span>
  )
}

function Thumb({ value }: { value: Theme }) {
  return (
    <span
      aria-hidden="true"
      className="flex h-16 w-full overflow-hidden rounded-lg border border-border"
    >
      {value === 'system' ? (
        <>
          <Mini tone="light" half="left" />
          <Mini tone="dark" half="right" />
        </>
      ) : (
        <Mini tone={value} />
      )}
    </span>
  )
}

const ORDER: readonly Theme[] = ['system', 'light', 'dark', 'oled']

/** ThemeCards is a roving-tabindex radio group of four visual theme choices. */
export function ThemeCards({
  value,
  onChange,
  label,
}: {
  value: Theme
  onChange: (next: Theme) => void
  label: string
}) {
  const { t } = useTranslation('settings')
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
    const next = (index + step + ORDER.length) % ORDER.length
    const target = ORDER[next]
    if (target) onChange(target)
    refs.current[next]?.focus()
  }

  return (
    <div
      role="radiogroup"
      aria-label={label}
      className="grid grid-cols-2 gap-2 sm:grid-cols-4"
    >
      {ORDER.map((v, i) => {
        const selected = v === value
        return (
          <button
            key={v}
            ref={(el) => {
              refs.current[i] = el
            }}
            type="button"
            role="radio"
            aria-checked={selected}
            tabIndex={selected ? 0 : -1}
            onClick={() => onChange(v)}
            onKeyDown={(e) => onKeyDown(e, i)}
            className={cn(
              selectableCardClass(
                selected,
                'flex min-w-0 flex-col gap-2 rounded-xl p-2 text-left forced-colors:border-[ButtonText]',
              ),
              selected &&
                'ring-1 ring-state-accent forced-colors:ring-[Highlight]',
            )}
          >
            <SelectedMark selected={selected} />
            <Thumb value={v} />
            <span className="px-0.5">
              <span className="block text-sm font-medium">
                {t(`appearancePage.hub.theme.${v}`)}
              </span>
              <span className="block text-xs text-muted-foreground">
                {t(`appearancePage.hub.theme.${v}Hint`)}
              </span>
            </span>
          </button>
        )
      })}
    </div>
  )
}
