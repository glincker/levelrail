import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  AppWindowIcon,
  ChartBarIcon,
  ChartLineIcon,
  ChatCircleIcon,
  CheckSquareIcon,
  CodeIcon,
  CoinsIcon,
  DatabaseIcon,
  HardDrivesIcon,
  LightningIcon,
  MagnifyingGlassIcon,
  PackageIcon,
  PlayCircleIcon,
  RocketLaunchIcon,
  ShieldCheckIcon,
  SparkleIcon,
  SquaresFourIcon,
  StackIcon,
  WarningIcon,
  WifiHighIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { TemplateLogo } from './TemplateLogo'
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
export const CATEGORY_ICONS: Record<string, Icon> = {
  AI: SparkleIcon,
  Analytics: ChartBarIcon,
  Applications: AppWindowIcon,
  Automation: LightningIcon,
  Communication: ChatCircleIcon,
  Dashboard: SquaresFourIcon,
  'Database Tools': DatabaseIcon,
  'Developer Tools': CodeIcon,
  Finance: CoinsIcon,
  Infrastructure: StackIcon,
  IoT: WifiHighIcon,
  Media: PlayCircleIcon,
  Monitoring: ChartLineIcon,
  Productivity: CheckSquareIcon,
  Security: ShieldCheckIcon,
  Storage: HardDrivesIcon,
  'Starter Kits': RocketLaunchIcon,
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
  const CategoryIcon = CATEGORY_ICONS[template.category] ?? PackageIcon
  return (
    <div className="flex flex-col items-start gap-2 rounded-lg border border-border bg-card p-3 text-left transition-colors hover:border-primary/40 hover:bg-muted">
      <Link
        to="/templates/$id"
        params={{ id: template.id }}
        className="flex w-full items-center gap-2 rounded-md focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <TemplateLogo
          id={template.id}
          className="size-6"
          fallback={<CategoryIcon className="size-6 text-muted-foreground" />}
        />
        <span className="truncate text-sm font-medium text-foreground hover:underline">
          {template.name}
        </span>
      </Link>
      <button
        type="button"
        onClick={() => {
          onSelect(template.id)
        }}
        className="flex w-full flex-col items-start gap-2 rounded-md text-left focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <Badge variant="outline">{template.category}</Badge>
        <span className="text-xs text-muted-foreground">{template.slogan}</span>
        {!!template.recommended_memory_bytes && (
          <Badge variant="muted" className="text-xs">
            ~{formatBytes(template.recommended_memory_bytes)} RAM recommended
          </Badge>
        )}
        {template.requires_gpu && (
          <Badge variant="muted" className="text-xs">
            Needs NVIDIA GPU
          </Badge>
        )}
      </button>
      <Button
        type="button"
        size="sm"
        variant="outline"
        className="w-full"
        disabled={deploying}
        onClick={() => {
          onDeployNow(template)
        }}
      >
        <RocketLaunchIcon />
        {deploying
          ? 'Deploying...'
          : template.requires_configuration
            ? 'Deploy now (needs setup)'
            : 'Deploy now'}
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
        <MagnifyingGlassIcon
          className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          value={search}
          onChange={(e) => {
            setSearch(e.target.value)
          }}
          placeholder="Search templates..."
          aria-label="Search templates"
          className="pl-8"
        />
      </div>
      {templatesQuery.isLoading ? (
        <div className="grid grid-cols-2 gap-3">
          {Array.from({ length: 6 }).map((_, index) => (
            <Skeleton key={index} className="h-24 w-full" />
          ))}
        </div>
      ) : templatesQuery.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{templatesQuery.error.message}</AlertDescription>
        </Alert>
      ) : (
        <div className="space-y-4">
          {categories.map((category) => (
            <div key={category} className="space-y-2">
              <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                {category}
              </h3>
              <div className="grid grid-cols-2 gap-3">
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
              No templates match &ldquo;{search}&rdquo;.
            </p>
          ) : null}
        </div>
      )}
    </div>
  )
}
