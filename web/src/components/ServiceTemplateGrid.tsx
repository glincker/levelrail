import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  AppWindowGlassIcon,
  ChartColumnGlassIcon,
  ChartLineGlassIcon,
  CirclePlayGlassIcon,
  CodeGlassIcon,
  CoinsGlassIcon,
  DatabaseGlassIcon,
  LayersGlassIcon,
  LayoutGridGlassIcon,
  MessageCircleGlassIcon,
  RocketGlassIcon,
  SearchGlassIcon,
  ServerGlassIcon,
  ShieldCheckGlassIcon,
  SparklesGlassIcon,
  SquareCheckGlassIcon,
  TriangleAlertGlassIcon,
  WifiHighGlassIcon,
  ZapGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { TemplateLogoTile } from './TemplateLogoTile'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { formatBytes } from '../lib/format'
import {
  useServiceTemplates,
  type ServiceTemplateListItem,
} from '../queries/serviceTemplates'

// internal/catalog/catalog.go's current Category values. Unlisted or
// future categories fall back to PackageIcon. Shared with the /templates
// routes so the fallback icon matches whichever this grid last showed.
export const CATEGORY_ICONS: Record<string, UiIcon> = {
  AI: SparklesGlassIcon,
  Analytics: ChartColumnGlassIcon,
  Applications: AppWindowGlassIcon,
  Automation: ZapGlassIcon,
  Communication: MessageCircleGlassIcon,
  Dashboard: LayoutGridGlassIcon,
  'Database Tools': DatabaseGlassIcon,
  'Developer Tools': CodeGlassIcon,
  Finance: CoinsGlassIcon,
  Infrastructure: LayersGlassIcon,
  IoT: WifiHighGlassIcon,
  Media: CirclePlayGlassIcon,
  Monitoring: ChartLineGlassIcon,
  Productivity: SquareCheckGlassIcon,
  Security: ShieldCheckGlassIcon,
  Storage: ServerGlassIcon,
  'Starter Kits': RocketGlassIcon,
}

function matchesSearch(
  template: ServiceTemplateListItem,
  query: string,
): boolean {
  if (!query) return true
  return (
    template.name.toLowerCase().includes(query) ||
    template.slogan.toLowerCase().includes(query) ||
    template.category.toLowerCase().includes(query)
  )
}

// A single template card, styled after CreateResourceWizard's own
// OptionCard so both pickers read as one family of controls. The name/
// logo header is its own Link to the standalone /templates/$id page
// (shareable outside the wizard); the rest of the card body stays a
// button that fires onSelect, the in-wizard preview/configure step, and
// "Deploy now" is a third, separate action below. Three sibling controls
// rather than nested ones, since a Link can't sit inside a button.
function TemplateCard({
  template,
  onSelect,
  onDeployNow,
  deploying,
}: {
  template: ServiceTemplateListItem
  onSelect: (id: string) => void
  onDeployNow: (template: ServiceTemplateListItem) => void
  deploying: boolean
}) {
  const { t } = useTranslation('templates')
  const ram = template.recommended_memory_bytes
    ? t('card.ramRecommended', {
        size: formatBytes(template.recommended_memory_bytes),
      })
    : ''
  return (
    <div className="flex h-full min-w-0 flex-col items-start gap-2 rounded-lg border border-border bg-card p-3 text-left transition-colors hover:border-primary/40 hover:bg-muted">
      <Link
        to="/templates/$id"
        params={{ id: template.id }}
        title={template.name}
        className="flex w-full min-w-0 items-start gap-2 rounded-md focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <TemplateLogoTile id={template.id} name={template.name} />
        <span className="line-clamp-2 min-w-0 text-sm font-medium break-words text-foreground hover:underline">
          {template.name}
        </span>
      </Link>
      <button
        type="button"
        onClick={() => {
          onSelect(template.id)
        }}
        className="flex min-w-0 w-full flex-1 flex-col items-start gap-2 rounded-md text-left focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <Badge variant="outline">{template.category}</Badge>
        <span className="text-xs break-words text-muted-foreground">
          {template.slogan}
        </span>
        {ram ? (
          <Badge
            variant="muted"
            title={ram}
            className="h-auto max-w-full text-xs whitespace-normal"
          >
            {ram}
          </Badge>
        ) : null}
        {template.requires_gpu && (
          <Badge
            variant="muted"
            className="h-auto max-w-full text-xs whitespace-normal"
          >
            {t('card.needsGpu')}
          </Badge>
        )}
      </button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="h-auto min-h-7 w-full shrink-0 py-1 text-center whitespace-normal"
        disabled={deploying}
        onClick={() => {
          onDeployNow(template)
        }}
      >
        <RocketGlassIcon size="1.2em" />
        {deploying
          ? t('card.deploying')
          : template.requires_configuration
            ? t('card.deployNowSetup')
            : t('card.deployNow')}
      </Button>
    </div>
  )
}

// Searchable, category-grouped grid over GET /api/v1/service-templates
// (queries/serviceTemplates.ts, internal/catalog's curated catalog, ADR
// 015). Self-contained (owns its own fetch and search state) so both
// BrowseTemplatesFields' in-wizard step and the standalone /templates
// catalog route render the exact same grid, with only the click/deploy
// callbacks differing per caller.
export function ServiceTemplateGrid({
  onSelect,
  onDeployNow,
  deployingId,
}: {
  onSelect: (id: string) => void
  onDeployNow: (template: ServiceTemplateListItem) => void
  deployingId?: string
}) {
  const { t } = useTranslation('templates')
  const [search, setSearch] = useState('')
  const templatesQuery = useServiceTemplates()

  const templates = templatesQuery.data ?? []
  const normalizedSearch = search.trim().toLowerCase()
  const filtered = templates.filter((template) =>
    matchesSearch(template, normalizedSearch),
  )
  const categories: string[] = []
  for (const template of filtered) {
    if (!categories.includes(template.category)) {
      categories.push(template.category)
    }
  }

  return (
    <div className="space-y-4">
      <div className="relative">
        <SearchGlassIcon
          className="pointer-events-none absolute top-1/2 left-2.5 size-4.5 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
          }}
          placeholder={t('card.searchPlaceholder')}
          aria-label={t('card.searchLabel')}
          className="pl-8"
        />
      </div>
      {templatesQuery.isLoading ? (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-5">
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton key={index} className="h-24 w-full" />
          ))}
        </div>
      ) : templatesQuery.isError ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.2em" />
          <AlertDescription>{templatesQuery.error.message}</AlertDescription>
        </Alert>
      ) : (
        <div className="space-y-4">
          {categories.map((category) => (
            <div key={category} className="space-y-2">
              <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                {category}
              </h3>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4 xl:grid-cols-5">
                {filtered
                  .filter((template) => template.category === category)
                  .map((template) => (
                    <TemplateCard
                      key={template.id}
                      template={template}
                      onSelect={onSelect}
                      onDeployNow={onDeployNow}
                      deploying={deployingId === template.id}
                    />
                  ))}
              </div>
            </div>
          ))}
          {filtered.length === 0 ? (
            <p className="py-6 text-center text-sm text-muted-foreground">
              {t('card.noMatch', { search })}
            </p>
          ) : null}
        </div>
      )}
    </div>
  )
}
