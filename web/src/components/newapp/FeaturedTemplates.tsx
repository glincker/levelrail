import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { ArrowRightGlassIcon, SearchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import {
  useServiceTemplates,
  type ServiceTemplateListItem,
} from '../../queries/serviceTemplates'
import { FEATURED_COUNT, pickFeatured } from '../../lib/featuredTemplates'
import { TEMPLATE_LOGO_LOADERS } from '../../lib/templateLogos'
import { TemplateLogoTile } from '../TemplateLogoTile'
import { SectionHeading } from './SectionHeading'
import { StarterTemplatesDialog } from './StarterTemplatesDialog'

function matches(tpl: ServiceTemplateListItem, q: string): boolean {
  return (
    tpl.name.toLowerCase().includes(q) ||
    tpl.id.toLowerCase().includes(q) ||
    tpl.category.toLowerCase().includes(q)
  )
}

/** Featured templates with category chips and a search that reaches the whole catalog. */
export function FeaturedTemplates() {
  const { t } = useTranslation('newApp')
  const { data, isPending, isError, refetch } = useServiceTemplates()
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState<string | null>(null)
  const list = useMemo(() => (Array.isArray(data) ? data : []), [data])
  const featured = useMemo(() => pickFeatured(list), [list])
  const categories = useMemo(
    () => [...new Set(featured.map((tpl) => tpl.category))].sort(),
    [featured],
  )
  const q = query.trim().toLowerCase()
  const filtering = q !== '' || category !== null
  const shown = useMemo(() => {
    if (!filtering) return featured
    return list
      .filter(
        (tpl) =>
          (category === null || tpl.category === category) &&
          (q === '' || matches(tpl, q)),
      )
      .sort(
        (a, b) =>
          Number(b.id in TEMPLATE_LOGO_LOADERS) -
          Number(a.id in TEMPLATE_LOGO_LOADERS),
      )
      .slice(0, FEATURED_COUNT)
  }, [filtering, featured, list, category, q])

  return (
    <section
      aria-labelledby="new-app-templates"
      className="space-y-3 rounded-xl border border-border bg-card p-3 shadow-xs"
    >
      <SectionHeading
        id="new-app-templates"
        action={
          <Link
            to="/templates"
            className="inline-flex items-center gap-1 text-sm font-medium text-foreground underline-offset-2 hover:underline"
          >
            {list.length > 0
              ? t('source.templates.seeAll', { count: list.length })
              : t('source.templates.browse')}
            <ArrowRightGlassIcon className="size-4" aria-hidden="true" />
          </Link>
        }
      >
        {t('source.templates.title')}
      </SectionHeading>
      {list.length > 0 ? (
        <>
          <div className="relative">
            <SearchGlassIcon
              className="pointer-events-none absolute top-1/2 left-2.5 size-4.5 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
              }}
              placeholder={t('source.templates.search')}
              aria-label={t('source.templates.search')}
              className="pl-8"
            />
          </div>
          <ul
            aria-label={t('source.templates.categories')}
            className="flex flex-wrap gap-1.5"
          >
            {categories.map((c) => (
              <li key={c}>
                <button
                  type="button"
                  aria-pressed={category === c}
                  onClick={() => {
                    setCategory(category === c ? null : c)
                  }}
                  className={cn(
                    'rounded-full border border-border px-2.5 py-0.5 text-xs text-muted-foreground transition-colors hover:bg-muted focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none',
                    category === c &&
                      'border-primary/50 bg-muted font-medium text-foreground',
                  )}
                >
                  {c}
                </button>
              </li>
            ))}
          </ul>
        </>
      ) : null}
      {isPending ? (
        <div
          role="status"
          aria-label={t('source.templates.loading')}
          className="grid grid-cols-2 gap-2 sm:grid-cols-4 xl:grid-cols-2"
        >
          {Array.from({ length: FEATURED_COUNT }, (_, i) => (
            <Skeleton key={i} className="h-14 w-full" />
          ))}
        </div>
      ) : isError || featured.length === 0 ? (
        <div className="flex flex-wrap items-center gap-2 rounded-md border border-dashed border-border p-3">
          <p className="min-w-0 flex-1 text-sm text-muted-foreground">
            {t('source.templates.unavailable')}
          </p>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => {
              void refetch()
            }}
          >
            {t('source.templates.retry')}
          </Button>
        </div>
      ) : shown.length === 0 ? (
        <p className="rounded-md border border-dashed border-border p-3 text-sm text-muted-foreground">
          {t('source.templates.noMatch', { query: query.trim() })}
        </p>
      ) : (
        <ul
          aria-label={t('source.templates.gridLabel')}
          className="grid grid-cols-2 gap-2 sm:grid-cols-4 xl:grid-cols-2"
        >
          {shown.map((tpl) => (
            <li key={tpl.id}>
              <Link
                to="/templates/$id"
                params={{ id: tpl.id }}
                title={tpl.name}
                className="flex h-full items-center gap-2 rounded-lg border border-border p-2 transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
              >
                <TemplateLogoTile id={tpl.id} name={tpl.name} size="sm" />
                <span className="min-w-0">
                  <span className="line-clamp-2 text-sm leading-tight font-medium text-foreground">
                    {tpl.name}
                  </span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {tpl.category}
                  </span>
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
      <StarterTemplatesDialog variant="row" />
    </section>
  )
}
