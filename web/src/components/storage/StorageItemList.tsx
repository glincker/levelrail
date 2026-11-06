import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon, SearchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { RelativeTime } from '@/components/kit'
import { cn } from '@/lib/utils'
import { formatSize } from '../../lib/format'
import { useImageInventory } from '../../queries/imageInventory'
import { useRuntimeSettings } from '../../queries/runtimeSettings'
import { useStorageInventory } from '../../queries/storageInventory'
import {
  ITEM_KINDS,
  buildItemRows,
  filterRows,
  groupRows,
  largeUnmanagedBytes,
  parseFilter,
  parseView,
  type ItemGroup,
  type ItemView,
  type SortKey,
  type StorageItemRow,
} from './itemRows'

export interface StorageItemListProps {
  node: string
  /** Raw `view` search param; anything but "app" means By type. */
  view: string | undefined
  onViewChange: (view: ItemView) => void
  /** Raw `filter` search param, one of the disk bar categories. */
  filter?: string
  onClearFilter?: () => void
  /** Docker's own build cache total; there is no per-item build cache list. */
  buildCacheBytes?: number | null
  renderRowAction?: (row: StorageItemRow) => ReactNode
}

function Reason({ row }: { row: StorageItemRow }) {
  const { t } = useTranslation('storageList')
  const { code, text, app } = row.reason
  const label = code === 'text' ? (text ?? '') : t(`reason.${code}`, { app })
  const amber = row.tone === 'amber'
  return (
    <span
      className={cn(
        'flex min-w-0 items-center gap-1.5 text-xs',
        amber ? 'text-tone-warning' : 'text-muted-foreground',
      )}
    >
      {row.protected && (
        <>
          <LockGlassIcon size="1em" aria-hidden className="shrink-0" />
          <span className="sr-only">{t('protected')}: </span>
        </>
      )}
      <span className="min-w-0 break-words">{label}</span>
    </span>
  )
}

function SortHead({
  active,
  column,
  onSort,
  sortKey,
  className,
}: {
  active: SortKey
  column: string
  sortKey: SortKey
  onSort: (k: SortKey) => void
  className?: string
}) {
  const { t } = useTranslation('storageList')
  return (
    <button
      type="button"
      onClick={() => onSort(sortKey)}
      aria-label={t('sortBy', { column })}
      aria-pressed={active === sortKey}
      className={cn(
        'text-left text-xs font-medium hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        active === sortKey ? 'text-foreground' : 'text-muted-foreground',
        className,
      )}
    >
      {column}
    </button>
  )
}

function GroupHeader({ group, view }: { group: ItemGroup; view: ItemView }) {
  const { t } = useTranslation('storageList')
  const title =
    view === 'type' && group.kind
      ? t(`kind.${group.kind}`)
      : (group.app ?? t('noApp'))
  const counts = ITEM_KINDS.filter((k) => group.counts[k] > 0).map((k) =>
    t('group.count', {
      count: group.counts[k],
      kind: t(`kind.${k}`).toLowerCase(),
    }),
  )
  return (
    <div className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 bg-muted/40 px-4 py-2">
      <h3 className="text-sm font-medium">{title}</h3>
      <span className="text-xs text-muted-foreground">
        {t('group.total', { size: formatSize(group.bytes) })}
      </span>
      {view === 'app' && (
        <span className="text-xs text-muted-foreground">
          {counts.join(', ')}
        </span>
      )}
      {view === 'app' && group.lastAt && (
        <span className="text-xs text-muted-foreground">
          {t('group.lastDeployed')} <RelativeTime at={group.lastAt} />
        </span>
      )}
    </div>
  )
}

