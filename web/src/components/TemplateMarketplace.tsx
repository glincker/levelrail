import { useEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  MagnifyingGlassIcon,
  PackageIcon,
  RocketLaunchIcon,
  UserIcon,
  WarningIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
import { CATEGORY_ICONS } from './ServiceTemplateGrid'
import {
  ALL_CATEGORIES,
  CategoryChips,
  CategorySidebar,
} from './TemplateCategoryRail'
import { TemplateLogo } from './TemplateLogo'
import { DeleteCustomTemplateDialog } from './DeleteCustomTemplateDialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { formatBytes } from '../lib/format'
import {
  useServiceTemplates,
  type ServiceTemplateListItem,
} from '../queries/serviceTemplates'
import { useCustomTemplates } from '../queries/customTemplates'

// Synthetic category for operator-defined templates; never a real
// catalog.Template.Category string, so the two can't collide.
const YOUR_TEMPLATES_CATEGORY = 'Your templates'

function matchesSearch(
  template: ServiceTemplateListItem,
  query: string,
): boolean {
  if (!query) return true
  return (
    template.id.toLowerCase().includes(query) ||
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
  const { t } = useTranslation('compose')
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
        {template.recommended_memory_bytes ||
        template.requires_gpu ||
        template.installable ? (
          <div className="flex flex-wrap items-center gap-1.5">
            {template.installable && (
              <Badge variant="muted" className="text-[11px]">
                {t('catalog.installForm')}
              </Badge>
            )}
            {!!template.recommended_memory_bytes && (
              <Badge variant="muted" className="text-[11px]">
                ~{formatBytes(template.recommended_memory_bytes)} RAM
              </Badge>
            )}
            {template.requires_gpu && (
              <Badge variant="muted" className="text-[11px]">
                {t('catalog.gpuRequired')}
              </Badge>
            )}
          </div>
        ) : null}
      </button>
      {template.installable ? (
        <Button
          type="button"
          size="sm"
          variant="outline"
          className="w-full"
          onClick={() => {
            onSelect(template.id)
          }}
        >
          <RocketLaunchIcon aria-hidden="true" />
          {t('catalog.install')}
        </Button>
      ) : (
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
            ? t('catalog.deploying')
            : template.requires_configuration
              ? t('catalog.deployNeedsSetup')
              : t('catalog.deployNow')}
        </Button>
      )}
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
  const { t } = useTranslation('compose')
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState<string>(ALL_CATEGORIES)
  const [installableOnly, setInstallableOnly] = useState(false)
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
        if (installableOnly && !template.installable) {
          return false
        }
        return matchesSearch(template, normalizedSearch)
      }),
    [templates, category, normalizedSearch, installableOnly],
  )

  const hasFilters = !!search || category !== ALL_CATEGORIES || installableOnly

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
          placeholder={t('catalog.searchPlaceholder', {
            count: templates.length,
          })}
          aria-label={t('catalog.searchLabel')}
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
            <div className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-xs text-muted-foreground">
                {t('catalog.count', {
                  shown: filtered.length,
                  total: templates.length,
                })}
              </p>
              <Button
                type="button"
                size="sm"
                variant={installableOnly ? 'default' : 'outline'}
                aria-pressed={installableOnly}
                onClick={() => {
                  setInstallableOnly((v) => !v)
                }}
              >
                {t('catalog.installableOnly')}
              </Button>
            </div>
            {filtered.length === 0 ? (
              <EmptyState
                icon={<PackageIcon className="size-5" />}
                title={t('catalog.emptyTitle')}
                description={
                  hasFilters
                    ? t('catalog.emptyFiltered')
                    : t('catalog.emptyCategory')
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
                        setInstallableOnly(false)
                      }}
                    >
                      <XIcon aria-hidden="true" />
                      {t('catalog.clearFilters')}
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
