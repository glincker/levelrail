import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useServiceTemplates } from '../../queries/serviceTemplates'
import { pickFeatured } from '../../lib/featuredTemplates'
import { TemplateLogoTile } from '../TemplateLogoTile'

const COUNT = 8

/** Wide-screen filler under the repo field: real popular templates, same-height tiles. */
export function PopularTemplatesRow() {
  const { t } = useTranslation('newApp')
  const { data } = useServiceTemplates()
  const featured = useMemo(
    () => pickFeatured(Array.isArray(data) ? data : []).slice(0, COUNT),
    [data],
  )
  if (featured.length === 0) return null
  return (
    <section
      aria-labelledby="new-app-popular"
      className="hidden space-y-2 min-[80rem]:block"
    >
      <h2
        id="new-app-popular"
        className="text-sm font-medium text-muted-foreground"
      >
        {t('source.templates.popular')}
      </h2>
      <ul className="grid grid-cols-4 gap-2">
        {featured.map((tpl) => (
          <li key={tpl.id}>
            <Link
              to="/templates/$id"
              params={{ id: tpl.id }}
              title={tpl.category}
              className="flex h-full items-center gap-2 rounded-container border border-border bg-card p-2 text-sm font-medium text-foreground shadow-card transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none motion-reduce:transition-none"
            >
              <TemplateLogoTile id={tpl.id} name={tpl.name} size="sm" />
              <span className="min-w-0 truncate">{tpl.name}</span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  )
}
