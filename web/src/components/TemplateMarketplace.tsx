import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  PackageGlassIcon,
  TriangleAlertGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { useTemplateDensity } from '../hooks/useTemplateDensity'
import {
  ALL_CATEGORIES,
  STARTER_KITS_CATEGORY,
  countCategories,
  filterTemplates,
  sortTemplates,
  type TemplateSort,
} from '../lib/templateCatalogView'
import {
  useServiceTemplates,
  type ServiceTemplateListItem,
} from '../queries/serviceTemplates'
import { useCustomTemplates } from '../queries/customTemplates'
import { TemplateCategoryRail } from './templates/TemplateCategoryRail'
import { TemplateFeaturedStrip } from './templates/TemplateFeaturedStrip'
import { TemplateGrid } from './templates/TemplateGrid'
import { TemplateToolbar } from './templates/TemplateToolbar'

// Synthetic category for operator-defined templates; never a real catalog value.
const YOUR_TEMPLATES_CATEGORY = 'Your templates'

function LoadingGrid() {
  return (
    <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {Array.from({ length: 8 }).map((_, index) => (
        <Skeleton key={index} className="h-[216px] w-full rounded-xl" />
      ))}
    </div>
  )
}

/** TemplateMarketplace is the /templates browse page: starter-kit band, toolbar, category rail and virtualized catalog. */
export function TemplateMarketplace({
  onSelect,
  onDeployNow,
  deployingId,
  emptyAction,
}: {
  onSelect: (id: string) => void
  onDeployNow: (template: ServiceTemplateListItem) => void
  deployingId?: string
  /** Next step offered in empty states, e.g. the "New from a template" trigger. */
  emptyAction?: ReactNode
}) {
  const { t } = useTranslation('templates')
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState<string>(ALL_CATEGORIES)
  const [sort, setSort] = useState<TemplateSort>('relevance')
  const [verifiedOnly, setVerifiedOnly] = useState(false)
  const [density, setDensity] = useTemplateDensity()
  const templatesQuery = useServiceTemplates()
  const customTemplatesQuery = useCustomTemplates()

  const customAsListItems = useMemo<ServiceTemplateListItem[]>(
    () =>
      (customTemplatesQuery.data ?? []).map((tpl) => ({
        id: tpl.id,
        name: tpl.name,
        slogan: tpl.description || 'Saved from ' + (tpl.source_app || 'an app'),
        category: YOUR_TEMPLATES_CATEGORY,
        documentation_url: '',
        requires_configuration: tpl.requires_configuration,
      })),
    [customTemplatesQuery.data],
  )
  const customTemplateIds = useMemo(
    () => new Set(customAsListItems.map((tpl) => tpl.id)),
    [customAsListItems],
  )
  const templates = useMemo(
    () => [...customAsListItems, ...(templatesQuery.data ?? [])],
    [customAsListItems, templatesQuery.data],
  )
  const categoryCounts = useMemo(() => countCategories(templates), [templates])
  const visible = useMemo(
    () =>
      sortTemplates(
        filterTemplates(templates, search, category, verifiedOnly),
        sort,
        search,
      ),
    [templates, search, category, sort, verifiedOnly],
  )
  const starterKits = useMemo(
    () => templates.filter((tpl) => tpl.category === STARTER_KITS_CATEGORY),
    [templates],
  )

  const searching = search.trim() !== ''
  const hasFilters = searching || category !== ALL_CATEGORIES || verifiedOnly
  const showFeatured = !searching && category === ALL_CATEGORIES
  const clearFilters = () => {
    setSearch('')
    setCategory(ALL_CATEGORIES)
    setVerifiedOnly(false)
  }

  if (templatesQuery.isLoading) return <LoadingGrid />
  if (templatesQuery.isError) {
    return (
      <Alert variant="destructive">
        <TriangleAlertGlassIcon
          className="text-tone-warning"
          size="1.4em"
          aria-hidden="true"
        />
        <AlertDescription>{templatesQuery.error.message}</AlertDescription>
      </Alert>
    )
  }

  return (
    <div className="space-y-6">
      {showFeatured ? (
        <TemplateFeaturedStrip
          templates={starterKits}
          onSelect={onSelect}
          onDeployNow={onDeployNow}
          deployingId={deployingId}
        />
      ) : null}
      <div className="grid grid-cols-1 gap-6 md:grid-cols-[200px_1fr]">
        <div className="md:sticky md:top-4 md:self-start">
          <TemplateCategoryRail
            categoryCounts={categoryCounts}
            total={templates.length}
            active={category}
            onChange={setCategory}
          />
        </div>
        <div className="min-w-0">
          <TemplateToolbar
            search={search}
            onSearch={setSearch}
            sort={sort}
            onSort={setSort}
            verifiedOnly={verifiedOnly}
            onVerifiedOnly={setVerifiedOnly}
            density={density}
            onDensity={setDensity}
            categoryCounts={categoryCounts}
            total={templates.length}
            category={category}
            onCategory={setCategory}
            shown={visible.length}
          />
          {visible.length === 0 ? (
            <EmptyState
              icon={<PackageGlassIcon className="size-6" />}
              title={
                hasFilters
                  ? t('marketplace.empty.title')
                  : t('marketplace.empty.catalogTitle')
              }
              description={
                !hasFilters
                  ? t('marketplace.empty.catalogDescription')
                  : searching
                    ? t('marketplace.empty.filtered', {
                        category:
                          category === ALL_CATEGORIES
                            ? t('marketplace.categories.all')
                            : category,
                        query: search.trim(),
                      })
                    : t('marketplace.empty.filteredNoQuery', { category })
              }
              action={
                hasFilters ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={clearFilters}
                  >
                    <XGlassIcon size="1.4em" aria-hidden="true" />
                    {t('marketplace.empty.clear')}
                  </Button>
                ) : undefined
              }
              secondaryAction={emptyAction}
            />
          ) : (
            <TemplateGrid
              templates={visible}
              density={density}
              resetKey={`${search}|${category}|${sort}|${verifiedOnly}`}
              onSelect={onSelect}
              onDeployNow={onDeployNow}
              deployingId={deployingId}
              customTemplateIds={customTemplateIds}
            />
          )}
        </div>
      </div>
    </div>
  )
}
