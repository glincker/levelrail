import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  ChevronRightGlassIcon,
  DatabaseGlassIcon,
  RefreshCwGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { formatSize } from '../../../lib/format'
import type { MongoCollectionsResponse } from '../../../types/databaseMongo'

export interface MongoSelection {
  database: string
  collection: string
}

export function MongoTree({
  data,
  selected,
  onSelect,
  onRefresh,
  refreshing,
}: {
  data: MongoCollectionsResponse
  selected: MongoSelection | null
  onSelect: (s: MongoSelection) => void
  onRefresh: () => void
  refreshing: boolean
}) {
  const { t } = useTranslation('databases')
  const [query, setQuery] = useState('')
  const [closed, setClosed] = useState<ReadonlySet<string>>(new Set())

  const groups = useMemo(() => {
    const q = query.trim().toLowerCase()
    return data.databases.map((db) => ({
      db,
      collections: data.collections.filter(
        (c) =>
          c.db === db.name && (q === '' || c.name.toLowerCase().includes(q)),
      ),
    }))
  }, [data, query])

  function toggle(name: string) {
    setClosed((cur) => {
      const next = new Set(cur)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  return (
    <nav aria-label={t('viewer.mongo.tree.label')} className="space-y-2">
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <SearchGlassIcon
            className="pointer-events-none absolute left-2 top-1/2 size-5 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            value={query}
            className="pl-8"
            placeholder={t('viewer.mongo.tree.search')}
            aria-label={t('viewer.mongo.tree.search')}
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
          <RefreshCwGlassIcon className="size-5" aria-hidden="true" />
        </Button>
      </div>
      {data.truncated ? (
        <p className="rounded-md bg-tone-warning-soft px-2 py-1 text-xs text-tone-warning">
          {t('viewer.mongo.tree.truncated')}
        </p>
      ) : null}
      <ul className="max-h-[36rem] space-y-1 overflow-auto">
        {groups.map(({ db, collections }) => {
          const isClosed = closed.has(db.name)
          return (
            <li key={db.name}>
              <button
                type="button"
                aria-expanded={!isClosed}
                onClick={() => {
                  toggle(db.name)
                }}
                className="flex w-full items-center gap-1 rounded-md px-1 py-1 text-left text-sm font-medium hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
              >
                {isClosed ? (
                  <ChevronRightGlassIcon
                    className="size-4"
                    aria-hidden="true"
                  />
                ) : (
                  <ChevronDownGlassIcon className="size-4" aria-hidden="true" />
                )}
                <DatabaseGlassIcon className="size-4" aria-hidden="true" />
                <span className="truncate">{db.name}</span>
                <span className="ml-auto text-xs font-normal text-muted-foreground tabular-nums">
                  {formatSize(db.sizeOnDisk)}
                </span>
              </button>
              {isClosed ? null : (
                <ul className="ml-3 border-l border-border pl-1">
                  {collections.length === 0 ? (
                    <li className="px-2 py-1 text-xs text-muted-foreground">
                      {t('viewer.mongo.tree.noCollections')}
                    </li>
                  ) : null}
                  {collections.map((c) => {
                    const active =
                      selected?.database === c.db &&
                      selected.collection === c.name
                    return (
                      <li key={c.name}>
                        <button
                          type="button"
                          aria-current={active ? 'true' : undefined}
                          onClick={() => {
                            onSelect({ database: c.db, collection: c.name })
                          }}
                          className={cn(
                            'flex w-full items-center gap-2 rounded-md px-2 py-1 text-left text-sm hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring',
                            active && 'bg-muted font-medium',
                          )}
                        >
                          <span className="min-w-0 flex-1">
                            <span className="block truncate">{c.name}</span>
                            <span className="block truncate text-xs font-normal text-muted-foreground">
                              {t('viewer.mongo.tree.meta', {
                                size: formatSize(c.size),
                                count: c.indexes.length,
                              })}
                            </span>
                          </span>
                          <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                            {c.count}
                          </span>
                        </button>
                      </li>
                    )
                  })}
                </ul>
              )}
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
