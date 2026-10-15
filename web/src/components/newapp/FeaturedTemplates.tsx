import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { ArrowRightIcon } from '@phosphor-icons/react/dist/ssr'
import { Skeleton } from '@/components/ui/skeleton'
import { useServiceTemplates } from '../../queries/serviceTemplates'
import { FEATURED_COUNT, pickFeatured } from '../../lib/featuredTemplates'
import { TemplateLogoTile } from '../TemplateLogoTile'

/** Featured template grid for the new app page, linking to each template's detail page. */
export function FeaturedTemplates() {
  const { t } = useTranslation('newApp')
  const { data, isPending, isError } = useServiceTemplates()
  const list = Array.isArray(data) ? data : []
  const featured = pickFeatured(list)

  return (
    <section aria-labelledby="new-app-templates" className="space-y-3">
      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <h2
            id="new-app-templates"
            className="text-base font-medium text-foreground"
          >
            {t('source.templates.title')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t('source.templates.description')}
          </p>
        </div>
        <Link
          to="/templates"
          className="inline-flex items-center gap-1 text-sm font-medium text-foreground underline-offset-2 hover:underline"
        >
          {list.length > 0
            ? t('source.templates.seeAll', { count: list.length })
            : t('source.templates.browse')}
          <ArrowRightIcon className="size-3.5" aria-hidden="true" />
        </Link>
      </div>
      {isPending ? (
        <div
          role="status"
          aria-label={t('source.templates.loading')}
          className="grid grid-cols-2 gap-2 sm:grid-cols-4"
        >
          {Array.from({ length: FEATURED_COUNT }, (_, i) => (
            <Skeleton key={i} className="h-16 w-full" />
          ))}
        </div>
      ) : isError || featured.length === 0 ? (
        <p className="rounded-md border border-dashed border-border p-4 text-sm text-muted-foreground">
          {t('source.templates.unavailable')}
        </p>
      ) : (
        <ul
          aria-label={t('source.templates.gridLabel')}
          className="grid grid-cols-2 gap-2 sm:grid-cols-4"
        >
          {featured.map((tpl) => (
            <li key={tpl.id}>
              <Link
                to="/templates/$id"
                params={{ id: tpl.id }}
                className="flex h-full items-center gap-2.5 rounded-lg border border-border bg-card p-3 transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
              >
                <TemplateLogoTile id={tpl.id} name={tpl.name} size="md" />
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium text-foreground">
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
    </section>
  )
}
