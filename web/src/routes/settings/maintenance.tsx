import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { TrashGlassIcon, WrenchGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { ApiError } from '../../lib/apiError'
import { ScheduleMaintenanceDialog } from '../../components/maintenance/ScheduleMaintenanceDialog'
import {
  useDeleteScheduledMaintenance,
  useScheduledMaintenance,
} from '../../queries/scheduledMaintenance'

export const Route = createFileRoute('/settings/maintenance')({
  component: MaintenancePage,
})

function MaintenancePage() {
  const { t, i18n } = useTranslation('maintenance')
  const { data, isPending, error } = useScheduledMaintenance()
  const remove = useDeleteScheduledMaintenance()
  const notConfigured = error instanceof ApiError && error.status === 501
  const fmt = new Intl.DateTimeFormat(i18n.language, {
    dateStyle: 'medium',
    timeStyle: 'short',
  })

  return (
    <PageTemplate
      icon={<WrenchGlassIcon size="1.4em" />}
      title={t('title')}
      description={t('description')}
      actions={notConfigured ? null : <ScheduleMaintenanceDialog />}
    >
      {isPending ? <Skeleton className="h-32 w-full" /> : null}
      {notConfigured ? (
        <Banner variant="critical" title={t('notConfigured')} />
      ) : null}
      {error && !notConfigured ? (
        <Banner variant="critical" title={error.message} />
      ) : null}
      {data && data.length === 0 ? (
        <EmptyState
          icon={<WrenchGlassIcon size="1.4em" />}
          title={t('empty.title')}
          description={t('empty.description')}
        />
      ) : null}
      <div className="space-y-3">
        {data?.map((w) => (
          <Card key={w.id}>
            <CardContent className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0 space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{w.title}</span>
                  <Badge variant="outline">{t(`state.${w.state}`)}</Badge>
                  {w.freeze ? (
                    <Badge variant="warning">{t('freezeBadge')}</Badge>
                  ) : null}
                </div>
                <p className="text-sm text-muted-foreground">
                  {fmt.format(new Date(w.starts_at))} {t('until')}{' '}
                  {fmt.format(new Date(w.ends_at))}
                </p>
                <p className="text-sm text-muted-foreground">
                  {w.scope === 'all'
                    ? t('scope.all')
                    : t(`scope.${w.scope}`, { target: w.target ?? '' })}
                </p>
                {w.message ? <p className="text-sm">{w.message}</p> : null}
              </div>
              {w.state === 'ended' ? null : (
                <Button
                  variant="outline"
                  size="sm"
                  disabled={remove.isPending}
                  onClick={() => {
                    remove.mutate(w.id)
                  }}
                >
                  <TrashGlassIcon className="size-5" aria-hidden="true" />
                  {t('cancel')}
                </Button>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
    </PageTemplate>
  )
}
