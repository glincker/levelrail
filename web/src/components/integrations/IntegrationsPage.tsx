import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import {
  DEFAULT_FILTERS,
  attentionList,
  filterConnections,
  groupByKind,
  type ConnectionFilters,
} from '../../lib/connections'
import { useConnections } from '../../queries/connections'
import { AttentionStrip } from './AttentionStrip'
import { CatalogSection } from './CatalogSection'
import { ConnectionCard } from './ConnectionCard'
import { FilterBar } from './FilterBar'

export function IntegrationsPage({
  focusCatalog = false,
}: Readonly<{ focusCatalog?: boolean }>) {
  const { t } = useTranslation('integrations')
  const { data, isPending, isError, refetch } = useConnections()
  const [filters, setFilters] = useState<ConnectionFilters>(DEFAULT_FILTERS)
  const [now] = useState(() => Date.now())
  const catalogRef = useRef<HTMLDivElement>(null)

  // Create modal entry lands here with the catalogue in view.
  useEffect(() => {
    if (focusCatalog) catalogRef.current?.scrollIntoView({ block: 'start' })
  }, [focusCatalog])

  const all = data ?? []
  const visible = filterConnections(all, filters, now)
  const groups = groupByKind(visible)
  const filtered =
    filters.kind !== 'all' ||
    filters.scope !== 'all' ||
    filters.health !== 'all' ||
    filters.query.trim() !== ''

  let connected: React.ReactNode
  if (isPending) {
    connected = <Skeleton className="h-32 w-full" />
  } else if (isError) {
    connected = (
      <EmptyState
        kind="error"
        icon={<PlugGlassIcon className="size-6" />}
        title={t('loadFailed')}
        action={
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void refetch()
            }}
          >
            {t('retry')}
          </Button>
        }
      />
    )
  } else if (all.length === 0) {
    connected = (
      <EmptyState
        compact
        icon={<PlugGlassIcon className="size-6" />}
        title={t('empty.title')}
        description={t('empty.description')}
      />
    )
  } else if (groups.length === 0) {
    connected = (
      <EmptyState
        compact
        icon={<PlugGlassIcon className="size-6" />}
        title={t('empty.noMatch')}
        action={
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setFilters(DEFAULT_FILTERS)
            }}
          >
            {t('filters.clear')}
          </Button>
        }
      />
    )
  } else {
    connected = (
      <div className="space-y-4">
        {groups.map((g) => (
          <section
            key={g.kind}
            aria-labelledby={`integrations-kind-${g.kind}`}
            className="rounded-lg border border-border bg-card px-3"
          >
            <h3
              id={`integrations-kind-${g.kind}`}
              className="border-b border-border py-2 text-xs font-semibold text-muted-foreground"
            >
              {t(`kinds.${g.kind}`)} ({g.items.length})
            </h3>
            <ul className="divide-y divide-border">
              {g.items.map((c) => (
                <ConnectionCard key={c.ref} conn={c} now={now} />
              ))}
            </ul>
          </section>
        ))}
      </div>
    )
  }

  return (
    <PageTemplate
      icon={<PlugGlassIcon size="1.4em" />}
      title={t('title')}
      description={t('description')}
      width="wide"
    >
      <div className="space-y-6">
        <AttentionStrip items={attentionList(all, now)} now={now} />
        <section aria-labelledby="integrations-connected" className="space-y-3">
          <h2
            id="integrations-connected"
            className="text-base font-semibold text-foreground"
          >
            {t('connected.title')}
          </h2>
          {all.length > 0 || filtered ? (
            <FilterBar filters={filters} onChange={setFilters} />
          ) : null}
          {connected}
        </section>
        <div ref={catalogRef}>
          <CatalogSection />
        </div>
      </div>
    </PageTemplate>
  )
}