export function StorageItemList({
  node,
  view,
  onViewChange,
  filter,
  onClearFilter,
  buildCacheBytes,
  renderRowAction,
}: StorageItemListProps) {
  const { t } = useTranslation('storageList')
  const [query, setQuery] = useState('')
  const [sort, setSort] = useState<SortKey>('size')
  const inventory = useStorageInventory(node)
  const images = useImageInventory(node)
  const largeBytes = largeUnmanagedBytes(useRuntimeSettings().data?.settings)
  const activeView = parseView(view)
  const activeFilter = parseFilter(filter)
  const unsupported = inventory.data?.unsupported === true

  const rows = useMemo(
    () =>
      buildItemRows({
        images: images.data?.images ?? [],
        inventory: inventory.data,
        buildCacheBytes,
        largeBytes,
      }),
    [images.data, inventory.data, buildCacheBytes, largeBytes],
  )
  const groups = useMemo(
    () => groupRows(filterRows(rows, activeFilter, query), activeView, sort),
    [rows, activeFilter, query, activeView, sort],
  )

  if (unsupported) {
    return (
      <section
        aria-label={t('unsupportedTitle')}
        className="rounded-lg border bg-card px-4 py-6 text-sm"
      >
        <p className="font-medium">{t('unsupportedTitle')}</p>
        <p className="mt-1 text-muted-foreground">
          {inventory.data?.reason || t('unsupportedFallback')}
        </p>
      </section>
    )
  }

  const loading = inventory.isPending || images.isPending
  const failed = inventory.isError || images.isError
  const colsClass = renderRowAction
    ? 'md:grid-cols-[minmax(0,2fr)_6rem_minmax(0,1fr)_minmax(0,2fr)_auto]'
    : 'md:grid-cols-[minmax(0,2fr)_6rem_minmax(0,1fr)_minmax(0,2fr)]'
  const grid = cn(
    'grid grid-cols-1 items-start gap-x-3 gap-y-0.5 px-4',
    colsClass,
  )

  return (
    <section className="space-y-3" aria-busy={loading}>
      <div className="flex flex-wrap items-center gap-2">
        <div
          role="group"
          aria-label={t('viewLabel')}
          className="inline-flex rounded-md border p-0.5"
        >
          {(['type', 'app'] as const).map((v) => (
            <Button
              key={v}
              type="button"
              size="sm"
              variant={activeView === v ? 'secondary' : 'ghost'}
              aria-pressed={activeView === v}
              onClick={() => onViewChange(v)}
            >
              {t(`view.${v}`)}
            </Button>
          ))}
        </div>
        <div className="relative min-w-48 flex-1 sm:max-w-xs">
          <SearchGlassIcon
            size="1em"
            aria-hidden
            className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t('search')}
            aria-label={t('searchLabel')}
            className="pl-8"
          />
        </div>
        {activeFilter && (
          <span className="flex items-center gap-2 text-xs text-muted-foreground">
            {t('filterActive', { category: t(`category.${activeFilter}`) })}
            {onClearFilter && (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={onClearFilter}
              >
                {t('clearFilter')}
              </Button>
            )}
          </span>
        )}
      </div>

      {loading && (
        <p role="status" className="px-1 py-6 text-sm text-muted-foreground">
          {t('loading')}
        </p>
      )}
      {!loading && failed && (
        <div role="alert" className="flex items-center gap-3 px-1 py-6 text-sm">
          <span>{t('error')}</span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              void inventory.refetch()
              void images.refetch()
            }}
          >
            {t('retry')}
          </Button>
        </div>
      )}
      {!loading && !failed && groups.length === 0 && (
        <p className="px-1 py-6 text-sm text-muted-foreground">
          {query || activeFilter ? t('emptyFiltered') : t('empty')}
        </p>
      )}

      {!loading && !failed && groups.length > 0 && (
        <div className="overflow-hidden rounded-lg border bg-card">
          <div className={cn(grid, 'hidden border-b py-2 md:grid')}>
            <SortHead
              active={sort}
              sortKey="name"
              column={t('cols.name')}
              onSort={setSort}
            />
            <SortHead
              active={sort}
              sortKey="size"
              column={t('cols.size')}
              onSort={setSort}
            />
            <span className="text-xs font-medium text-muted-foreground">
              {activeView === 'type' ? t('cols.app') : t('cols.type')}
            </span>
            <span className="text-xs font-medium text-muted-foreground">
              {t('cols.reason')}
            </span>
            {renderRowAction && <span />}
          </div>
          {groups.map((group) => (
            <div
              key={group.key}
              role="rowgroup"
              className="border-b last:border-b-0"
            >
              <GroupHeader group={group} view={activeView} />
              <ul className="divide-y">
                {group.rows.map((row) => (
                  <li key={row.id} className={cn(grid, 'py-2')}>
                    <span
                      className={cn(
                        'min-w-0 truncate text-sm',
                        row.mono && 'font-mono text-xs',
                      )}
                      title={row.name}
                    >
                      {row.kind === 'buildCache'
                        ? t('kindOne.buildCache')
                        : row.name}
                    </span>
                    <span className="text-sm tabular-nums">
                      {row.bytes === null ? (
                        <span className="text-xs text-muted-foreground">
                          {t('sizeUnknown')}
                        </span>
                      ) : (
                        formatSize(row.bytes)
                      )}
                    </span>
                    <span className="min-w-0 truncate text-xs text-muted-foreground">
                      {activeView === 'type'
                        ? (row.ownerApp ?? t('noApp'))
                        : t(`kindOne.${row.kind}`)}
                    </span>
                    <Reason row={row} />
                    {renderRowAction && (
                      <span className="justify-self-end">
                        {renderRowAction(row)}
                      </span>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
