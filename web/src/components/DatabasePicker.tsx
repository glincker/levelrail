import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { DatabaseGlassIcon, SearchGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  DATABASE_FAMILIES,
  type DatabaseFamily,
  type PickerEntry,
} from '@/lib/databaseCatalog'
import { BrandIcon } from './BrandIcon'

type FamilyFilter = 'all' | DatabaseFamily

// Explicit maps keep every translation key a literal the typed t() checks.
const TAGLINE_KEYS = {
  postgres: 'picker.taglines.postgres',
  pgvector: 'picker.taglines.pgvector',
  timescaledb: 'picker.taglines.timescaledb',
  postgis: 'picker.taglines.postgis',
  mysql: 'picker.taglines.mysql',
  mariadb: 'picker.taglines.mariadb',
  redis: 'picker.taglines.redis',
  valkey: 'picker.taglines.valkey',
  keydb: 'picker.taglines.keydb',
  dragonfly: 'picker.taglines.dragonfly',
  mongodb: 'picker.taglines.mongodb',
  clickhouse: 'picker.taglines.clickhouse',
  meilisearch: 'picker.taglines.meilisearch',
  qdrant: 'picker.taglines.qdrant',
  typesense: 'picker.taglines.typesense',
  memcached: 'picker.taglines.memcached',
} as const

const FAMILY_KEYS = {
  all: 'picker.families.all',
  relational: 'picker.families.relational',
  keyvalue: 'picker.families.keyvalue',
  document: 'picker.families.document',
  analytics: 'picker.families.analytics',
  search: 'picker.families.search',
  other: 'picker.families.other',
} as const

const KEY_NAV = new Set([
  'ArrowRight',
  'ArrowLeft',
  'ArrowDown',
  'ArrowUp',
  'Home',
  'End',
])

function isTaglineKey(key: string): key is keyof typeof TAGLINE_KEYS {
  return key in TAGLINE_KEYS
}

function EngineTile({ entry }: { entry: PickerEntry }) {
  return (
    <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border border-border bg-background">
      {entry.brand ? (
        <BrandIcon name={entry.brand} className="size-6" />
      ) : entry.flavor ? (
        <span
          className="text-xs font-semibold text-foreground"
          aria-hidden="true"
        >
          {entry.label.slice(0, 2)}
        </span>
      ) : (
        <DatabaseGlassIcon
          className="size-7 text-muted-foreground"
          aria-hidden="true"
        />
      )}
    </span>
  )
}

