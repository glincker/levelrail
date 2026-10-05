import { useEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Link } from '@tanstack/react-router'
import {
  MagnifyingGlassIcon,
  PackageIcon,
  RocketLaunchIcon,
  SquaresFourIcon,
  UserIcon,
  WarningIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { CATEGORY_ICONS } from './ServiceTemplateGrid'
import { TemplateLogo } from './TemplateLogo'
import { DeleteCustomTemplateDialog } from './DeleteCustomTemplateDialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { formatBytes } from '../lib/format'
import {
  useServiceTemplates,
  type ServiceTemplateListItem,
} from '../queries/serviceTemplates'
import { useCustomTemplates } from '../queries/customTemplates'

// Sentinel for "no category filter applied", distinct from any real
// catalog.go Category value so it can never collide with one.
const ALL_CATEGORIES = 'All'

// Synthetic category for operator-defined templates; never a real
// catalog.Template.Category string, so the two can't collide.
const YOUR_TEMPLATES_CATEGORY = 'Your templates'

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

// Card min-width drives the breakpoints: wide enough that a logo, name,
// category badge and two action buttons never wrap awkwardly.
function columnsForWidth(width: number): number {
  if (width >= 1360) return 4
  if (width >= 1024) return 3
  if (width >= 640) return 2
  return 1
}

function useGridColumns(ref: React.RefObject<HTMLDivElement | null>): number {
  const [cols, setCols] = useState(1)
  useEffect(() => {
    const el = ref.current
    if (!el || typeof ResizeObserver === 'undefined') return undefined
    const ro = new ResizeObserver(([entry]) => {
      if (entry) setCols(columnsForWidth(entry.contentRect.width))
    })
    ro.observe(el)
    return () => {
      ro.disconnect()
    }
  }, [ref])
  return cols
}

function CategoryItem({
  icon: ItemIcon,
  label,
  count,
  active,
  onClick,
}: {
  icon: Icon
  label: string
  count: number
  active: boolean
  onClick: () => void
}) {
  return (
    <li>
      <button
        type="button"
        onClick={onClick}
        aria-pressed={active}
        className={cn(
          'flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
          active
            ? 'bg-primary/10 font-medium text-primary'
            : 'text-muted-foreground hover:bg-muted hover:text-foreground',
        )}
      >
        <ItemIcon className="size-4 shrink-0" aria-hidden="true" />
        <span className="min-w-0 flex-1 truncate">{label}</span>
        <span className="text-xs tabular-nums text-muted-foreground">
          {count}
        </span>
      </button>
    </li>
  )
}

// Desktop-only vertical filter rail. Hidden below md, where CategoryChips
// below takes over, so the same active/onChange state drives whichever
// one the viewport shows.
function CategorySidebar({
  categoryCounts,
  total,
  active,
  onChange,
}: {
  categoryCounts: [string, number][]
  total: number
  active: string
  onChange: (category: string) => void
}) {
  return (
    <nav
      aria-label="Template categories"
      className="hidden md:sticky md:top-4 md:block md:self-start"
    >
      <p className="px-2 pb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        Categories
      </p>
      <ul className="space-y-0.5">
        <CategoryItem
          icon={SquaresFourIcon}
          label="All templates"
          count={total}
          active={active === ALL_CATEGORIES}
          onClick={() => {
            onChange(ALL_CATEGORIES)
          }}
        />
        {categoryCounts.map(([category, count]) => (
          <CategoryItem
            key={category}
            icon={CATEGORY_ICONS[category] ?? PackageIcon}
            label={category}
            count={count}
            active={active === category}
            onClick={() => {
              onChange(category)
            }}
          />
        ))}
      </ul>
    </nav>
  )
}

function CategoryChip({
  label,
  active,
  onClick,
}: {
  label: string
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        'shrink-0 rounded-full border px-3 py-1 text-xs font-medium whitespace-nowrap transition-colors focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
        active
          ? 'border-primary bg-primary/10 text-primary'
          : 'border-border bg-card text-muted-foreground hover:text-foreground',
      )}
    >
      {label}
    </button>
  )
}

// Mobile/narrow equivalent of CategorySidebar: a horizontally scrollable
// chip row instead of a column that would otherwise eat half the width.
function CategoryChips({
  categoryCounts,
  total,
  active,
  onChange,
}: {
  categoryCounts: [string, number][]
  total: number
  active: string
  onChange: (category: string) => void
}) {
  return (
    <div
      role="group"
      aria-label="Filter templates by category"
      className="flex gap-2 overflow-x-auto pb-1 md:hidden"
    >
      <CategoryChip
        label={`All (${total})`}
        active={active === ALL_CATEGORIES}
        onClick={() => {
          onChange(ALL_CATEGORIES)
        }}
      />
      {categoryCounts.map(([category, count]) => (
        <CategoryChip
          key={category}
          label={`${category} (${count})`}
          active={active === category}
          onClick={() => {
            onChange(category)
          }}
        />
      ))}
    </div>
  )
}

