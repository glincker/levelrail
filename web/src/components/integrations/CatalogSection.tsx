import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { catalogDestination, groupCatalog } from '../../lib/connections'
import { useConnectionCatalog } from '../../queries/connections'
import type { CatalogEntry, ConnectMethod } from '../../types/connections'
import { ConnectionGlyph } from './ConnectionGlyph'

const METHOD_ORDER: readonly ConnectMethod[] = [
  'one_click',
  'guided_oauth',
  'token',
  'settings_page',
]

// The hub never forks a create form: every connect button opens the page that owns it.
function CatalogCard({ entry }: Readonly<{ entry: CatalogEntry }>) {
  const { t } = useTranslation('integrations')
  const methods = METHOD_ORDER.filter((m) =>
    (entry.connect_methods ?? []).includes(m),
  )
  const primary = methods[0] ?? 'settings_page'
  const to = catalogDestination(entry.category)
  return (
    <li className="flex items-center gap-3 rounded-lg border border-border bg-card p-3">
      <ConnectionGlyph provider={entry.id} kind={entry.category} />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium text-foreground">
          {entry.name}
        </p>
        <p className="truncate text-xs text-muted-foreground">
          {methods.length > 0
            ? methods.map((m) => t(`methods.${m}`)).join(' · ')
            : t('methods.settings_page')}
        </p>
      </div>
      <Button
        variant="outline"
        size="sm"
        render={<Link to={to} />}
        aria-label={t('catalog.connectNamed', { name: entry.name })}
      >
        <PlusGlassIcon aria-hidden="true" />
        {t(`connect.${primary}`)}
      </Button>
    </li>
  )
}

export function CatalogSection() {
  const { t } = useTranslation('integrations')
  const { data, isPending, isError, refetch } = useConnectionCatalog()
  const groups = groupCatalog(data ?? [])

  let body: React.ReactNode
  if (isPending) {
    body = <Skeleton className="h-24 w-full" />
  } else if (isError) {
    body = (
      <EmptyState
        compact
        icon={<PlusGlassIcon className="size-6" />}
        title={t('catalog.loadFailed')}
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
  } else if (groups.length === 0) {
    body = (
      <EmptyState
        compact
        icon={<PlusGlassIcon className="size-6" />}
        title={t('catalog.empty')}
      />
    )
  } else {
    body = (
      <div className="space-y-5">
        {groups.map((g) => (
          <div key={g.category} className="space-y-2">
            <h3 className="text-xs font-semibold text-muted-foreground">
              {t(`categories.${g.category}`, { defaultValue: g.category })}
            </h3>
            <ul className="grid gap-2 md:grid-cols-2 xl:grid-cols-3">
              {g.items.map((entry) => (
                <CatalogCard key={entry.id} entry={entry} />
              ))}
            </ul>
          </div>
        ))}
      </div>
    )
  }

  return (
    <section
      id="integrations-catalog"
      aria-labelledby="integrations-catalog-title"
      className="scroll-mt-4 space-y-3"
    >
      <div>
        <h2
          id="integrations-catalog-title"
          className="text-base font-semibold text-foreground"
        >
          {t('catalog.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('catalog.description')}
        </p>
      </div>
      {body}
    </section>
  )
}
