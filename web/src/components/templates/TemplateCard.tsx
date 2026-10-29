import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  DatabaseGlassIcon,
  RocketGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { formatBytes } from '../../lib/format'
import { memoryTier } from '../../lib/templateCatalogView'
import { categoryStyle } from '../../lib/templateCategoryStyle'
import { STARTER_STACKS } from '../../lib/templateStacks'
import type { ServiceTemplateListItem } from '../../queries/serviceTemplates'
import { DeleteCustomTemplateDialog } from '../DeleteCustomTemplateDialog'
import { TemplateLogoTile } from '../TemplateLogoTile'
import { TemplateVerifiedBadge } from './TemplateVerifiedBadge'

export interface TemplateCardProps {
  template: ServiceTemplateListItem
  onSelect: (id: string) => void
  onDeployNow: (template: ServiceTemplateListItem) => void
  deploying: boolean
  isCustom: boolean
}

function MemoryChip({ bytes }: { bytes: number }) {
  const { t } = useTranslation('templates')
  const tier = memoryTier(bytes)
  const size = formatBytes(bytes)
  return (
    <Badge
      variant="muted"
      className="gap-1.5 text-xs"
      title={t('marketplace.card.ramLabel', { size })}
    >
      <span aria-hidden="true" className="flex items-end gap-px">
        {[1, 2, 3].map((bar) => (
          <span
            key={bar}
            className={cn(
              'w-0.5 rounded-sm bg-current',
              bar === 1 ? 'h-1' : bar === 2 ? 'h-1.5' : 'h-2',
              bar > tier && 'opacity-25',
            )}
          />
        ))}
      </span>
      {t('marketplace.card.ram', { size })}
    </Badge>
  )
}

function DeployButton({
  template,
  onDeployNow,
  deploying,
  className,
}: Pick<TemplateCardProps, 'template' | 'onDeployNow' | 'deploying'> & {
  className?: string
}) {
  const { t } = useTranslation('templates')
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      disabled={deploying}
      className={cn(
        'text-foreground/80 group-focus-within/card:border-primary group-focus-within/card:bg-primary group-focus-within/card:text-primary-foreground group-hover/card:border-primary group-hover/card:bg-primary group-hover/card:text-primary-foreground',
        className,
      )}
      onClick={() => {
        onDeployNow(template)
      }}
    >
      <RocketGlassIcon size="1.4em" aria-hidden="true" />
      {deploying
        ? t('marketplace.card.deploying')
        : template.requires_configuration
          ? t('marketplace.card.deployNeedsSetup')
          : t('marketplace.card.deploy')}
    </Button>
  )
}

function Chips({ template }: { template: ServiceTemplateListItem }) {
  const { t } = useTranslation('templates')
  const services = STARTER_STACKS[template.id]?.services ?? 0
  return (
    <div className="flex flex-wrap items-center gap-2">
      {template.verified && <TemplateVerifiedBadge className="text-xs" />}
      {services > 1 && (
        <Badge variant="outline" className="text-xs">
          {t('marketplace.card.services', { count: services })}
        </Badge>
      )}
      {!!template.recommended_memory_bytes && (
        <MemoryChip bytes={template.recommended_memory_bytes} />
      )}
      {template.needs_database && (
        <Badge variant="muted" className="gap-1 text-xs">
          <DatabaseGlassIcon className="size-4.5" aria-hidden="true" />
          {t('marketplace.card.database')}
        </Badge>
      )}
      {template.has_volume && (
        <Badge variant="muted" className="gap-1 text-xs">
          <ServerGlassIcon className="size-4.5" aria-hidden="true" />
          {t('marketplace.card.volume')}
        </Badge>
      )}
      {template.requires_gpu && (
        <Badge variant="muted" className="text-xs">
          {t('marketplace.card.gpu')}
        </Badge>
      )}
    </div>
  )
}

const CARD_SHELL =
  'group/card relative flex rounded-xl border border-border bg-card transition-[transform,opacity,border-color] motion-safe:hover:-translate-y-0.5 hover:border-primary/50 focus-within:border-primary/50'

/** TemplateCard is one catalog entry as a card (comfortable) or a row (compact). */
export function TemplateCard(props: TemplateCardProps & { compact?: boolean }) {
  const { template, onSelect, isCustom, compact = false } = props
  const category = categoryStyle(template.category)
  const CategoryIcon = category.icon
  const focus =
    'focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50'

  const heading = (
    <Link
      to="/templates/$id"
      params={{ id: template.id }}
      className={cn('flex min-w-0 items-center gap-3 rounded-md', focus)}
    >
      <span className="rounded-xl bg-muted/70 p-1">
        <TemplateLogoTile id={template.id} name={template.name} size="xl" />
      </span>
      <span className="min-w-0">
        <span className="block truncate text-base font-semibold text-foreground">
          {template.name}
        </span>
        <span
          className={cn(
            'flex items-center gap-1 text-xs font-medium',
            category.toneClass,
          )}
        >
          <CategoryIcon className="size-4.5" aria-hidden="true" />
          {template.category}
        </span>
      </span>
    </Link>
  )

  if (compact) {
    return (
      <div className={cn(CARD_SHELL, 'items-center gap-4 px-3 py-2')}>
        <div className="w-64 shrink-0 max-md:w-auto max-md:min-w-0 max-md:flex-1">
          {heading}
        </div>
        <button
          type="button"
          onClick={() => {
            onSelect(template.id)
          }}
          title={template.slogan}
          className={cn(
            'hidden min-w-0 flex-1 rounded-md text-left text-sm text-foreground/70 md:block',
            focus,
          )}
        >
          <span className="line-clamp-1">{template.slogan}</span>
        </button>
        <div className="hidden shrink-0 sm:block">
          <Chips template={template} />
        </div>
        {isCustom ? (
          <DeleteCustomTemplateDialog id={template.id} name={template.name} />
        ) : null}
        <DeployButton {...props} className="shrink-0" />
      </div>
    )
  }

  return (
    <div className={cn(CARD_SHELL, 'h-full flex-col gap-3 p-4')}>
      <div className="flex items-start gap-2">
        <div className="min-w-0 flex-1">{heading}</div>
        {isCustom ? (
          <DeleteCustomTemplateDialog id={template.id} name={template.name} />
        ) : null}
      </div>
      <button
        type="button"
        onClick={() => {
          onSelect(template.id)
        }}
        title={template.slogan}
        className={cn('flex flex-1 flex-col gap-3 rounded-md text-left', focus)}
      >
        <span className="line-clamp-2 text-sm leading-5 text-foreground/70">
          {template.slogan}
        </span>
        <Chips template={template} />
      </button>
      <DeployButton {...props} className="w-full" />
    </div>
  )
}