// Richer sibling of ServiceTemplateGrid's TemplateCard: same three
// sibling controls (name/logo link, select button, deploy button) so
// click semantics stay identical, bigger logo and wrapping RAM/GPU
// badges so a 260px+ card reads as a real marketplace listing.
function MarketplaceCard({
  template,
  onSelect,
  onDeployNow,
  deploying,
  isCustom,
}: {
  template: ServiceTemplateListItem
  onSelect: (id: string) => void
  onDeployNow: (template: ServiceTemplateListItem) => void
  deploying: boolean
  isCustom: boolean
}) {
  const CategoryIcon = isCustom
    ? UserIcon
    : (CATEGORY_ICONS[template.category] ?? PackageIcon)
  return (
    <div className="flex h-full flex-col gap-3 rounded-xl border border-border bg-card p-4 transition-colors hover:border-primary/40 hover:shadow-sm">
      <div className="flex items-start gap-3">
        <Link
          to="/templates/$id"
          params={{ id: template.id }}
          className="flex min-w-0 flex-1 items-start gap-3 rounded-md focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
        >
          <TemplateLogo
            id={template.id}
            className="size-10 shrink-0 rounded-md"
            fallback={
              <span className="flex size-10 shrink-0 items-center justify-center rounded-md bg-muted">
                <CategoryIcon
                  className="size-5 text-muted-foreground"
                  aria-hidden="true"
                />
              </span>
            }
          />
          <div className="min-w-0 flex-1 pt-0.5">
            <p className="line-clamp-1 text-sm font-semibold text-foreground hover:underline">
              {template.name}
            </p>
            <Badge variant="outline" className="mt-1 text-[11px]">
              {template.category}
            </Badge>
          </div>
        </Link>
        {isCustom ? (
          <DeleteCustomTemplateDialog id={template.id} name={template.name} />
        ) : null}
      </div>
      <button
        type="button"
        onClick={() => {
          onSelect(template.id)
        }}
        className="flex flex-1 flex-col gap-2 rounded-md text-left focus-visible:outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <p className="line-clamp-2 text-xs text-muted-foreground">
          {template.slogan}
        </p>
        {template.recommended_memory_bytes || template.requires_gpu ? (
          <div className="flex flex-wrap items-center gap-1.5">
            {!!template.recommended_memory_bytes && (
              <Badge variant="muted" className="text-[11px]">
                ~{formatBytes(template.recommended_memory_bytes)} RAM
              </Badge>
            )}
            {template.requires_gpu && (
              <Badge variant="muted" className="text-[11px]">
                GPU required
              </Badge>
            )}
          </div>
        ) : null}
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
        <RocketLaunchIcon aria-hidden="true" />
        {deploying
          ? 'Deploying...'
          : template.requires_configuration
            ? 'Deploy now (needs setup)'
            : 'Deploy now'}
      </Button>
    </div>
  )
}

// Row height estimate for the virtualizer below; measureElement corrects
// it per row once rendered, same as AppsListBody's own grid mode.
const ESTIMATED_ROW_HEIGHT = 224

