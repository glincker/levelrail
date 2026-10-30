import { type FormEvent, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleCheckGlassIcon,
  CopyGlassIcon,
  KeyGlassIcon,
  LockGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  useConfirmTwoFactor,
  useDisableTwoFactor,
  useRegenerateRecoveryCodes,
  useSetupTwoFactor,
  twoFactorStatusQueryOptions,
} from '../../queries/twoFactor'
import { QrCode } from './QrCode'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { CardError, CardLoading } from '../account/AccountCardStates'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'

// Reads GET /api/v1/auth/2fa (works with no master key configured) and
// branches into either the enable flow or the enabled-state actions.
// Setup/confirm/disable/regenerate all 501 without a master key, but
// that's surfaced per-dialog (each mutation's own error message), not
// hidden here: an operator who hasn't configured one yet should still
// see the feature and understand why it's unavailable, not have it
// silently missing from the page.
export function TwoFactorCard() {
  const { t } = useTranslation('settings')
  const {
    data: status,
    isPending,
    isError,
    isFetching,
    refetch,
  } = useQuery(twoFactorStatusQueryOptions())

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LockGlassIcon
            className="size-5 text-muted-foreground"
            aria-hidden="true"
          />
          {t('accountPage.twoFactor.title')}
        </CardTitle>
        <CardDescription>
          {t('accountPage.twoFactor.description')}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {isPending ? <CardLoading /> : null}
        {isError ? (
          <CardError
            retrying={isFetching}
            onRetry={() => {
              void refetch()
            }}
          />
        ) : null}
        {status?.enabled ? (
          <>
            <p className="text-sm text-muted-foreground">
              {t('accountPage.twoFactor.enabled', {
                count: status.recovery_codes_remaining,
              })}
            </p>
            <div className="flex flex-wrap gap-2">
              <RegenerateRecoveryCodesDialog />
              <DisableTwoFactorDialog />
            </div>
          </>
        ) : null}
        {status && !status.enabled ? (
          <>
            <p className="text-sm text-muted-foreground">
              {t('accountPage.twoFactor.disabledHint')}
            </p>
            <EnableTwoFactorDialog />
          </>
        ) : null}
      </CardContent>
    </Card>
  )
}

// A read-only value with a copy button, the same box-plus-copy-button
// shape CreateTokenDialog.tsx uses for a freshly minted token: this
// feature shows two of these (the setup key and the otpauth link) plus
// a grid of recovery codes, all copy-once-then-gone values.
function CopyableValue({ value, label }: { value: string; label: string }) {
  const { t } = useTranslation('settings')
  const { copied, copy } = useCopyToClipboard()
  return (
    <div className="space-y-1">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
        <code className="min-w-0 flex-1 overflow-x-auto text-xs break-all">
          {value}
        </code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            copy(value)
          }}
        >
          {copied ? (
            <CheckGlassIcon size="1.4em" />
          ) : (
            <CopyGlassIcon size="1.4em" />
          )}
          {copied
            ? t('accountPage.twoFactor.copied')
            : t('accountPage.twoFactor.copy')}
        </Button>
      </div>
    </div>
  )
}

function RecoveryCodeGrid({ codes }: { codes: string[] }) {
  const { t } = useTranslation('settings')
  const { copied, copy } = useCopyToClipboard()
  return (
    <div className="space-y-2">
      <div className="grid grid-cols-2 gap-2 rounded-lg border border-input bg-muted/50 p-3 break-all font-mono text-xs">
        {codes.map((code) => (
          <span key={code}>{code}</span>
        ))}
      </div>
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => {
          copy(codes.join('\n'))
        }}
      >
        {copied ? (
          <CheckGlassIcon size="1.4em" />
        ) : (
          <CopyGlassIcon size="1.4em" />
        )}
        {copied
          ? t('accountPage.twoFactor.copied')
          : t('accountPage.twoFactor.copyAll')}
      </Button>
    </div>
  )
}

