import { useTranslation } from 'react-i18next'
import { CalendarCheckGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { toast } from '@/components/ui/toast'
import {
  useCancelScheduledDeploy,
  useScheduledDeploys,
} from '../../queries/scheduledDeploys'

const VARIANT: Record<string, 'success' | 'warning' | 'destructive' | 'muted'> =
  {
    pending: 'muted',
    running: 'muted',
    fired: 'success',
    pending_approval: 'warning',
    skipped_freeze: 'warning',
    missed: 'warning',
    canceled: 'muted',
    failed: 'destructive',
  }

// Upcoming and recent one-shot deploys of an app, with cancel for pending ones.
export function ScheduledDeploysList({ appName }: { appName: string }) {
  const { t } = useTranslation('smartDeploy')
  const list = useScheduledDeploys(appName)
  const cancel = useCancelScheduledDeploy(appName)
  const rows = list.data ?? []
  if (!list.isLoading && !list.isError && rows.length === 0) return null

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <CalendarCheckGlassIcon className="size-4.5" aria-hidden="true" />
          {t('schedule.list.title')}
        </CardTitle>
        <p className="text-sm text-muted-foreground">
          {t('schedule.list.description')}
        </p>
      </CardHeader>
      <CardContent className="space-y-2">
        {list.isError ? (
          <p className="text-sm text-destructive">
            {t('schedule.list.loadFailed')}
          </p>
        ) : null}
        {rows.map((d) => (
          <div
            key={d.id}
            className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3"
          >
            <div className="min-w-0 space-y-0.5">
              <p className="truncate font-mono text-sm">
                {d.kind === 'image'
                  ? t('schedule.list.image', { image: d.image ?? '' })
                  : t('schedule.list.ref', { ref: d.ref ?? '' })}
              </p>
              <p className="text-xs text-muted-foreground">
                {t('schedule.list.runsAt', {
                  when: new Date(d.run_at).toLocaleString(),
                })}
                {d.reason ? `, ${d.reason}` : ''}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <Badge variant={VARIANT[d.status] ?? 'muted'}>
                {t(`schedule.list.status.${d.status}`)}
              </Badge>
              {d.status === 'pending' ? (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={cancel.isPending}
                  onClick={() => {
                    cancel.mutate(d.id, {
                      onSuccess: () => {
                        toast.add({
                          title: t('schedule.toast.canceled'),
                          type: 'success',
                        })
                      },
                      onError: (e) => {
                        toast.add({
                          title: t('schedule.toast.cancelFailed'),
                          description: e.message,
                          type: 'error',
                        })
                      },
                    })
                  }}
                >
                  {cancel.isPending
                    ? t('schedule.list.canceling')
                    : t('schedule.list.cancel')}
                </Button>
              ) : null}
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}
