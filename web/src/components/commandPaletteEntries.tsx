import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import { fuzzyMatchIndices } from '@/lib/fuzzy'
import { spanIndices } from '@/lib/paletteGroups'
import type { SearchSpan } from '@/queries/search'
import { chipLabel, type Chip } from '@/lib/paletteQuery'
import type { PaletteItem } from './commandPaletteData'

function Highlighted({
  text,
  query,
  spans,
}: {
  text: string
  query: string
  spans?: SearchSpan[]
}) {
  const hit = spans
    ? spanIndices(spans)
    : new Set(fuzzyMatchIndices(query, text))
  if (hit.size === 0) return <>{text}</>
  const runs: { text: string; hit: boolean }[] = []
  Array.from(text).forEach((ch, i) => {
    const h = hit.has(i)
    const last = runs[runs.length - 1]
    if (last && last.hit === h) last.text += ch
    else runs.push({ text: ch, hit: h })
  })
  return (
    <>
      {runs.map((r, i) =>
        r.hit ? (
          <mark key={i} className="bg-transparent font-semibold text-primary">
            {r.text}
          </mark>
        ) : (
          r.text
        ),
      )}
    </>
  )
}

export function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="rounded border border-border bg-background px-1 font-mono text-[10px] leading-4 text-muted-foreground">
      {children}
    </kbd>
  )
}

export function ResultRow({
  item,
  optionId,
  active,
  query = '',
  onSelect,
}: {
  item: PaletteItem
  query?: string
  optionId: string
  active: boolean
  onSelect: () => void
}) {
  const ref = React.useRef<HTMLDivElement>(null)
  React.useEffect(() => {
    if (active) ref.current?.scrollIntoView?.({ block: 'nearest' })
  }, [active])

  return (
    <div
      ref={ref}
      id={optionId}
      role="option"
      aria-selected={active}
      data-active={active}
      tabIndex={-1}
      onClick={onSelect}
      className={cn(
        'flex w-full cursor-pointer items-center gap-3 rounded-xl px-3 py-2 text-left text-[15px] outline-none',
        active
          ? 'bg-foreground/10 text-foreground'
          : 'text-foreground/90 hover:bg-foreground/5',
      )}
    >
      <span className="flex size-6 shrink-0 items-center justify-center text-muted-foreground [&_svg]:size-6">
        {item.icon}
      </span>
      <span
        className={cn(
          'min-w-0',
          item.stacked ? 'flex flex-1 flex-col' : 'truncate',
        )}
      >
        <span className="truncate">
          <Highlighted
            text={item.label}
            query={query}
            spans={item.highlightSpans}
          />
        </span>
        {item.subtitle ? (
          <span
            className={cn(
              'truncate text-xs text-muted-foreground',
              item.stacked ? '' : 'ml-2',
            )}
          >
            {item.subtitle}
          </span>
        ) : null}
      </span>
      {item.locked ? (
        <LockGlassIcon
          className="size-5 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
      ) : null}
      {item.badge ? (
        <span className="ml-auto shrink-0">{item.badge}</span>
      ) : null}
      {item.onRemove ? (
        <button
          type="button"
          tabIndex={-1}
          aria-label={item.removeLabel}
          onClick={(e) => {
            e.stopPropagation()
            item.onRemove?.()
          }}
          className={cn(
            'shrink-0 rounded p-0.5 text-muted-foreground hover:text-foreground',
            item.badge ? '' : 'ml-auto',
          )}
        >
          <XGlassIcon className="size-5" aria-hidden="true" />
        </button>
      ) : null}
      {active ? (
        <span
          className={cn(
            'flex items-center gap-1',
            item.badge || item.onRemove ? 'ml-2' : 'ml-auto',
          )}
          aria-hidden="true"
        >
          {item.secondaryHint ? (
            <span className="flex items-center gap-1 text-[10px] text-muted-foreground">
              <Kbd>Tab</Kbd>
              {item.secondaryHint}
            </span>
          ) : null}
          <Kbd>Enter</Kbd>
        </span>
      ) : item.hint ? (
        <span className="ml-auto flex gap-0.5" aria-hidden="true">
          {item.hint.map((k) => (
            <Kbd key={k}>{k}</Kbd>
          ))}
        </span>
      ) : null}
    </div>
  )
}

export function ChipBar({
  chips,
  onRemove,
}: {
  chips: readonly Chip[]
  onRemove: (chip: Chip) => void
}) {
  const { t } = useTranslation('palette')
  if (chips.length === 0) return null
  return (
    <ul
      aria-label={t('filtersLabel')}
      className="flex shrink-0 flex-wrap items-center gap-1"
    >
      {chips.map((chip) => (
        <li key={chipLabel(chip)}>
          <button
            type="button"
            aria-label={t('removeChip', { chip: chipLabel(chip) })}
            onClick={() => onRemove(chip)}
            className="flex items-center gap-1 rounded-full border border-border bg-muted px-2 py-0.5 text-xs text-foreground hover:bg-muted/60"
          >
            {chipLabel(chip)}
            <XGlassIcon className="size-4.5" aria-hidden="true" />
          </button>
        </li>
      ))}
    </ul>
  )
}

export function PaletteFooter({ children }: { children?: React.ReactNode }) {
  const { t } = useTranslation('palette')
  return (
    <div className="flex items-center gap-3 border-t border-border px-3 py-2 text-xs text-muted-foreground">
      <span aria-hidden="true" className="flex items-center gap-1">
        <Kbd>Up</Kbd>
        <Kbd>Down</Kbd>
        {t('footer.navigate')}
      </span>
      <span aria-hidden="true" className="flex items-center gap-1">
        <Kbd>Enter</Kbd>
        {t('footer.select')}
      </span>
      <span aria-hidden="true" className="flex items-center gap-1">
        <Kbd>Tab</Kbd>
        {t('footer.secondary')}
      </span>
      <span aria-hidden="true" className="flex items-center gap-1">
        <Kbd>Esc</Kbd>
        {t('footer.close')}
      </span>
      {children ? <span className="ml-auto">{children}</span> : null}
    </div>
  )
}
