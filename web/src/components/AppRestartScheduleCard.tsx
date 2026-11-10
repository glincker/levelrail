import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  useAppRestartSchedule,
  useClearAppRestartSchedule,
  useSetAppRestartSchedule,
} from '../queries/appRestartSchedule'

// Periodic restart to cap slow memory leaks; the server enforces the floor.
export function AppRestartScheduleCard({ appName }: { appName: string }) {
  const { t } = useTranslation('appRestartSchedule')
  const id = useId()
  const current = useAppRestartSchedule(appName)
  const save = useSetAppRestartSchedule(appName)
  const clear = useClearAppRestartSchedule(appName)
  const data = current.data
  const [expr, setExpr] = useState('')
  const [zone, setZone] = useState(
    () => Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
  )
  const sched = data?.schedule

  const onError = (err: Error) =>
    toast.add({ title: t('failed'), description: err.message, type: 'error' })
  const onSave = () =>
    save.mutate(
      { schedule: expr.trim(), timezone: zone.trim() },
      {
        onSuccess: () => {
          setExpr('')
          toast.add({ title: t('saved'), type: 'success' })
        },
        onError,
      },
    )
  const onClear = () =>
    clear.mutate(undefined, {
      onSuccess: () => toast.add({ title: t('cleared'), type: 'success' }),
      onError,
    })

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('title')}</CardTitle>
        <CardDescription>
          {t('description', { min: data?.min_interval ?? '1h0m0s' })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {sched ? (
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-sm">
              {t('current', {
                schedule: sched.schedule,
                zone: sched.timezone,
                next: new Date(sched.next_run_at).toLocaleString(),
              })}
            </p>
            <Button
              variant="outline"
              size="sm"
              disabled={clear.isPending}
              onClick={onClear}
            >
              {t('clear')}
            </Button>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('none')}</p>
        )}
        {sched?.last_status ? (
          <p className="text-sm text-muted-foreground">
            {t(`last.${sched.last_status}`, {
              at: sched.last_run_at
                ? new Date(sched.last_run_at).toLocaleString()
                : '',
              reason: sched.last_reason ?? '',
            })}
          </p>
        ) : null}
        <div className="flex flex-wrap items-end gap-3">
          <div className="space-y-1">
            <label htmlFor={`${id}-expr`} className="text-sm font-medium">
              {t('scheduleLabel')}
            </label>
            <Input
              id={`${id}-expr`}
              className="w-56 font-mono"
              placeholder="0 3 * * *"
              value={expr}
              onChange={(e) => setExpr(e.target.value)}
            />
          </div>
          <div className="space-y-1">
            <label htmlFor={`${id}-zone`} className="text-sm font-medium">
              {t('timezoneLabel')}
            </label>
            <Input
              id={`${id}-zone`}
              className="w-48"
              value={zone}
              onChange={(e) => setZone(e.target.value)}
            />
          </div>
          <Button
            disabled={expr.trim() === '' || save.isPending}
            onClick={onSave}
          >
            {t('save')}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">{t('hint')}</p>
      </CardContent>
    </Card>
  )
}

export default AppRestartScheduleCard
