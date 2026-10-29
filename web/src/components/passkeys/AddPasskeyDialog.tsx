import { type FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleGlassIcon,
  FingerprintPatternGlassIcon,
  LoaderCircleGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  type PasskeyResource,
  type RegisterPhase,
  useRegisterPasskey,
} from '@/queries/passkeys'
import {
  classifyPasskeyError,
  isRetryablePasskeyError,
  type PasskeyErrorKind,
} from '@/lib/passkeyErrors'
import { detectPasskeyDevice } from '@/lib/passkeyDevice'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { PasskeyProviderIcon } from './PasskeyProviderIcon'

type Stage = 'name' | RegisterPhase | 'done'

const STEPS: Array<{
  stage: Stage
  labelKey:
    'addDialog.stepName' | 'addDialog.stepPrompt' | 'addDialog.stepSaved'
}> = [
  { stage: 'name', labelKey: 'addDialog.stepName' },
  { stage: 'prompt', labelKey: 'addDialog.stepPrompt' },
  { stage: 'done', labelKey: 'addDialog.stepSaved' },
]

function stepState(stage: Stage, step: Stage): 'done' | 'active' | 'pending' {
  const order: Stage[] = ['name', 'prompt', 'saving', 'done']
  const at = order.indexOf(stage)
  const own = order.indexOf(step)
  if (stage === 'done' || at > own) return 'done'
  if (at === own || (step === 'prompt' && stage === 'saving')) return 'active'
  return 'pending'
}

export function AddPasskeyDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('passkeys')
  const register = useRegisterPasskey()
  const [label, setLabel] = useState('')
  const [stage, setStage] = useState<Stage>('name')
  const [errorKind, setErrorKind] = useState<PasskeyErrorKind | null>(null)
  const [created, setCreated] = useState<PasskeyResource | null>(null)

  const handleOpenChange = onOpenChange

  // The parent owns `open`; reseed the form on each open and clear it on close.
  const [seeded, setSeeded] = useState(false)
  if (open && !seeded) {
    setSeeded(true)
    setLabel(t(`addDialog.defaults.${detectPasskeyDevice()}`))
    setStage('name')
  }
  if (!open && seeded) {
    setSeeded(false)
    setErrorKind(null)
    setCreated(null)
    register.reset()
  }

  function start(e?: FormEvent) {
    e?.preventDefault()
    setErrorKind(null)
    setStage('prompt')
    register.mutate(
      {
        label: label.trim() || t('addDialog.defaults.generic'),
        onPhase: setStage,
      },
      {
        onSuccess: (row) => {
          setCreated(row)
          setStage('done')
        },
        onError: (err) => {
          setErrorKind(classifyPasskeyError(err))
          setStage('name')
        },
      },
    )
  }

  const working = stage === 'prompt' || stage === 'saving'

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={start} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t('addDialog.title')}</DialogTitle>
            <DialogDescription>{t('addDialog.description')}</DialogDescription>
          </DialogHeader>
          <ol
            className="flex items-center gap-2 text-xs"
            aria-label={t('addDialog.title')}
          >
            {STEPS.map(({ stage: step, labelKey }) => {
              const state = stepState(stage, step)
              return (
                <li
                  key={step}
                  aria-current={state === 'active' ? 'step' : undefined}
                  className={
                    state === 'pending'
                      ? 'flex items-center gap-1 text-muted-foreground'
                      : 'flex items-center gap-1 text-foreground'
                  }
                >
                  {state === 'done' ? (
                    <CircleCheckGlassIcon className="size-5 text-tone-success" />
                  ) : state === 'active' && working ? (
                    <LoaderCircleGlassIcon className="size-5 animate-spin" />
                  ) : (
                    <CircleGlassIcon className="size-5" />
                  )}
                  {t(labelKey)}
                </li>
              )
            })}
          </ol>

          {stage === 'done' && created ? (
            <Alert>
              <CircleCheckGlassIcon
                className="text-tone-success"
                size="1.4em"
              />
              <AlertTitle>{t('addDialog.savedTitle')}</AlertTitle>
              <AlertDescription>
                <p>{t('addDialog.savedBody', { name: created.label })}</p>
                <p className="mt-1 flex items-center gap-2">
                  <PasskeyProviderIcon providerKey={created.provider_key} />
                  {created.provider_name
                    ? t('addDialog.savedProvider', {
                        provider: created.provider_name,
                      })
                    : t('row.provider')}
                </p>
              </AlertDescription>
            </Alert>
          ) : (
            <Field>
              <FieldLabel htmlFor="passkey-label">
                {t('addDialog.nameLabel')}
              </FieldLabel>
              <Input
                id="passkey-label"
                value={label}
                disabled={working}
                maxLength={64}
                onChange={(e) => {
                  setLabel(e.target.value)
                }}
                autoFocus
              />
            </Field>
          )}

          {working ? (
            <p className="text-sm text-muted-foreground" role="status">
              {stage === 'saving'
                ? t('addDialog.saving')
                : t('addDialog.waiting')}
            </p>
          ) : null}

          {errorKind ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.4em"
              />
              <AlertDescription>{t(`errors.${errorKind}`)}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            {stage === 'done' ? (
              <Button
                type="button"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                {t('addDialog.done')}
              </Button>
            ) : (
              <>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    handleOpenChange(false)
                  }}
                >
                  {t('addDialog.cancel')}
                </Button>
                <Button type="submit" disabled={working}>
                  <FingerprintPatternGlassIcon size="1.4em" />
                  {errorKind && isRetryablePasskeyError(errorKind)
                    ? t('addDialog.retry')
                    : t('addDialog.continue')}
                </Button>
              </>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
