import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon, RocketGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { formatBytes } from '../../lib/format'
import { STARTER_STACKS } from '../../lib/templateStacks'
import type { ServiceTemplateListItem } from '../../queries/serviceTemplates'
import { TemplateLogoTile } from '../TemplateLogoTile'

/** TemplateFeaturedStrip is the collapsible hero band of starter kits above the catalog. */
export function TemplateFeaturedStrip({
  templates,
  onSelect,
  onDeployNow,
  deployingId,
}: {
  templates: ServiceTemplateListItem[]
  onSelect: (id: string) => void
  onDeployNow: (template: ServiceTemplateListItem) => void
  deployingId?: string
}) {
  const { t } = useTranslation('templates')
  const [open, setOpen] = useState(true)
  if (templates.length === 0) return null
  return (
    <section
      aria-labelledby="template-featured"
      className="rounded-2xl border border-border bg-muted/40 p-4"
    >
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2
            id="template-featured"
            className="text-base font-semibold text-foreground"
          >
            {t('marketplace.featured.title')}
          </h2>
          <p className="text-sm text-foreground/70">
            {t('marketplace.featured.subtitle')}
          </p>
        </div>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          aria-expanded={open}
          aria-controls="template-featured-list"
          onClick={() => {
            setOpen((v) => !v)
          }}
        >
          <ChevronDownGlassIcon
            size="1.2em"
            aria-hidden="true"
            className={cn('transition-transform', !open && '-rotate-90')}
          />
          {open
            ? t('marketplace.featured.hide')
            : t('marketplace.featured.show')}
        </Button>
      </div>
      <ul
        id="template-featured-list"
        hidden={!open}
        className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3"
      >
        {templates.map((template) => {
          const stack = STARTER_STACKS[template.id]
          const deploying = deployingId === template.id
          return (
            <li
              key={template.id}
              className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 transition-[transform,border-color] motion-safe:hover:-translate-y-0.5 hover:border-primary/50"
            >
              <Link
                to="/templates/$id"
                params={{ id: template.id }}
                className="flex items-center gap-3 rounded-md focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
              >
                <TemplateLogoTile
                  id={template.id}
                  name={template.name}
                  size="xl"
                />
                <span className="min-w-0">
                  <span className="block text-base font-semibold text-foreground">
                    {template.name}
                  </span>
                  {stack ? (
                    <span className="block text-sm text-foreground/70">
                      {stack.labels.join(', ')}
                    </span>
                  ) : null}
                </span>
              </Link>
              <button
                type="button"
                onClick={() => {
                  onSelect(template.id)
                }}
                title={template.slogan}
                className="flex-1 rounded-md text-left focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
              >
                <span className="line-clamp-2 text-sm leading-5 text-foreground/70">
                  {template.slogan}
                </span>
              </button>
              <div className="flex items-center justify-between gap-2">
                {template.recommended_memory_bytes ? (
                  <Badge variant="muted" className="text-xs">
                    {t('marketplace.card.ram', {
                      size: formatBytes(template.recommended_memory_bytes),
                    })}
                  </Badge>
                ) : (
                  <span />
                )}
                <Button
                  type="button"
                  size="sm"
                  disabled={deploying}
                  onClick={() => {
                    onDeployNow(template)
                  }}
                >
                  <RocketGlassIcon size="1.2em" aria-hidden="true" />
                  {deploying
                    ? t('marketplace.card.deploying')
                    : t('marketplace.card.deploy')}
                </Button>
              </div>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