// Virtualized card grid: the catalog (internal/catalog) is well
// past this repo's "lists over 50 items must be virtualized" rule, and the
// row-of-N windowing here mirrors AppsListBody's own grid-mode pattern
// rather than introducing a second virtualization approach.
function VirtualTemplateGrid({
  templates,
  onSelect,
  onDeployNow,
  deployingId,
  customTemplateIds,
}: {
  templates: ServiceTemplateListItem[]
  onSelect: (id: string) => void
  onDeployNow: (template: ServiceTemplateListItem) => void
  deployingId?: string
  customTemplateIds: Set<string>
}) {
  const parentRef = useRef<HTMLDivElement>(null)
  const cols = useGridColumns(parentRef)
  const rowCount = Math.ceil(templates.length / cols)
  const virtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ESTIMATED_ROW_HEIGHT,
    overscan: 4,
  })

  useEffect(() => {
    virtualizer.scrollToIndex(0)
    // Jump back to the top whenever the filtered set changes shape, not
    // on every render: re-running per templates identity would fight the
    // user's own scrolling within an unchanged result set.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [templates.length])

  return (
    <div ref={parentRef} className="h-[70vh] overflow-auto">
      <div style={{ height: virtualizer.getTotalSize(), position: 'relative' }}>
        {virtualizer.getVirtualItems().map((virtualRow) => {
          const slice = templates.slice(
            virtualRow.index * cols,
            virtualRow.index * cols + cols,
          )
          return (
            <div
              key={virtualRow.key}
              data-index={virtualRow.index}
              ref={virtualizer.measureElement}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${virtualRow.start}px)`,
              }}
            >
              <div
                className="grid gap-3 pb-3"
                style={{
                  gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
                }}
              >
                {slice.map((template) => (
                  <MarketplaceCard
                    key={template.id}
                    template={template}
                    onSelect={onSelect}
                    onDeployNow={onDeployNow}
                    isCustom={customTemplateIds.has(template.id)}
                    deploying={deployingId === template.id}
                  />
                ))}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}

function LoadingGrid() {
  return (
    <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {Array.from({ length: 8 }).map((_, index) => (
        <Skeleton key={index} className="h-[212px] w-full rounded-xl" />
      ))}
    </div>
  )
}

// Full browse/discover experience for the standalone /templates route:
// search, a category rail (sidebar on md+, chips below it), and the
// virtualized grid above. Deploy mechanics are untouched: onSelect and
// onDeployNow are passed straight through, same contract
// ServiceTemplateGrid already exposes to its own callers.
export function TemplateMarketplace({
  onSelect,
  onDeployNow,
  deployingId,
}: {
  onSelect: (id: string) => void
  onDeployNow: (template: ServiceTemplateListItem) => void
  deployingId?: string
}) {
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState<string>(ALL_CATEGORIES)
  const templatesQuery = useServiceTemplates()
  const customTemplatesQuery = useCustomTemplates()

  // Reshaped into ServiceTemplateListItem's own wire shape so the rest
  // of this component treats them like any built-in catalog entry.
  const customAsListItems = useMemo<ServiceTemplateListItem[]>(
    () =>
      (customTemplatesQuery.data ?? []).map((t) => ({
        id: t.id,
        name: t.name,
        slogan: t.description || 'Saved from ' + (t.source_app || 'an app'),
        category: YOUR_TEMPLATES_CATEGORY,
        documentation_url: '',
        requires_configuration: t.requires_configuration,
      })),
    [customTemplatesQuery.data],
  )
  const customTemplateIds = useMemo(
    () => new Set(customAsListItems.map((t) => t.id)),
    [customAsListItems],
  )

  const templates = useMemo(
    () => [...customAsListItems, ...(templatesQuery.data ?? [])],
    [customAsListItems, templatesQuery.data],
  )

  const categoryCounts = useMemo(() => {
    const counts = new Map<string, number>()
    for (const template of templates) {
      counts.set(template.category, (counts.get(template.category) ?? 0) + 1)
    }
    return Array.from(counts.entries()).sort(
      (a, b) => b[1] - a[1] || a[0].localeCompare(b[0]),
    )
  }, [templates])

  const normalizedSearch = search.trim().toLowerCase()
  const filtered = useMemo(
    () =>
      templates.filter((template) => {
        if (category !== ALL_CATEGORIES && template.category !== category) {
          return false
        }
        return matchesSearch(template, normalizedSearch)
      }),
    [templates, category, normalizedSearch],
  )

  const hasFilters = !!search || category !== ALL_CATEGORIES

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
          placeholder={`Search ${templates.length || ''} templates...`}
          aria-label="Search templates"
          className="pl-8"
        />
      </div>

      {templatesQuery.isLoading ? (
        <LoadingGrid />
      ) : templatesQuery.isError ? (
        <Alert variant="destructive">
          <WarningIcon aria-hidden="true" />
          <AlertDescription>{templatesQuery.error.message}</AlertDescription>
        </Alert>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-[200px_1fr]">
          <CategorySidebar
            categoryCounts={categoryCounts}
            total={templates.length}
            active={category}
            onChange={setCategory}
          />
          <div className="min-w-0 space-y-3">
            <CategoryChips
              categoryCounts={categoryCounts}
              total={templates.length}
              active={category}
              onChange={setCategory}
            />
            <p className="text-xs text-muted-foreground">
              {filtered.length} of {templates.length} templates
            </p>
            {filtered.length === 0 ? (
              <EmptyState
                icon={<PackageIcon className="size-5" />}
                title="No templates match"
                description={
                  hasFilters
                    ? `Nothing in ${category === ALL_CATEGORIES ? 'the catalog' : category} matches "${search}".`
                    : 'This category has no templates yet.'
                }
                action={
                  hasFilters ? (
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        setSearch('')
                        setCategory(ALL_CATEGORIES)
                      }}
                    >
                      <XIcon aria-hidden="true" />
                      Clear filters
                    </Button>
                  ) : undefined
                }
              />
            ) : (
              <VirtualTemplateGrid
                templates={filtered}
                onSelect={onSelect}
                onDeployNow={onDeployNow}
                deployingId={deployingId}
                customTemplateIds={customTemplateIds}
              />
            )}
          </div>
        </div>
      )}
    </div>
  )
}
