import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LayersGlassIcon, SearchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { emptyQuery, resourceKey } from '@/lib/resourceLinks'
import {
  RESOURCE_KINDS,
  useResourceList,
  type BulkTagAction,
  type ConsoleQuery,
  type ConsoleResource,
} from '@/queries/resources'
import { BulkAppActionDialog, type BulkAppAction } from './BulkAppActionDialog'
import { BulkTagDialog } from './BulkTagDialog'
import { CostByTag } from './CostByTag'
import { ResourceRows } from './ResourceRows'
import { ResourceTagFilter } from './ResourceTagFilter'
import { SavedViews } from './SavedViews'

export function ResourceExplorer({
  query,
  onQueryChange,
}: {
  query: ConsoleQuery
  onQueryChange: (q: ConsoleQuery) => void
}) {
  const { t } = useTranslation('resources')
  const list = useResourceList(query)
  const [selected, setSelected] = useState<Map<string, ConsoleResource>>(
    () => new Map(),
  )
  const [dialog, setDialog] = useState<BulkTagAction | null>(null)
  const [appAction, setAppAction] = useState<BulkAppAction | null>(null)
  const items = useMemo(() => list.data?.items ?? [], [list.data])
  const keys = useMemo(() => new Set(selected.keys()), [selected])
  const selectedApps = useMemo(
    () =>
      [...selected.values()].filter((r) => r.kind === 'app').map((r) => r.name),
    [selected],
  )

  function toggle(r: ConsoleResource) {
    setSelected((prev) => {
      const next = new Map(prev)
      const k = resourceKey(r)
      if (next.has(k)) next.delete(k)
      else next.set(k, r)
      return next
    })
  }

  function toggleAll() {
    setSelected((prev) => {
      const allOn =
        items.length > 0 && items.every((r) => prev.has(resourceKey(r)))
      const next = new Map(prev)
      for (const r of items) {
        if (allOn) next.delete(resourceKey(r))
        else next.set(resourceKey(r), r)
      }
      return next
    })
  }

  function addTagFilter(tag: string) {
    if (!query.tags.includes(tag))
      onQueryChange({ ...query, tags: [...query.tags, tag] })
  }

  const counts = list.data?.counts ?? {}
  const filtered = query.q !== '' || query.kind !== '' || query.tags.length > 0
  const allCount = Object.values(counts).reduce((n, c) => n + (c ?? 0), 0)

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_18rem]">
      <section className="min-w-0 space-y-3">
        <div className="flex flex-wrap items-center gap-2">
          <div className="relative min-w-52 flex-1">
            <SearchGlassIcon
              size="1.4em"
              className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              type="search"
              className="pl-8"
              value={query.q}
              aria-label={t('search.label')}
              placeholder={t('search.placeholder')}
              onChange={(e) => onQueryChange({ ...query, q: e.target.value })}
            />
          </div>
          <ResourceTagFilter
            selected={query.tags}
            onChange={(tags) => onQueryChange({ ...query, tags })}
          />
          <SavedViews current={query} onApply={onQueryChange} />
        </div>
        <div
          className="flex flex-wrap gap-1"
          role="group"
          aria-label={t('list.label')}
        >
          <Button
            size="sm"
            variant={query.kind === '' ? 'secondary' : 'ghost'}
            aria-pressed={query.kind === ''}
            onClick={() => onQueryChange({ ...query, kind: '' })}
          >
            {t('kind.all')} {allCount}
          </Button>
          {RESOURCE_KINDS.map((k) => (
            <Button
              key={k}
              size="sm"
              variant={query.kind === k ? 'secondary' : 'ghost'}
              aria-pressed={query.kind === k}
              onClick={() => onQueryChange({ ...query, kind: k })}
            >
              {t(`kind.${k}`)} {counts[k] ?? 0}
            </Button>
          ))}
        </div>
        {selected.size > 0 ? (
          <div
            className="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/40 px-3 py-2"
            role="toolbar"
          >
            <span className="text-sm font-medium">
              {t('bulk.selected', { count: selected.size })}
            </span>
            <Button size="sm" onClick={() => setDialog('add')}>
              {t('bulk.addTag')}
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setDialog('remove')}
            >
              {t('bulk.removeTag')}
            </Button>
            {selectedApps.length > 0 ? (
              <>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setAppAction('restart')}
                >
                  {t('appAction.restart')}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setAppAction('redeploy')}
                >
                  {t('appAction.redeploy')}
                </Button>
              </>
            ) : null}
            <Button
              size="sm"
              variant="ghost"
              onClick={() => setSelected(new Map())}
            >
              {t('bulk.clear')}
            </Button>
          </div>
        ) : null}
        {list.isPending ? (
          <Skeleton className="h-64 w-full rounded-lg" />
        ) : list.isError ? (
          <EmptyState
            icon={<LayersGlassIcon className="size-6" />}
            title={t('list.errorTitle')}
            description={list.error.message}
            action={
              <Button variant="outline" onClick={() => void list.refetch()}>
                {t('list.retry')}
              </Button>
            }
          />
        ) : items.length === 0 ? (
          <EmptyState
            icon={<SearchGlassIcon className="size-6" />}
            title={t('list.emptyTitle')}
            description={t('list.emptyDescription')}
            action={
              filtered ? (
                <Button
                  variant="outline"
                  onClick={() => onQueryChange(emptyQuery())}
                >
                  {t('list.emptyAction')}
                </Button>
              ) : undefined
            }
          />
        ) : (
          <>
            <ResourceRows
              items={items}
              selected={keys}
              onToggle={toggle}
              onToggleAll={toggleAll}
              onTag={addTagFilter}
            />
            <p className="text-xs text-muted-foreground">
              {t('list.shown', {
                shown: items.length,
                total: list.data?.total ?? 0,
              })}
            </p>
          </>
        )}
      </section>
      <aside className="space-y-4">
        <CostByTag onPickTag={addTagFilter} />
      </aside>
      {appAction ? (
        <BulkAppActionDialog
          open
          onOpenChange={(o) => {
            if (!o) setAppAction(null)
          }}
          action={appAction}
          names={selectedApps}
          onDone={() => setSelected(new Map())}
        />
      ) : null}
      {dialog ? (
        <BulkTagDialog
          open
          onOpenChange={(o) => {
            if (!o) setDialog(null)
          }}
          action={dialog}
          selected={[...selected.values()]}
          onDone={() => setSelected(new Map())}
        />
      ) : null}
    </div>
  )
}
