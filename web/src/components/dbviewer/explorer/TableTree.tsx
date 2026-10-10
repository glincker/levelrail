import { useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwiseIcon,
  CaretDownIcon,
  CaretRightIcon,
  EyeIcon,
  MagnifyingGlassIcon,
  TableIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { formatSize } from '../../../lib/format'
import {
  buildTreeItems,
  formatRowEstimate,
  type TreeItem,
} from '../../../lib/explorerTree'
import type { DbSchemaNode } from '../../../types/databaseViewer'

const ROW_HEIGHT_PX = 32
const OVERSCAN_ROWS = 10

export function TableTree({
  schemas,
  selectedKey,
  onSelect,
  onRefresh,
  refreshing,
  truncatedAt,
}: {
  schemas: DbSchemaNode[]
  selectedKey: string | null
  onSelect: (schema: string, table: string) => void
  onRefresh: () => void
  refreshing: boolean
  truncatedAt?: number
}) {
  const { t } = useTranslation('databases')
  const [query, setQuery] = useState('')
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(new Set())
  const [active, setActive] = useState(0)
  const scrollRef = useRef<HTMLDivElement>(null)

  const items = useMemo(
    () => buildTreeItems(schemas, query, collapsed),
    [schemas, query, collapsed],
  )
  const tableCount = useMemo(
    () => schemas.reduce((n, s) => n + s.tables.length, 0),
    [schemas],
  )

  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: OVERSCAN_ROWS,
  })

  useEffect(() => {
    setActive((a) => Math.min(a, Math.max(0, items.length - 1)))
  }, [items.length])

  useEffect(() => {
    if (!selectedKey) return
    const i = items.findIndex((it) => it.key === selectedKey)
    if (i >= 0) {
      setActive(i)
      virtualizer.scrollToIndex(i, { align: 'auto' })
    }
    // Only re-centre when the selection itself changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedKey])

  function toggleSchema(schema: string) {
    setCollapsed((cur) => {
      const next = new Set(cur)
      if (next.has(schema)) next.delete(schema)
      else next.add(schema)
      return next
    })
  }

  function activate(item: TreeItem | undefined) {
    if (!item) return
    if (item.type === 'schema') toggleSchema(item.schema)
    else onSelect(item.schema, item.table.name)
  }

  function move(next: number) {
    const bounded = Math.max(0, Math.min(items.length - 1, next))
    setActive(bounded)
    virtualizer.scrollToIndex(bounded, { align: 'auto' })
  }

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        move(active + 1)
        break
      case 'ArrowUp':
        e.preventDefault()
        move(active - 1)
        break
      case 'Home':
        e.preventDefault()
        move(0)
        break
      case 'End':
        e.preventDefault()
        move(items.length - 1)
        break
      case 'Enter':
      case ' ':
        e.preventDefault()
        activate(items[active])
        break
      default:
    }
  }

  const activeId = items[active] ? `tree-${items[active].key}` : undefined

  return (
    <aside className="space-y-2">
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <MagnifyingGlassIcon
            className="pointer-events-none absolute left-2 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            value={query}
            className="pl-8"
            placeholder={t('viewer.explorer.searchTables')}
            aria-label={t('viewer.explorer.searchTables')}
            onChange={(e) => {
              setQuery(e.target.value)
            }}
          />
        </div>
        <Button
          type="button"
          size="icon"
          variant="outline"
          disabled={refreshing}
          title={t('viewer.explorer.refresh')}
          aria-label={t('viewer.explorer.refresh')}
          onClick={onRefresh}
        >
          <ArrowsClockwiseIcon className="size-4" aria-hidden="true" />
        </Button>
      </div>
      <p className="text-xs text-muted-foreground" aria-live="polite">
        {query
          ? t('viewer.explorer.matches', {
              count: items.filter((i) => i.type === 'table').length,
              total: tableCount,
            })
          : t('viewer.explorer.tableCount', { count: tableCount })}
      </p>
      {truncatedAt ? (
        <p className="rounded-md bg-tone-warning-soft px-2 py-1 text-xs text-tone-warning">
          {t('viewer.explorer.truncated', { count: truncatedAt })}
        </p>
      ) : null}
      <div
        ref={scrollRef}
        role="listbox"
        tabIndex={0}
        aria-label={t('viewer.explorer.tables')}
        aria-activedescendant={activeId}
        onKeyDown={onKeyDown}
        className="max-h-[36rem] min-h-40 overflow-auto rounded-lg border border-border outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
      >
        {items.length === 0 ? (
          <p className="p-4 text-center text-sm text-muted-foreground">
            {t('viewer.explorer.noMatches')}
          </p>
        ) : (
          <div
            className="relative w-full"
            style={{ height: virtualizer.getTotalSize() }}
          >
            {virtualizer.getVirtualItems().map((v) => {
              const item = items[v.index]
              if (!item) return null
              const isActive = v.index === active
              return (
                <div
                  key={item.key}
                  id={`tree-${item.key}`}
                  role="option"
                  aria-selected={
                    item.type === 'table' && item.key === selectedKey
                  }
                  className={cn(
                    'absolute left-0 top-0 flex w-full cursor-pointer items-center gap-2 px-2 text-sm',
                    item.type === 'schema'
                      ? 'bg-muted/50 text-xs font-medium uppercase text-muted-foreground'
                      : 'hover:bg-muted',
                    item.type === 'table' &&
                      item.key === selectedKey &&
                      'bg-muted font-medium',
                    isActive && 'ring-1 ring-inset ring-ring/60',
                  )}
                  style={{
                    height: ROW_HEIGHT_PX,
                    transform: `translateY(${v.start}px)`,
                  }}
                  onClick={() => {
                    setActive(v.index)
                    activate(item)
                  }}
                >
                  {item.type === 'schema' ? (
                    <>
                      {collapsed.has(item.schema) && !query ? (
                        <CaretRightIcon className="size-3" aria-hidden="true" />
                      ) : (
                        <CaretDownIcon className="size-3" aria-hidden="true" />
                      )}
                      <span className="truncate">{item.schema}</span>
                      <span className="ml-auto tabular-nums">{item.count}</span>
                    </>
                  ) : (
                    <>
                      {item.table.kind === 'table' ? (
                        <TableIcon
                          className="size-4 shrink-0 text-muted-foreground"
                          aria-hidden="true"
                        />
                      ) : (
                        <EyeIcon
                          className="size-4 shrink-0 text-muted-foreground"
                          aria-hidden="true"
                        />
                      )}
                      <span className="min-w-0 flex-1 truncate">
                        {item.table.name}
                      </span>
                      <span className="shrink-0 text-xs tabular-nums text-muted-foreground">
                        {item.table.kind === 'table'
                          ? `${formatRowEstimate(item.table.row_estimate)} / ${formatSize(item.table.size_bytes)}`
                          : t('viewer.explorer.view')}
                      </span>
                    </>
                  )}
                </div>
              )
            })}
          </div>
        )}
      </div>
    </aside>
  )
}
