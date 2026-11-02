import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SlidersHorizontalGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { RuntimeCategoryNav } from '@/components/settings/RuntimeCategoryNav'
import { SettingRows } from '@/components/ui/setting-row'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { RuntimeRestartBanner } from '@/components/settings/RuntimeRestartBanner'
import { RetentionOverviewCard } from '@/components/settings/RetentionOverviewCard'
import { RuntimeSettingRow } from '@/components/settings/RuntimeSettingRow'
import { RuntimeSettingsToolbar } from '@/components/settings/RuntimeSettingsToolbar'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'
import { useDebouncedValue } from '../../hooks/useDebouncedValue'
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  categoryAnchor,
  filterSettings,
  groupByCategory,
  parseCategory,
  settingItemId,
} from '../../lib/runtimeSettings'
import type { RuntimeSettingCategory } from '../../types/runtimeSettings'
import {
  runtimeSettingsQueryOptions,
  useRuntimeSettings,
} from '../../queries/runtimeSettings'

const SEARCH_DEBOUNCE_MS = 200

export const Route = createFileRoute('/settings/security-runtime')({
  validateSearch: (
    search: Record<string, unknown>,
  ): { category?: RuntimeSettingCategory } => {
    const category = parseCategory(search.category)
    return category ? { category } : {}
  },
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(runtimeSettingsQueryOptions()),
  component: SecurityRuntimePage,
  pendingComponent: () => (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton rows={3} />
    </div>
  ),
})

function SecurityRuntimePage() {
  const { t } = useTranslation('runtimeSettings')
  const isRoot = useIsRoot()
  const { data, error, refetch } = useRuntimeSettings()
  const settings = useMemo(() => data?.settings ?? [], [data])
  const pending = settings.filter((s) => s.pending_restart).length
  const { category } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const [query, setQuery] = useState('')
  const [changedOnly, setChangedOnly] = useState(false)
  const needle = useDebouncedValue(query, SEARCH_DEBOUNCE_MS)
  const textOf = useMemo(
    () => (s: (typeof settings)[number]) => {
      const id = settingItemId(s.key)
      return {
        title: t(`items.${id}.title`, { defaultValue: s.title }),
        description: t(`items.${id}.description`, {
          defaultValue: s.description,
        }),
      }
    },
    [t],
  )
  // Counts ignore the category so the nav shows where matches live.
  const matched = useMemo(
    () => filterSettings(settings, { query: needle, changedOnly }, textOf),
    [settings, needle, changedOnly, textOf],
  )
  const visible = category
    ? matched.filter((s) => s.category === category)
    : matched
  const groups = groupByCategory(visible)
  const counts = groupByCategory(matched).map((g) => ({
    category: g.category,
    count: g.items.length,
  }))
  const filtering = needle.trim() !== '' || changedOnly
  const selectCategory = (next: RuntimeSettingCategory | undefined) => {
    void navigate({ search: next ? { category: next } : {}, replace: true })
  }

  return (
    <PageTemplate
      icon={<SlidersHorizontalGlassIcon size="1.4em" />}
      title={t('title')}
      description={t('description')}
      banners={
        pending > 0 ? (
          <RuntimeRestartBanner
            count={pending}
            supported={data?.restart.supported ?? false}
            canEdit={isRoot}
          />
        ) : null
      }
    >
      {error && !data ? (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-wrap items-center gap-3">
            {t('loadError', { message: error.message })}
            <Button size="sm" variant="outline" onClick={() => void refetch()}>
              {t('retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : null}
      {!isRoot && settings.length > 0 ? (
        <p className="text-sm text-muted-foreground">{t('readOnly')}</p>
      ) : null}
      {data && settings.length === 0 ? (
        <EmptyState
          icon={<SlidersHorizontalGlassIcon className="size-8" />}
          title={t('empty.title')}
          description={t('empty.body')}
          action={
            <Button size="sm" variant="outline" onClick={() => void refetch()}>
              {t('retry')}
            </Button>
          }
        />
      ) : null}
      {settings.length > 0 ? (
        <RuntimeSettingsToolbar
          query={query}
          onQuery={setQuery}
          changedOnly={changedOnly}
          onChangedOnly={setChangedOnly}
          shown={visible.length}
          total={settings.length}
        />
      ) : null}
      {(filtering || category) &&
      settings.length > 0 &&
      visible.length === 0 ? (
        <EmptyState
          icon={<SlidersHorizontalGlassIcon className="size-8" />}
          title={t('toolbar.noMatchTitle')}
          description={t('toolbar.noMatchBody')}
          action={
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setQuery('')
                setChangedOnly(false)
                selectCategory(undefined)
              }}
            >
              {t('toolbar.clear')}
            </Button>
          }
        />
      ) : null}
      <div className="flex flex-col gap-4 lg:flex-row lg:items-start">
        {settings.length > 0 ? (
          <RuntimeCategoryNav
            counts={counts}
            total={matched.length}
            active={category}
            onSelect={selectCategory}
          />
        ) : null}
        <div className="min-w-0 flex-1 space-y-4">
          {groups.map((group) => (
            <Card
              key={group.category}
              id={categoryAnchor(group.category)}
              className="scroll-mt-28"
            >
              <CardHeader>
                <CardTitle>{t(`categories.${group.category}`)}</CardTitle>
              </CardHeader>
              <CardContent>
                <SettingRows>
                  {group.items.map((s) => (
                    <RuntimeSettingRow
                      key={s.key}
                      setting={s}
                      canEdit={isRoot}
                      highlight={needle}
                    />
                  ))}
                </SettingRows>
              </CardContent>
            </Card>
          ))}
          {settings.length > 0 && !filtering && !category ? (
            <RetentionOverviewCard />
          ) : null}
        </div>
      </div>
    </PageTemplate>
  )
}
