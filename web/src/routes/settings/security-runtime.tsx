import { createFileRoute } from '@tanstack/react-router'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SlidersHorizontalGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
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
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  categoryAnchor,
  filterSettings,
  groupByCategory,
  settingItemId,
} from '../../lib/runtimeSettings'
import {
  runtimeSettingsQueryOptions,
  useRuntimeSettings,
} from '../../queries/runtimeSettings'

export const Route = createFileRoute('/settings/security-runtime')({
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
  const [query, setQuery] = useState('')
  const [changedOnly, setChangedOnly] = useState(false)
  const visible = useMemo(
    () =>
      filterSettings(settings, { query, changedOnly }, (s) => {
        const id = settingItemId(s.key)
        return {
          title: t(`items.${id}.title`, { defaultValue: s.title }),
          description: t(`items.${id}.description`, {
            defaultValue: s.description,
          }),
        }
      }),
    [settings, query, changedOnly, t],
  )
  const groups = groupByCategory(visible)
  const filtering = query.trim() !== '' || changedOnly

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
          counts={groups.map((g) => ({
            category: g.category,
            count: g.items.length,
          }))}
          shown={visible.length}
          total={settings.length}
        />
      ) : null}
      {filtering && settings.length > 0 && visible.length === 0 ? (
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
              }}
            >
              {t('toolbar.clear')}
            </Button>
          }
        />
      ) : null}
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
                <RuntimeSettingRow key={s.key} setting={s} canEdit={isRoot} />
              ))}
            </SettingRows>
          </CardContent>
        </Card>
      ))}
      {settings.length > 0 && !filtering ? <RetentionOverviewCard /> : null}
    </PageTemplate>
  )
}
