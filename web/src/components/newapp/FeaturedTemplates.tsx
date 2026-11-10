import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { ArrowRightGlassIcon, SearchGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  useServiceTemplates,
  type ServiceTemplateListItem,
} from '../../queries/serviceTemplates'
import { pickFeatured } from '../../lib/featuredTemplates'
import { TEMPLATE_LOGO_LOADERS } from '../../lib/templateLogos'
import { TemplateLogoTile } from '../TemplateLogoTile'
import { SectionHeading } from './SectionHeading'
import { StarterTemplatesDialog } from './StarterTemplatesDialog'

// Six chips fit one or two rows without scrolling.
const POPULAR_COUNT = 6

function matches(tpl: ServiceTemplateListItem, q: string): boolean {
  return (
    tpl.name.toLowerCase().includes(q) ||
    tpl.id.toLowerCase().includes(q) ||
    tpl.category.toLowerCase().includes(q)
  )
}

/** A compact row of popular template chips with a search that reaches the whole catalog. */
export function FeaturedTemplates() {
  const { t } = useTranslation('newApp')
  const { data, isPending, isError, refetch } = useServiceTemplates()
  const [query, setQuery] = useState('')
  const list = useMemo(() => (Array.isArray(data) ? data : []), [data])
  const featured = useMemo(
    () => pickFeatured(list).slice(0, POPULAR_COUNT),
    [list],
  )
  const q = query.trim().toLowerCase()
  const filtering = q !== ''
  const shown = useMemo(() => {
    if (!filtering) return featured
    return list
      .filter((tpl) => matches(tpl, q))
      .sort(
        (a, b) =>
          Number(b.id in TEMPLATE_LOGO_LOADERS) -
          Number(a.id in TEMPLATE_LOGO_LOADERS),
      )
      .slice(0, POPULAR_COUNT)
  }, [filtering, featured, list, q])

  return (
    <section aria-labelledby="new-app-templates" className="space-y-3">
      <SectionHeading
        id="new-app-templates"
        action={
          <Link
            to="/templates"
            className="inline-flex items-center gap-1 text-sm font-medium text-foreground underline-offset-2 hover:underline"
          >
            {list.length > 0
              ? t('source.templates.browseAll', { count: list.length })
              : t('source.templates.browse')}
            <ArrowRightGlassIcon className="size-5" aria-hidden="true" />
          </Link>
        }
      >
        {t('source.templates.popular')}
      </SectionHeading>
      {list.length > 0 ? (
        <div className="relative">
          <SearchGlassIcon
            className="pointer-events-none absolute top-1/2 left-3 size-5 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            value={query}
            onChange={(e) => {
              setQuery(e.target.value)
            }}
            placeholder={t('source.templates.search')}
            aria-label={t('source.templates.search')}
            className="h-11 pl-10 text-base"
          />
        </div>
      ) : null}
      {isPending ? (
        <div
          role="status"
          aria-label={t('source.templates.loading')}
          className="flex flex-wrap gap-2"
        >
          {Array.from({ length: POPULAR_COUNT }, (_, i) => (
            <Skeleton key={i} className="h-10 w-32" />
          ))}
        </div>
      ) : isError || featured.length === 0 ? (
        <div className="flex flex-wrap items-center gap-2 rounded-element border border-dashed border-border p-3">
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
        <p className="rounded-element border border-dashed border-border p-3 text-sm text-muted-foreground">
          {t('source.templates.noMatch', { query: query.trim() })}
        </p>
      ) : (
        <ul
          aria-label={t('source.templates.gridLabel')}
          className="flex flex-wrap gap-2"
        >
          {shown.map((tpl) => (
            <li key={tpl.id}>
              <Link
                to="/templates/$id"
                params={{ id: tpl.id }}
                title={tpl.category}
                className="flex items-center gap-2 rounded-full border border-border py-1 pr-3 pl-1 text-sm font-medium text-foreground transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
              >
                <TemplateLogoTile id={tpl.id} name={tpl.name} size="sm" />
                {tpl.name}
              </Link>
            </li>
          ))}
        </ul>
      )}
      <StarterTemplatesDialog variant="row" />
    </section>
  )
}
