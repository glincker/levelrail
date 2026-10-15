import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useServiceTemplates } from '../../queries/serviceTemplates'
import { TemplateLogoTile } from '../TemplateLogoTile'

const RELATED_COUNT = 4

/** Other templates from the same category, linking to their detail pages. */
export function RelatedTemplates({
  id,
  category,
}: {
  id: string
  category: string
}) {
  const { t } = useTranslation('templates')
  const { data } = useServiceTemplates()
  const related = (Array.isArray(data) ? data : [])
    .filter((tpl) => tpl.category === category && tpl.id !== id)
    .slice(0, RELATED_COUNT)
  if (related.length === 0) return null
  return (
    <section aria-labelledby="template-related" className="space-y-3">
      <h2 id="template-related" className="text-base font-medium">
        {t('detail.related.title', { category })}
      </h2>
      <ul className="grid gap-2 sm:grid-cols-2">
        {related.map((tpl) => (
          <li key={tpl.id}>
            <Link
              to="/templates/$id"
              params={{ id: tpl.id }}
              className="flex h-full items-center gap-2.5 rounded-lg border border-border bg-card p-3 transition-colors hover:bg-muted/50 focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
            >
              <TemplateLogoTile id={tpl.id} name={tpl.name} size="md" />
              <span className="min-w-0">
                <span className="block truncate text-sm font-medium">
                  {tpl.name}
                </span>
                <span className="block truncate text-xs text-muted-foreground">
                  {tpl.slogan}
                </span>
              </span>
            </Link>
          </li>
        ))}
      </ul>
    </section>
  )
}
