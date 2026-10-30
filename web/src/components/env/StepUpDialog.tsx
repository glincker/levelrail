import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { FingerprintPatternGlassIcon, LockGlassIcon } from '@thesvg/ui-react'
import {
  EnvApiError,
  stepUpWithPasskey,
  stepUpWithPassword,
  useStepUpOptions,
  type StepUpScope,
} from '../../queries/envProtection'
import { isPasskeySupported } from '../../queries/passkeys'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export interface StepUpDialogProps {
  scope: StepUpScope | null
  subject?: string
  /** Replaces the action-based sentence, for scopes that are not an app. */
  description?: string
  onVerified: () => void
  onCancel: () => void
}

function errorText(
  error: unknown,
  t: ReturnType<typeof useTranslation<'envEditor'>>['t'],
): string {
  if (error instanceof EnvApiError) {
    if (error.code === 'step_up_locked') return t('stepUp.locked')
    if (error.code === 'step_up_invalid') return t('stepUp.invalid')
    if (error.code === 'step_up_code_required') return t('stepUp.codeHint')
  }
  return error instanceof Error ? error.message : String(error)
}

export function StepUpDialog({
  scope,
  subject,
  description: descriptionOverride,
  onVerified,
  onCancel,
}: StepUpDialogProps) {
  const { t } = useTranslation('envEditor')
  const open = scope !== null
  const options = useStepUpOptions(open)
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [needCode, setNeedCode] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const reset = () => {
    setPassword('')
    setCode('')
    setNeedCode(false)
    setError(null)
    setBusy(false)
  }

  const methods = options.data?.methods ?? []
  const showCode = needCode || options.data?.code_required === true
  const minutes = Math.max(
    1,
    Math.round((options.data?.grant_ttl_seconds ?? 300) / 60),
  )

  const succeed = () => {
    reset()
    onVerified()
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!scope) return
    setBusy(true)
    setError(null)
    try {
      await stepUpWithPassword(scope, password, code)
      succeed()
    } catch (err) {
      if (err instanceof EnvApiError && err.code === 'step_up_code_required') {
        setNeedCode(true)
      }
      setError(errorText(err, t))
      setBusy(false)
    }
  }

  const confirmWithPasskey = async () => {
    if (!scope) return
    setBusy(true)
    setError(null)
    try {
      await stepUpWithPasskey(scope)
      succeed()
    } catch (err) {
      setError(errorText(err, t))
      setBusy(false)
    }
  }

  const description = descriptionOverride
    ? descriptionOverride
    : !scope
      ? ''
      : scope.action === 'reveal'
        ? t('stepUp.descriptionReveal', {
            key: subject ?? '',
            app: scope.app,
            minutes,
          })
        : scope.action === 'export'
          ? t('stepUp.descriptionExport', { app: scope.app })
          : t('stepUp.descriptionManage', { app: scope.app })

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          reset()
          onCancel()
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <LockGlassIcon aria-hidden className="size-5" />
            {t('stepUp.title')}
          </DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {options.isSuccess && methods.length === 0 ? (
          <Alert>
            <AlertDescription>{t('stepUp.noMethods')}</AlertDescription>
          </Alert>
        ) : null}
        {methods.includes('password') ? (
          <form
            id="step-up-form"
            className="space-y-3"
            onSubmit={(e) => {
              void submit(e)
            }}
          >
            <div className="space-y-1.5">
              <Label htmlFor="step-up-password">{t('stepUp.password')}</Label>
              <Input
                id="step-up-password"
                type="password"
                autoComplete="current-password"
                autoFocus
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value)
                }}
              />
            </div>
            {showCode ? (
              <div className="space-y-1.5">
                <Label htmlFor="step-up-code">{t('stepUp.code')}</Label>
                <Input
                  id="step-up-code"
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  value={code}
                  onChange={(e) => {
                    setCode(e.target.value)
                  }}
                />
                <p className="text-xs text-muted-foreground">
                  {t('stepUp.codeHint')}
                </p>
              </div>
            ) : null}
          </form>
        ) : null}
        {methods.includes('passkey') && isPasskeySupported() ? (
          <div className="space-y-2">
            {methods.includes('password') ? (
              <p className="text-center text-xs text-muted-foreground">
                {t('stepUp.or')}
              </p>
            ) : null}
            <Button
              type="button"
              variant="outline"
              className="w-full"
              disabled={busy}
              onClick={() => {
                void confirmWithPasskey()
              }}
            >
              <FingerprintPatternGlassIcon size="1.4em" />
              {t('stepUp.passkey')}
            </Button>
          </div>
        ) : null}
        {error ? (
          <Alert variant="destructive" role="alert">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        {methods.includes('password') ? (
          <DialogFooter>
            <Button
              type="submit"
              form="step-up-form"
              disabled={busy || password === ''}
            >
              {busy ? t('stepUp.confirming') : t('stepUp.confirm')}
            </Button>
          </DialogFooter>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
