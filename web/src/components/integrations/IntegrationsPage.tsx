import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import {
  DEFAULT_FILTERS,
  attentionList,
  filterConnections,
  groupByKind,
  type ConnectionFilters,
} from '../../lib/connections'
import { useConnections } from '../../queries/connections'
import { CardGrid, ObjectCardSkeleton } from '../cards/ObjectCard'
import { AttentionStrip } from './AttentionStrip'
import { CatalogSection } from './CatalogSection'
import { ConnectionCard } from './ConnectionCard'
import { FilterBar } from './FilterBar'

export function IntegrationsPage({
  focusCatalog = false,
  reauthorizeConnection,
}: Readonly<{
  focusCatalog?: boolean
  /** Git connection id from an attention link: scroll to its card and ready Reconnect. */
  reauthorizeConnection?: string
}>) {
  const { t } = useTranslation('integrations')
  const { data, isPending, isError, refetch } = useConnections()
  const [filters, setFilters] = useState<ConnectionFilters>(DEFAULT_FILTERS)
  const [now] = useState(() => Date.now())
  const catalogRef = useRef<HTMLDivElement>(null)
  const targetRef = reauthorizeConnection ? `git:${reauthorizeConnection}` : ''

  // Create modal entry lands here with the catalogue in view.
  useEffect(() => {
    if (focusCatalog) catalogRef.current?.scrollIntoView({ block: 'start' })
  }, [focusCatalog])

  const all = data ?? []
  const targetFound = targetRef !== '' && all.some((c) => c.ref === targetRef)

  useEffect(() => {
    if (!targetFound) return
    const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches
    document.getElementById(`integration-${targetRef}`)?.scrollIntoView({
      block: 'center',
      behavior: reduce ? 'auto' : 'smooth',
    })
  }, [targetFound, targetRef])

  // The linked card must stay visible whatever filter was left on.
  const matched = filterConnections(all, filters, now)
  const linked = all.find((c) => c.ref === targetRef)
  const visible =
    linked && !matched.includes(linked) ? [...matched, linked] : matched
  const groups = groupByKind(visible)
  const filtered =
    filters.kind !== 'all' ||
    filters.scope !== 'all' ||
    filters.health !== 'all' ||
    filters.query.trim() !== ''

  let connected: React.ReactNode
  if (isPending) {
    connected = <ObjectCardSkeleton count={3} />
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
      <div className="space-y-6">
        {groups.map((g) => (
          <section
            key={g.kind}
            aria-labelledby={`integrations-kind-${g.kind}`}
            className="space-y-2"
          >
            <h3
              id={`integrations-kind-${g.kind}`}
              className="flex items-baseline gap-1.5 text-xs font-semibold text-muted-foreground"
            >
              {t(`kinds.${g.kind}`)}
              <span className="font-normal tabular-nums">{g.items.length}</span>
            </h3>
            <CardGrid min="lg">
              {g.items.map((c) => (
                <ConnectionCard
                  key={c.ref}
                  conn={c}
                  now={now}
                  highlight={c.ref === targetRef}
                />
              ))}
            </CardGrid>
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
        <section aria-labelledby="integrations-connected" className="space-y-4">
          <h2
            id="integrations-connected"
            className="font-(family-name:--glinr-font-display) text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground"
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
