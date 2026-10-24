import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { SlidersHorizontalIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { SettingRows } from '@/components/ui/setting-row'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { RuntimeRestartBanner } from '@/components/settings/RuntimeRestartBanner'
import { RuntimeSettingRow } from '@/components/settings/RuntimeSettingRow'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'
import { useIsRoot } from '../../hooks/useIsRoot'
import { groupByCategory } from '../../lib/runtimeSettings'
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
  const settings = data?.settings ?? []
  const pending = settings.filter((s) => s.pending_restart).length

  return (
    <PageTemplate
      icon={<SlidersHorizontalIcon />}
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
          icon={<SlidersHorizontalIcon className="size-6" />}
          title={t('empty.title')}
          description={t('empty.body')}
          action={
            <Button size="sm" variant="outline" onClick={() => void refetch()}>
              {t('retry')}
            </Button>
          }
        />
      ) : null}
      {groupByCategory(settings).map((group) => (
        <Card key={group.category}>
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
    </PageTemplate>
  )
}
