import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
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
  useAppExpiry,
  useClearAppExpiry,
  useSetAppExpiry,
} from '../queries/appExpiry'

// datetime-local carries no zone, so the browser's local time is sent as UTC.
function toRFC3339(local: string): string {
  const d = new Date(local)
  return Number.isNaN(d.getTime())
    ? ''
    : d.toISOString().replace(/\.\d{3}Z$/, 'Z')
}

// Expiry for a temporary app: stop at the date, delete after the grace period.
export function AppExpiryCard({ appName }: { appName: string }) {
  const { t } = useTranslation('appExpiry')
  const id = useId()
  const current = useAppExpiry(appName)
  const save = useSetAppExpiry(appName)
  const clear = useClearAppExpiry(appName)
  const [at, setAt] = useState('')
  const [deleteData, setDeleteData] = useState(false)
  const [confirm, setConfirm] = useState('')
  const data = current.data
  const iso = toRFC3339(at)
  const canSave = iso !== '' && (!deleteData || confirm === appName)

  const onError = (err: Error) =>
    toast.add({ title: t('failed'), description: err.message, type: 'error' })
  const onSave = () =>
    save.mutate(
      { expires_at: iso, delete_data: deleteData, confirm },
      {
        onSuccess: () => {
          setAt('')
          setConfirm('')
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
          {t('description', {
            grace: data?.grace_hours ?? 168,
            notice: data?.notice_hours ?? 24,
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {data?.expiry ? (
          <div className="flex flex-wrap items-center gap-3">
            <p className="text-sm">
              {t(`state.${data.state}`, {
                at: new Date(data.expiry.expires_at).toLocaleString(),
                after: data.delete_after
                  ? new Date(data.delete_after).toLocaleString()
                  : '',
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
        {data && !data.delete_enabled ? (
          <p className="text-sm text-muted-foreground">{t('deleteDisabled')}</p>
        ) : null}
        {data?.expiry?.kept_reason ? (
          <Alert>
            <AlertDescription>
              {t('kept', { reason: data.expiry.kept_reason })}
            </AlertDescription>
          </Alert>
        ) : null}
        <div className="flex flex-wrap items-end gap-3">
          <div className="space-y-1">
            <label htmlFor={`${id}-at`} className="text-sm font-medium">
              {t('atLabel')}
            </label>
            <Input
              id={`${id}-at`}
              type="datetime-local"
              className="w-60"
              value={at}
              onChange={(e) => setAt(e.target.value)}
            />
          </div>
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={deleteData}
              onCheckedChange={(checked) => setDeleteData(checked === true)}
            />
            {t('deleteData')}
          </label>
          {deleteData ? (
            <div className="space-y-1">
              <label htmlFor={`${id}-confirm`} className="text-sm font-medium">
                {t('confirmLabel', { name: appName })}
              </label>
              <Input
                id={`${id}-confirm`}
                className="w-48"
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
              />
            </div>
          ) : null}
          <Button disabled={!canSave || save.isPending} onClick={onSave}>
            {t('save')}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          {t('hint', { days: data?.max_horizon_days ?? 90 })}
        </p>
      </CardContent>
    </Card>
  )
}

export default AppExpiryCard