export function DatabasePicker({
  entries,
  query,
  selectedKey,
  onSelect,
  showEmpty = true,
  onClearQuery,
}: {
  entries: PickerEntry[]
  /** Already trimmed and lower-cased by the caller. */
  query: string
  selectedKey: string | null
  onSelect: (entry: PickerEntry) => void
  /** False when sibling sections already have matches for the query. */
  showEmpty?: boolean
  onClearQuery: () => void
}) {
  const { t } = useTranslation('databases')
  const [family, setFamily] = useState<FamilyFilter>('all')
  const [focusKey, setFocusKey] = useState<string | null>(null)
  const gridsRef = useRef<HTMLDivElement>(null)

  const taglineFor = (entry: PickerEntry): string =>
    isTaglineKey(entry.key)
      ? t(TAGLINE_KEYS[entry.key])
      : t('picker.genericTagline', { label: entry.label })

  const chipsFor = (entry: PickerEntry): string[] => {
    const chips: string[] = []
    if (entry.port !== null)
      chips.push(t('picker.chips.port', { port: entry.port }))
    if (entry.compat === 'redis') chips.push(t('picker.chips.redisCompatible'))
    if (entry.compat === 'mysql') chips.push(t('picker.chips.mysqlCompatible'))
    if (entry.compat === 'postgres')
      chips.push(t('picker.chips.postgresCompatible'))
    if (entry.httpApi) chips.push(t('picker.chips.httpApi'))
    if (entry.backup === 'dump') chips.push(t('picker.chips.backups'))
    if (entry.backup === 'snapshot') chips.push(t('picker.chips.snapshots'))
    if (entry.backup === 'none') chips.push(t('picker.chips.noPersistence'))
    return chips
  }

  const present = useMemo(
    () => DATABASE_FAMILIES.filter((f) => entries.some((e) => e.family === f)),
    [entries],
  )

  const visible = entries.filter((entry) => {
    if (family !== 'all' && entry.family !== family) return false
    if (!query) return true
    return (
      entry.label.toLowerCase().includes(query) ||
      entry.engineId.includes(query) ||
      taglineFor(entry).toLowerCase().includes(query)
    )
  })

  const groups = DATABASE_FAMILIES.map((f) => ({
    family: f,
    items: visible.filter((e) => e.family === f),
  })).filter((g) => g.items.length > 0)

  const activeKey =
    focusKey && visible.some((e) => e.key === focusKey)
      ? focusKey
      : (visible.find((e) => e.key === selectedKey)?.key ?? visible[0]?.key)

  function onKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    if (!KEY_NAV.has(event.key)) return
    const root = gridsRef.current
    if (!root) return
    const cards = Array.from(
      root.querySelectorAll<HTMLButtonElement>('button[data-card]'),
    )
    const index = cards.findIndex((c) => c === document.activeElement)
    if (index < 0) return
    const grid = cards[index]?.closest<HTMLElement>('[data-grid]')
    const columns = grid
      ? Math.max(
          1,
          getComputedStyle(grid).gridTemplateColumns.split(' ').filter(Boolean)
            .length,
        )
      : 1
    let next = index
    if (event.key === 'ArrowRight') next = index + 1
    if (event.key === 'ArrowLeft') next = index - 1
    if (event.key === 'ArrowDown') next = index + columns
    if (event.key === 'ArrowUp') next = index - columns
    if (event.key === 'Home') next = 0
    if (event.key === 'End') next = cards.length - 1
    next = Math.min(cards.length - 1, Math.max(0, next))
    event.preventDefault()
    cards[next]?.focus()
  }

  function renderCard(entry: PickerEntry) {
    const selected = entry.key === selectedKey
    return (
      <button
        key={entry.key}
        type="button"
        data-card
        data-testid={`engine-card-${entry.key}`}
        tabIndex={entry.key === activeKey ? 0 : -1}
        aria-pressed={selected}
        onFocus={() => {
          setFocusKey(entry.key)
        }}
        onClick={() => {
          onSelect(entry)
        }}
        className={cn(
          'flex h-full flex-col items-start gap-3 rounded-lg border border-border bg-card p-3 text-left transition-colors motion-reduce:transition-none',
          'hover:border-primary/40 hover:bg-muted focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
          selected && 'border-primary ring-2 ring-primary',
        )}
      >
        <span className="flex w-full items-center gap-3">
          <EngineTile entry={entry} />
          <span className="text-sm font-medium text-foreground">
            {entry.label}
          </span>
        </span>
        <span className="text-xs text-muted-foreground">
          {taglineFor(entry)}
        </span>
        <span className="mt-auto flex flex-wrap gap-1">
          {chipsFor(entry).map((chip) => (
            <Badge key={chip} variant="muted" className="font-normal">
              {chip}
            </Badge>
          ))}
        </span>
      </button>
    )
  }

  const gridClassName = 'grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4'

  return (
    <div className="space-y-3">
      <div
        role="group"
        aria-label={t('picker.filterLabel')}
        className="flex flex-wrap gap-1.5"
      >
        {(['all', ...present] as FamilyFilter[]).map((f) => (
          <Button
            key={f}
            type="button"
            size="sm"
            variant={family === f ? 'secondary' : 'ghost'}
            aria-pressed={family === f}
            onClick={() => {
              setFamily(f)
            }}
          >
            {t(FAMILY_KEYS[f])}
          </Button>
        ))}
      </div>
      {visible.length === 0 ? (
        showEmpty && query ? (
          <div className="flex flex-col items-center gap-2 py-10 text-center">
            <SearchGlassIcon
              className="size-7 text-muted-foreground"
              aria-hidden="true"
            />
            <p className="text-sm font-medium text-foreground">
              {t('picker.empty.title', { query })}
            </p>
            <p className="max-w-sm text-xs text-muted-foreground">
              {t('picker.empty.hint')}
            </p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                setFamily('all')
                onClearQuery()
              }}
            >
              {t('picker.empty.clear')}
            </Button>
          </div>
        ) : null
      ) : (
        <div ref={gridsRef} onKeyDown={onKeyDown} className="space-y-4">
          {family === 'all' ? (
            groups.map((g) => (
              <section key={g.family} className="space-y-2">
                <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  {t(FAMILY_KEYS[g.family])}
                </h3>
                <div data-grid className={gridClassName}>
                  {g.items.map(renderCard)}
                </div>
              </section>
            ))
          ) : (
            <div data-grid className={gridClassName}>
              {visible.map(renderCard)}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