// The enable flow is two steps in one dialog: setup (show the secret,
// collect a confirming code) then, on success, the recovery codes
// (shown exactly once, per handleConfirmTwoFactor's own contract).
// Opening the dialog calls setup immediately rather than waiting for a
// second click, since there is nothing else to configure first.
function EnableTwoFactorDialog() {
  const { t } = useTranslation('settings')
  const [open, setOpen] = useState(false)
  const [step, setStep] = useState<'setup' | 'recovery-codes'>('setup')
  const [code, setCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const setup = useSetupTwoFactor()
  const confirm = useConfirmTwoFactor()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setStep('setup')
      setCode('')
      setup.mutate()
    } else {
      setup.reset()
      confirm.reset()
    }
  }

  function handleConfirm(e: FormEvent) {
    e.preventDefault()
    confirm.mutate(code, {
      onSuccess: (resp) => {
        setRecoveryCodes(resp.recovery_codes)
        setStep('recovery-codes')
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button />}>
        <LockGlassIcon size="1.4em" />
        {t('accountPage.twoFactor.enable')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {step === 'setup' ? (
          <>
            <DialogHeader>
              <DialogTitle>{t('accountPage.twoFactor.setupTitle')}</DialogTitle>
              <DialogDescription>
                {t('accountPage.twoFactor.setupBody')}
              </DialogDescription>
            </DialogHeader>
            {setup.isPending ? (
              <CardLoading rows={2} />
            ) : setup.isError ? (
              <Alert variant="destructive">
                <TriangleAlertGlassIcon
                  className="text-tone-warning"
                  size="1.7em"
                />
                <AlertDescription className="flex flex-wrap items-center justify-between gap-2">
                  <span>{setup.error.message}</span>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setup.mutate()
                    }}
                  >
                    {t('accountPage.retry')}
                  </Button>
                </AlertDescription>
              </Alert>
            ) : setup.data ? (
              <form onSubmit={handleConfirm} className="space-y-3">
                <QrCode value={setup.data.provisioning_uri} />
                <p className="text-center text-xs text-muted-foreground">
                  {t('accountPage.twoFactor.qrFallback')}
                </p>
                <CopyableValue
                  value={setup.data.secret}
                  label={t('accountPage.twoFactor.setupKey')}
                />
                <CopyableValue
                  value={setup.data.provisioning_uri}
                  label={t('accountPage.twoFactor.setupLink')}
                />
                <Field>
                  <FieldLabel htmlFor="totp-confirm-code">
                    {t('accountPage.twoFactor.codeLabel')}
                  </FieldLabel>
                  <Input
                    id="totp-confirm-code"
                    autoComplete="one-time-code"
                    value={code}
                    onChange={(e) => {
                      setCode(e.target.value)
                    }}
                    placeholder={t('accountPage.twoFactor.codePlaceholder')}
                  />
                </Field>
                {confirm.isError ? (
                  <Alert variant="destructive">
                    <TriangleAlertGlassIcon
                      className="text-tone-warning"
                      size="1.7em"
                    />
                    <AlertDescription>{confirm.error.message}</AlertDescription>
                  </Alert>
                ) : null}
                <DialogFooter>
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      handleOpenChange(false)
                    }}
                  >
                    {t('accountPage.twoFactor.cancel')}
                  </Button>
                  <Button
                    type="submit"
                    disabled={confirm.isPending || code === ''}
                  >
                    {confirm.isPending
                      ? t('accountPage.twoFactor.verifying')
                      : t('accountPage.twoFactor.verify')}
                  </Button>
                </DialogFooter>
              </form>
            ) : null}
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <CircleCheckGlassIcon
                  size="1.4em"
                  className="text-green-700 dark:text-green-400"
                />
                {t('accountPage.twoFactor.enabledTitle')}
              </DialogTitle>
              <DialogDescription>
                {t('accountPage.twoFactor.recoveryBody')}
              </DialogDescription>
            </DialogHeader>
            <RecoveryCodeGrid codes={recoveryCodes} />
            <DialogFooter>
              <Button
                type="button"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                {t('accountPage.twoFactor.saved')}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

// Regenerating replaces the whole recovery-code set (the backend
// invalidates every previously issued code), gated on a live TOTP code
// specifically, not a recovery code, so this can never consume one of
// the codes it's about to throw away.
function RegenerateRecoveryCodesDialog() {
  const { t } = useTranslation('settings')
  const [open, setOpen] = useState(false)
  const [step, setStep] = useState<'confirm' | 'recovery-codes'>('confirm')
  const [code, setCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const regenerate = useRegenerateRecoveryCodes()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setStep('confirm')
      setCode('')
    } else {
      regenerate.reset()
    }
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    regenerate.mutate(code, {
      onSuccess: (resp) => {
        setRecoveryCodes(resp.recovery_codes)
        setStep('recovery-codes')
      },
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" />}>
        <KeyGlassIcon size="1.4em" />
        {t('accountPage.twoFactor.regenerate')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {step === 'confirm' ? (
          <form onSubmit={handleSubmit} className="space-y-3">
            <DialogHeader>
              <DialogTitle>{t('accountPage.twoFactor.regenerate')}</DialogTitle>
              <DialogDescription>
                {t('accountPage.twoFactor.regenerateBody')}
              </DialogDescription>
            </DialogHeader>
            <Field>
              <FieldLabel htmlFor="regenerate-code">
                {t('accountPage.twoFactor.codeLabel')}
              </FieldLabel>
              <Input
                id="regenerate-code"
                autoComplete="one-time-code"
                value={code}
                onChange={(e) => {
                  setCode(e.target.value)
                }}
                placeholder={t('accountPage.twoFactor.codePlaceholder')}
              />
            </Field>
            {regenerate.isError ? (
              <Alert variant="destructive">
                <TriangleAlertGlassIcon
                  className="text-tone-warning"
                  size="1.7em"
                />
                <AlertDescription>{regenerate.error.message}</AlertDescription>
              </Alert>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                {t('accountPage.twoFactor.cancel')}
              </Button>
              <Button
                type="submit"
                disabled={regenerate.isPending || code === ''}
              >
                {regenerate.isPending
                  ? t('accountPage.twoFactor.regenerating')
                  : t('accountPage.twoFactor.regenerateAction')}
              </Button>
            </DialogFooter>
          </form>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <CircleCheckGlassIcon
                  size="1.4em"
                  className="text-green-700 dark:text-green-400"
                />
                {t('accountPage.twoFactor.regeneratedTitle')}
              </DialogTitle>
              <DialogDescription>
                {t('accountPage.twoFactor.regeneratedBody')}
              </DialogDescription>
            </DialogHeader>
            <RecoveryCodeGrid codes={recoveryCodes} />
            <DialogFooter>
              <Button
                type="button"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                {t('accountPage.twoFactor.done')}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}

// Deliberately re-verifies the second factor itself (a TOTP code or a
// recovery code), not the account password: internal/api's
// handleDisableTwoFactor doc comment explains why a password re-check
// would undermine the exact property 2FA exists to provide.
function DisableTwoFactorDialog() {
  const { t } = useTranslation('settings')
  const [open, setOpen] = useState(false)
  const [useRecoveryCode, setUseRecoveryCode] = useState(false)
  const [code, setCode] = useState('')
  const disable = useDisableTwoFactor()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (next) {
      setCode('')
      setUseRecoveryCode(false)
    } else {
      disable.reset()
    }
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    disable.mutate(useRecoveryCode ? { recoveryCode: code } : { code }, {
      onSuccess: () => handleOpenChange(false),
    })
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" />}>
        {t('accountPage.twoFactor.disable')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <form onSubmit={handleSubmit} className="space-y-3">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <TriangleAlertGlassIcon className="size-6 text-destructive" />
              {t('accountPage.twoFactor.disableTitle')}
            </DialogTitle>
            <DialogDescription>
              {t('accountPage.twoFactor.disableBody')}
            </DialogDescription>
          </DialogHeader>
          <Field>
            <FieldLabel htmlFor="disable-code">
              {useRecoveryCode
                ? t('accountPage.twoFactor.recoveryCodeLabel')
                : t('accountPage.twoFactor.authCodeLabel')}
            </FieldLabel>
            <Input
              id="disable-code"
              autoComplete="one-time-code"
              value={code}
              onChange={(e) => {
                setCode(e.target.value)
              }}
              placeholder={
                useRecoveryCode
                  ? t('accountPage.twoFactor.recoveryPlaceholder')
                  : t('accountPage.twoFactor.codePlaceholder')
              }
            />
          </Field>
          <button
            type="button"
            className="text-xs text-muted-foreground hover:text-foreground"
            onClick={() => {
              setUseRecoveryCode((v) => !v)
              setCode('')
            }}
          >
            {useRecoveryCode
              ? t('accountPage.twoFactor.useAuthCode')
              : t('accountPage.twoFactor.useRecoveryCode')}
          </button>
          {disable.isError ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.7em"
              />
              <AlertDescription>{disable.error.message}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                handleOpenChange(false)
              }}
            >
              {t('accountPage.twoFactor.cancel')}
            </Button>
            <Button
              type="submit"
              variant="destructive"
              disabled={disable.isPending || code === ''}
            >
              {disable.isPending
                ? t('accountPage.twoFactor.disabling')
                : t('accountPage.twoFactor.disableAction')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
