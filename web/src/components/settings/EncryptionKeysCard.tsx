import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  KeyGlassIcon,
  RefreshCwGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { HelpLink } from '@/components/HelpLink'
import { RotateMasterKeyDialog } from '@/components/RotateMasterKeyDialog'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  ENCRYPTION_KEYS_STEP_UP_APP,
  useEncryptionKeys,
  useRotateDEK,
  useUpdateKeyPolicy,
  type EncryptionKeyHint,
  type EncryptionKeysStatus,
  type RotateDEKResult,
  type RotateDEKTarget,
} from '../../queries/encryptionKeys'
import { StepUpDialog } from '../env/StepUpDialog'
import { AppKeyList } from './AppKeyList'

const STEP_UP_SCOPE = {
  action: 'manage',
  app: ENCRYPTION_KEYS_STEP_UP_APP,
} as const

function PolicyForm({ status }: Readonly<{ status: EncryptionKeysStatus }>) {
  const { t } = useTranslation('encryptionKeys')
  const update = useUpdateKeyPolicy()
  const [maxAge, setMaxAge] = useState(String(status.policy.max_key_age_days))
  const [every, setEvery] = useState(String(status.policy.remind_every_days))
  const maxAgeValue = Number.parseInt(maxAge, 10)
  const everyValue = Number.parseInt(every, 10)
  const valid =
    Number.isInteger(maxAgeValue) &&
    maxAgeValue >= 0 &&
    Number.isInteger(everyValue) &&
    everyValue >= 1

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault()
        update.mutate(
          { max_key_age_days: maxAgeValue, remind_every_days: everyValue },
          {
            onSuccess: () =>
              toast.add({ title: t('policy.saved'), type: 'success' }),
            onError: (error) =>
              toast.add({ title: error.message, type: 'error' }),
          },
        )
      }}
    >
      <div>
        <h3 className="text-sm font-medium">{t('policy.title')}</h3>
        <p className="text-xs text-muted-foreground">
          {t('policy.description')}
        </p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="key-max-age">{t('policy.maxAge')}</FieldLabel>
          <Input
            id="key-max-age"
            type="number"
            min={0}
            value={maxAge}
            onChange={(e) => setMaxAge(e.target.value)}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor="key-remind-every">
            {t('policy.remindEvery')}
          </FieldLabel>
          <Input
            id="key-remind-every"
            type="number"
            min={1}
            value={every}
            onChange={(e) => setEvery(e.target.value)}
          />
        </Field>
      </div>
      <Button type="submit" size="sm" disabled={!valid || update.isPending}>
        {update.isPending ? t('policy.saving') : t('policy.save')}
      </Button>
    </form>
  )
}

function HintRow({
  hint,
  onFix,
}: Readonly<{
  hint: EncryptionKeyHint
  onFix: (h: EncryptionKeyHint) => void
}>) {
  const { t } = useTranslation('encryptionKeys')
  const updatePolicy = useUpdateKeyPolicy()
  const fixLabel = {
    rotate_master_key: t('hint.fixMaster'),
    rotate_dek: t('hint.fixApp'),
    rotate_all_deks: t('hint.fixAll'),
    enable_key_policy: t('hint.fixPolicy'),
  }[hint.fix.kind]
  const fix = () => {
    if (hint.fix.kind !== 'enable_key_policy') return onFix(hint)
    updatePolicy.mutate(
      { max_key_age_days: 90 },
      {
        onSuccess: () =>
          toast.add({ title: t('policy.saved'), type: 'success' }),
        onError: (error) => toast.add({ title: error.message, type: 'error' }),
      },
    )
  }
  return (
    <Alert variant={hint.severity === 'warning' ? 'destructive' : 'default'}>
      <TriangleAlertGlassIcon
        className="text-tone-warning"
        size="1.4em"
        aria-hidden="true"
      />
      <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
        <span>{hint.message}</span>
        {hint.fix.kind === 'rotate_master_key' ? (
          <RotateMasterKeyDialog />
        ) : (
          <Button
            variant="outline"
            size="sm"
            disabled={updatePolicy.isPending}
            onClick={fix}
          >
            {fixLabel}
          </Button>
        )}
      </AlertDescription>
    </Alert>
  )
}

function LastRun({
  run,
}: Readonly<{ run: NonNullable<EncryptionKeysStatus['last_run']> }>) {
  const { t } = useTranslation('encryptionKeys')
  return (
    <div className="text-xs text-muted-foreground">
      <span className="font-medium text-foreground">{t('lastRun.title')}</span>
      {': '}
      {t('lastRun.progress', {
        done: run.done,
        total: run.total,
        failed: run.failed,
      })}
      {' · '}
      {run.finished_at
        ? t('lastRun.finished', {
            date: new Date(run.finished_at).toLocaleString(),
          })
        : t('lastRun.interrupted')}
    </div>
  )
}

