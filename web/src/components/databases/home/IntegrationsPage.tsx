import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { EngineMark } from '@/components/EngineMark'
import {
  useDatabaseIntegrations,
  type IntegrationCard,
  type IntegrationStateCode,
} from '../../../queries/databaseIntegrations'

// The card docs field is a URL, a rooted path or a bare page slug under /docs.
function docsHref(docs: string): string {
  return /^(https?:)?\//.test(docs) ? docs : `/docs/${docs}`
}
const CATEGORY_ORDER = ['monitor', 'develop', 'deploy', 'query', 'replicate']

// Only cards whose setup lives on an existing page get a Configure action.
const CONFIGURE_ROUTE: Record<string, string> = {
  datadog: '/settings/data-export',
  opentelemetry: '/settings/data-export',
}

const CHIP_VARIANT: Record<IntegrationStateCode, 'default' | 'muted'> = {
  connected: 'default',
  available: 'muted',
  needs_setup: 'muted',
  docs_only: 'muted',
  unavailable: 'muted',
  planned: 'muted',
}

function IntegrationTile({ card }: Readonly<{ card: IntegrationCard }>) {
  const { t } = useTranslation('dbIntegrations')
  const configure = CONFIGURE_ROUTE[card.id]
  const actionable =
    configure !== undefined &&
    card.kind === 'connector' &&
    card.state.code !== 'planned' &&
    card.state.code !== 'unavailable'
  return (
    <li className="flex flex-col gap-2 rounded-lg border bg-card p-3">
      <div className="flex items-center gap-2">
        <EngineMark engine={card.id} className="size-6 shrink-0" />
        <h3 className="min-w-0 flex-1 truncate text-sm font-medium">
          {card.title}
        </h3>
        <Badge
          variant={CHIP_VARIANT[card.state.code]}
          title={card.state.reason}
        >
          {t(`state.${card.state.code}`)}
        </Badge>
      </div>
      <p className="flex-1 text-xs text-muted-foreground">{card.summary}</p>
      <div className="flex items-center gap-2">
        {actionable ? (
          <Button size="sm" render={<Link to={configure} />}>
            {t('configure')}
          </Button>
        ) : null}
        {card.docs_planned ? (
          <span className="text-xs text-muted-foreground">
            {t('docsPlanned')}
          </span>
        ) : (
          <a
            href={docsHref(card.docs)}
            target="_blank"
            rel="noreferrer"
            className="text-xs underline underline-offset-2"
          >
            {t('docs')}
          </a>
        )}
      </div>
    </li>
  )
}

function Grid({ cards }: Readonly<{ cards: IntegrationCard[] }>) {
  return (
    <ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
      {cards.map((c) => (
        <IntegrationTile key={c.id} card={c} />
      ))}
    </ul>
  )
}

/** IntegrationsPage lists the integration catalog for one database: featured first, then grouped by category. */
export function IntegrationsPage({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('dbIntegrations')
  const { data, isLoading, error } = useDatabaseIntegrations(name)
  const [query, setQuery] = useState('')

  const cards = useMemo(() => {
    const q = query.trim().toLowerCase()
    return (data?.integrations ?? []).filter(
      (c) =>
        q === '' ||
        c.title.toLowerCase().includes(q) ||
        c.summary.toLowerCase().includes(q),
    )
  }, [data, query])
  const featured = cards.filter((c) => c.featured)
  const rest = cards.filter((c) => !c.featured)
  const groups = CATEGORY_ORDER.map((cat) => ({
    cat,
    items: rest.filter((c) => c.category === cat),
  })).filter((g) => g.items.length > 0)

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
            {t('title')}
          </h2>
          <p className="text-sm text-muted-foreground">{t('description')}</p>
        </div>
        <Input
          type="search"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value)
          }}
          aria-label={t('search')}
          placeholder={t('search')}
          className="w-full sm:w-64"
        />
      </div>
      {isLoading ? (
        <Skeleton className="h-40 w-full" />
      ) : error ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('loadFailed', { error: error.message })}
          </AlertDescription>
        </Alert>
      ) : cards.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('empty')}</p>
      ) : (
        <>
          {featured.length > 0 ? (
            <section
              aria-labelledby="integrations-featured"
              className="space-y-2"
            >
              <h3
                id="integrations-featured"
                className="text-xs font-semibold text-muted-foreground"
              >
                {t('featured')}
              </h3>
              <Grid cards={featured} />
            </section>
          ) : null}
          {groups.map((g) => (
            <section
              key={g.cat}
              aria-labelledby={`integrations-${g.cat}`}
              className="space-y-2"
            >
              <h3
                id={`integrations-${g.cat}`}
                className="text-xs font-semibold text-muted-foreground"
              >
                {t(`categories.${g.cat}` as 'categories.monitor')}
              </h3>
              <Grid cards={g.items} />
            </section>
          ))}
        </>
      )}
    </div>
  )
}
