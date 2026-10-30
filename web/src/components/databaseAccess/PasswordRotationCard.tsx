import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { KeyGlassIcon, RefreshCwGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  databasePasswordRotationKeys,
  previewDatabasePasswordRotation,
  stepUpScopeFor,
  useDatabasePasswordRotation,
  useRotateDatabasePassword,
  useSetDatabasePasswordInterval,
  type DatabasePasswordRotation,
} from '../../queries/databasePasswordRotation'
import { StepUpDialog } from '../env/StepUpDialog'

const STATE_VARIANT = {
  idle: 'success',
  pending: 'warning',
  failed: 'destructive',
} as const

function ScheduleForm({
  databaseName,
  status,
}: Readonly<{ databaseName: string; status: DatabasePasswordRotation }>) {
  const { t } = useTranslation('databaseAccess')
  const save = useSetDatabasePasswordInterval(databaseName)
  const [days, setDays] = useState(String(status.interval_days))
  const value = Number.parseInt(days, 10)
  const valid = Number.isInteger(value) && value >= 0 && value <= 3650

  return (
    <form
      className="flex flex-wrap items-end gap-3"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate(value, {
          onSuccess: () =>
            toast.add({ title: t('rotation.scheduleSaved'), type: 'success' }),
          onError: (error) =>
            toast.add({ title: error.message, type: 'error' }),
        })
      }}
    >
      <Field className="w-44">
        <FieldLabel htmlFor="db-rotation-days">
          {t('rotation.scheduleLabel')}
        </FieldLabel>
        <Input
          id="db-rotation-days"
          type="number"
          min={0}
          max={3650}
          value={days}
          onChange={(e) => setDays(e.target.value)}
        />
      </Field>
      <Button type="submit" size="sm" disabled={!valid || save.isPending}>
        {save.isPending ? t('rotation.scheduleSaving') : t('rotation.schedule')}
      </Button>
      <p className="basis-full text-xs text-muted-foreground">
        {status.interval_days > 0 && status.next_due
          ? t('rotation.scheduleNext', {
              days: status.interval_days,
              date: new Date(status.next_due).toLocaleDateString(),
            })
          : t('rotation.scheduleOff')}
      </p>
    </form>
  )
}

/** PasswordRotationCard shows the owner password's age and rotates it with a preview of the apps that restart. */
export function PasswordRotationCard({
  databaseName,
}: Readonly<{ databaseName: string }>) {
  const { t } = useTranslation('databaseAccess')
  const { data: status } = useDatabasePasswordRotation(databaseName)
  const rotate = useRotateDatabasePassword(databaseName)
  const [open, setOpen] = useState(false)
  const [failure, setFailure] = useState<string | null>(null)

  const preview = useQuery({
    queryKey: [
      ...databasePasswordRotationKeys.detail(databaseName),
      'preview',
    ] as const,
    queryFn: () => previewDatabasePasswordRotation(databaseName),
    enabled: open,
    gcTime: 0,
  })

  const onGateError = useCallback(
    (e: unknown) => {
      const message = e instanceof Error ? e.message : t('rotation.failed')
      setFailure(message)
      toast.add({ title: message, type: 'error' })
      setOpen(false)
    },
    [t],
  )
  const gate = useStepUpGate(onGateError)

  if (!status) return null

  const confirm = async () => {
    setFailure(null)
    await gate.guard(stepUpScopeFor(databaseName), async () => {
      const res = await rotate.mutateAsync()
      const restarted = res.restarted_apps?.length ?? 0
      toast.add({
        title: t('rotation.done', { count: restarted }),
        type: 'success',
      })
      setOpen(false)
    })
  }

  const apps = preview.data?.apps ?? []
  const consequences = [
    apps.length > 0
      ? t('rotation.willRestart', { count: apps.length, apps: apps.join(', ') })
      : t('rotation.noRestart'),
    t('rotation.verifies'),
  ]
  if ((preview.data?.suspended_apps.length ?? 0) > 0) {
    consequences.push(
      t('rotation.suspended', {
        apps: (preview.data?.suspended_apps ?? []).join(', '),
      }),
    )
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <KeyGlassIcon className="size-5" aria-hidden="true" />
            </div>
            <div>
              <CardTitle>{t('rotation.title')}</CardTitle>
              <CardDescription>{t('rotation.description')}</CardDescription>
            </div>
          </div>
          {status.supported ? (
            <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
              <RefreshCwGlassIcon className="size-5" aria-hidden="true" />
              {t('rotation.rotate')}
            </Button>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {!status.supported ? (
          <Alert>
            <AlertTitle>{t('rotation.unsupportedTitle')}</AlertTitle>
            <AlertDescription>{t('rotation.unsupportedBody')}</AlertDescription>
          </Alert>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-3 text-sm">
              <span>
                {status.last_changed_at && status.age_days !== undefined
                  ? t('rotation.age', { count: status.age_days })
                  : t('rotation.ageUnknown')}
              </span>
              <Badge variant={STATE_VARIANT[status.state]}>
                {t(`rotation.state.${status.state}`)}
              </Badge>
            </div>
            {status.state === 'pending' ? (
              <Alert>
                <AlertDescription>{t('rotation.pendingBody')}</AlertDescription>
              </Alert>
            ) : null}
            {status.state === 'failed' ? (
              <Alert variant="destructive">
                <AlertDescription>
                  {t('rotation.failedBody', {
                    reason: t(`rotation.reason.${status.last_error ?? ''}`, {
                      defaultValue: t('rotation.reason.unknown'),
                    }),
                  })}
                </AlertDescription>
              </Alert>
            ) : null}
            {failure ? (
              <Alert variant="destructive">
                <AlertDescription>{failure}</AlertDescription>
              </Alert>
            ) : null}
            <ScheduleForm
              key={`${status.interval_days}:${status.next_due ?? ''}`}
              databaseName={databaseName}
              status={status}
            />
          </>
        )}
      </CardContent>
      <ConfirmDialog
        open={open && !gate.pending}
        onOpenChange={setOpen}
        title={t('rotation.confirmTitle')}
        description={t('rotation.confirmDescription')}
        consequences={
          preview.isLoading ? [t('rotation.loading')] : consequences
        }
        confirmLabel={t('rotation.confirmAction')}
        pendingLabel={t('rotation.confirmPending')}
        pending={rotate.isPending}
        error={preview.error ? t('rotation.previewFailed') : null}
        onConfirm={() => {
          void confirm()
        }}
      />
      <StepUpDialog
        scope={gate.pending?.scope ?? null}
        onVerified={() => {
          void gate.onVerified()
        }}
        onCancel={gate.cancel}
      />
    </Card>
  )
}