export function EncryptionKeysCard() {
  const { t } = useTranslation('encryptionKeys')
  const { data: status, error } = useEncryptionKeys()
  const rotate = useRotateDEK()
  const [target, setTarget] = useState<RotateDEKTarget | null>(null)
  const [failure, setFailure] = useState<string | null>(null)

  const onGateError = useCallback(
    (e: unknown) => {
      const message = e instanceof Error ? e.message : t('errors.rotate')
      setFailure(message)
      toast.add({ title: message, type: 'error' })
    },
    [t],
  )
  const gate = useStepUpGate(onGateError)

  if (status === null) return null
  if (error) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{t('errors.load')}</AlertDescription>
      </Alert>
    )
  }
  if (!status) return null

  const announce = (res: RotateDEKResult) => {
    if (res.app) {
      toast.add({
        title: t('result.single', { app: res.app, count: res.values ?? 0 }),
        type: 'success',
      })
      return
    }
    const failed = res.failed_count ?? 0
    toast.add({
      title: t('result.all', {
        rotated: res.rotated ?? 0,
        skipped: res.skipped ?? 0,
      }),
      type: failed > 0 || !res.complete ? 'warning' : 'success',
    })
    if (failed > 0) {
      setFailure(
        t('result.allFailed', {
          failed,
          names: (res.failed ?? []).map((f) => f.app).join(', '),
        }),
      )
    } else if (!res.complete) {
      setFailure(t('result.partial'))
    }
  }

  const confirmRotation = async () => {
    const chosen = target
    if (!chosen) return
    setFailure(null)
    await gate.guard(STEP_UP_SCOPE, async () => {
      announce(await rotate.mutateAsync(chosen))
      setTarget(null)
    })
  }

  const openFix = (hint: EncryptionKeyHint) =>
    setTarget(
      hint.fix.kind === 'rotate_dek'
        ? { service: hint.fix.app }
        : { all: true },
    )

  const masterAge = status.master_key.age_days
  const apps = status.apps
  const rotatingAll = target?.all === true

  return (
    <Card>
      <CardHeader>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
              <KeyGlassIcon className="size-5" aria-hidden="true" />
            </div>
            <div>
              <CardTitle className="flex items-center gap-1.5">
                {t('title')}
                <HelpLink path="/master-key-rotation" label={t('guide')} />
              </CardTitle>
              <CardDescription>{t('description')}</CardDescription>
            </div>
          </div>
          {apps.length > 0 ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => setTarget({ all: true })}
            >
              <RefreshCwGlassIcon className="size-5" aria-hidden="true" />
              {t('apps.rotateAll')}
            </Button>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {status.hints.map((hint) => (
          <HintRow
            key={`${hint.code}:${hint.fix.app ?? ''}`}
            hint={hint}
            onFix={openFix}
          />
        ))}

        <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border bg-card px-3 py-2">
          <span className="text-sm font-medium">{t('master.title')}</span>
          <span className="text-sm text-muted-foreground">
            {masterAge === undefined
              ? t('master.never')
              : t('master.age', { count: masterAge })}
          </span>
        </div>

        <div className="space-y-2">
          <h3 className="text-sm font-medium">{t('apps.title')}</h3>
          {apps.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('apps.empty')}</p>
          ) : (
            <AppKeyList
              apps={apps}
              onRotate={(app) => setTarget({ service: app })}
            />
          )}
          {status.last_run ? <LastRun run={status.last_run} /> : null}
        </div>

        {failure ? (
          <Alert variant="destructive">
            <AlertDescription>{failure}</AlertDescription>
          </Alert>
        ) : null}

        <PolicyForm
          key={`${status.policy.max_key_age_days}:${status.policy.remind_every_days}`}
          status={status}
        />
      </CardContent>

      <ConfirmDialog
        open={target !== null && !gate.pending}
        onOpenChange={(open) => {
          if (!open) setTarget(null)
        }}
        title={
          rotatingAll
            ? t('confirm.titleAll')
            : t('confirm.titleOne', { app: target?.service ?? '' })
        }
        description={
          rotatingAll
            ? t('confirm.descriptionAll')
            : t('confirm.descriptionOne')
        }
        consequences={[
          t('confirm.consequenceReadable'),
          t('confirm.consequenceAudit'),
        ]}
        confirmLabel={t('confirm.action')}
        pendingLabel={t('confirm.pending')}
        requireTyped={rotatingAll ? t('confirm.typed') : undefined}
        pending={rotate.isPending}
        onConfirm={() => {
          void confirmRotation()
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
