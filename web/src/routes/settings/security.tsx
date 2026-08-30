import { createFileRoute } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import {
  CheckCircleIcon,
  CheckIcon,
  CopyIcon,
  KeyIcon,
  LockKeyIcon,
  SignOutIcon,
  ShieldCheckIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  sessionQueryOptions,
  useRevokeOtherSessions,
  useSession,
} from '../../queries/security'
import {
  twoFactorStatusQueryOptions,
  useConfirmTwoFactor,
  useDisableTwoFactor,
  useRegenerateRecoveryCodes,
  useSetupTwoFactor,
  useTwoFactorStatus,
} from '../../queries/twoFactor'
import { useCopyToClipboard } from '../../hooks/useCopyToClipboard'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
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
  DialogTrigger,
} from '@/components/ui/dialog'
import { PageSpinner } from '@/components/ui/page-spinner'

// Loader-primed the same way routes/settings/tokens.tsx primes
// tokenListQueryOptions: the component below only ever reads that warm
// cache via useSuspenseQuery, never fetches data in its own body (no
// data fetching in component bodies is a project-wide rule).
export const Route = createFileRoute('/settings/security')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(sessionQueryOptions()),
      queryClient.ensureQueryData(twoFactorStatusQueryOptions()),
    ]),
  component: SecuritySettingsPage,
  pendingComponent: PageSpinner,
})

// Matches TokenTable.tsx's own formatDate convention: toLocaleString(),
// no separate date-formatting library, same as every other date already
// rendered in this app.
function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

function SecuritySettingsPage() {
  const { data: session } = useSession()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">Security</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Sessions and login protection.
        </p>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Current session</CardTitle>
          <CardDescription>
            The account and session this browser is signed in with.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-1.5 text-sm">
          <p>
            <span className="text-muted-foreground">Signed in as </span>
            <span className="font-medium text-foreground">
              {session.username}
            </span>
          </p>
          <p>
            <span className="text-muted-foreground">Signed in until </span>
            <span className="font-medium text-foreground">
              {formatDate(session.expires_at)}
            </span>
          </p>
        </CardContent>
      </Card>

      <OtherSessionsCard />

      <TwoFactorCard />

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheckIcon className="size-4 text-muted-foreground" />
            Login protection
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">
            Failed sign-in attempts are rate limited per IP address and
            username, with each additional failure making the next allowed
            attempt wait longer than the last. A handful of mistyped passwords
            will never lock you out, but repeated automated guessing gets slower
            with every try.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}

// Own card, own dialog: sits between the read-only "current session"
// card and the static login-protection panel, matching
// RevokeTokenDialog.tsx's confirm-then-mutate shape for the same
// reason, this is a real, meaningful, if not strictly irreversible,
// security action (every other live session for this admin account is
// ended), not a one-click toggle.
function OtherSessionsCard() {
  const [open, setOpen] = useState(false)
  const [justRevoked, setJustRevoked] = useState(false)
  const revokeOtherSessions = useRevokeOtherSessions()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      revokeOtherSessions.reset()
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Other sessions</CardTitle>
        <CardDescription>
          Sign out every other browser or device currently signed in to this
          account, without changing your password. This browser stays signed in.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {justRevoked ? (
          <Alert>
            <CheckCircleIcon className="text-green-700 dark:text-green-400" />
            <AlertTitle>Other sessions signed out</AlertTitle>
            <AlertDescription>
              Every other session for this account has been ended. This browser
              was not affected.
            </AlertDescription>
          </Alert>
        ) : null}
        <Dialog open={open} onOpenChange={handleOpenChange}>
          <DialogTrigger render={<Button variant="outline" />}>
            <SignOutIcon />
            Sign out other sessions
          </DialogTrigger>
          <DialogContent className="sm:max-w-sm">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <WarningIcon className="size-4 text-destructive" />
                Sign out other sessions?
              </DialogTitle>
              <DialogDescription>
                Any other browser or device currently signed in to this account
                will be signed out immediately. This browser is not affected.
              </DialogDescription>
            </DialogHeader>
            {revokeOtherSessions.isError ? (
              <Alert variant="destructive">
                <WarningIcon />
                <AlertDescription>
                  {revokeOtherSessions.error.message}
                </AlertDescription>
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
                Cancel
              </Button>
              <Button
                type="button"
                disabled={revokeOtherSessions.isPending}
                onClick={() => {
                  revokeOtherSessions.mutate(undefined, {
                    onSuccess: () => {
                      setOpen(false)
                      setJustRevoked(true)
                    },
                  })
                }}
              >
                {revokeOtherSessions.isPending
                  ? 'Signing out...'
                  : 'Sign out other sessions'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  )
}

// Reads GET /api/v1/auth/2fa (works with no master key configured) and
// branches into either the enable flow or the enabled-state actions.
// Setup/confirm/disable/regenerate all 501 without a master key, but
// that's surfaced per-dialog (each mutation's own error message), not
// hidden here: an operator who hasn't configured one yet should still
// see the feature and understand why it's unavailable, not have it
// silently missing from the page.
function TwoFactorCard() {
  const { data: status } = useTwoFactorStatus()

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <LockKeyIcon className="size-4 text-muted-foreground" />
          Two-factor authentication
        </CardTitle>
        <CardDescription>
          Require a code from an authenticator app in addition to your password
          when signing in.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {status.enabled ? (
          <>
            <p className="text-sm text-muted-foreground">
              Enabled. {status.recovery_codes_remaining} recovery code
              {status.recovery_codes_remaining === 1 ? '' : 's'} remaining.
            </p>
            <div className="flex flex-wrap gap-2">
              <RegenerateRecoveryCodesDialog />
              <DisableTwoFactorDialog />
            </div>
          </>
        ) : (
          <EnableTwoFactorDialog />
        )}
      </CardContent>
    </Card>
  )
}

// A read-only value with a copy button, the same box-plus-copy-button
// shape CreateTokenDialog.tsx uses for a freshly minted token: this
// feature shows two of these (the setup key and the otpauth link) plus
// a grid of recovery codes, all copy-once-then-gone values.
function CopyableValue({ value, label }: { value: string; label: string }) {
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
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>
    </div>
  )
}

function RecoveryCodeGrid({ codes }: { codes: string[] }) {
  const { copied, copy } = useCopyToClipboard()
  return (
    <div className="space-y-2">
      <div className="grid grid-cols-2 gap-2 rounded-lg border border-input bg-muted/50 p-3 font-mono text-xs">
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
        {copied ? <CheckIcon /> : <CopyIcon />}
        {copied ? 'Copied' : 'Copy all'}
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
        <LockKeyIcon />
        Enable two-factor authentication
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {step === 'setup' ? (
          <>
            <DialogHeader>
              <DialogTitle>Set up two-factor authentication</DialogTitle>
              <DialogDescription>
                Add this to an authenticator app (1Password, Google
                Authenticator, Authy), then enter the 6-digit code it shows to
                confirm.
              </DialogDescription>
            </DialogHeader>
            {setup.isPending ? (
              <p className="text-sm text-muted-foreground">
                Generating a setup key...
              </p>
            ) : setup.isError ? (
              <Alert variant="destructive">
                <WarningIcon />
                <AlertDescription>{setup.error.message}</AlertDescription>
              </Alert>
            ) : setup.data ? (
              <form onSubmit={handleConfirm} className="space-y-3">
                <CopyableValue value={setup.data.secret} label="Setup key" />
                <CopyableValue
                  value={setup.data.provisioning_uri}
                  label="Or use this link"
                />
                <Field>
                  <FieldLabel htmlFor="totp-confirm-code">
                    Code from your authenticator app
                  </FieldLabel>
                  <Input
                    id="totp-confirm-code"
                    autoComplete="one-time-code"
                    value={code}
                    onChange={(e) => {
                      setCode(e.target.value)
                    }}
                    placeholder="123456"
                  />
                </Field>
                {confirm.isError ? (
                  <Alert variant="destructive">
                    <WarningIcon />
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
                    Cancel
                  </Button>
                  <Button
                    type="submit"
                    disabled={confirm.isPending || code === ''}
                  >
                    {confirm.isPending ? 'Verifying...' : 'Verify and enable'}
                  </Button>
                </DialogFooter>
              </form>
            ) : null}
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <CheckCircleIcon className="text-green-700 dark:text-green-400" />
                Two-factor authentication enabled
              </DialogTitle>
              <DialogDescription>
                Save these recovery codes somewhere safe. Each one can be used
                once to sign in if you lose access to your authenticator app.
                They will not be shown again.
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
                I've saved these codes
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
        <KeyIcon />
        Regenerate recovery codes
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        {step === 'confirm' ? (
          <form onSubmit={handleSubmit} className="space-y-3">
            <DialogHeader>
              <DialogTitle>Regenerate recovery codes</DialogTitle>
              <DialogDescription>
                Every existing recovery code stops working the moment new ones
                are generated. Enter a current code from your authenticator app
                to confirm.
              </DialogDescription>
            </DialogHeader>
            <Field>
              <FieldLabel htmlFor="regenerate-code">
                Code from your authenticator app
              </FieldLabel>
              <Input
                id="regenerate-code"
                autoComplete="one-time-code"
                value={code}
                onChange={(e) => {
                  setCode(e.target.value)
                }}
                placeholder="123456"
              />
            </Field>
            {regenerate.isError ? (
              <Alert variant="destructive">
                <WarningIcon />
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
                Cancel
              </Button>
              <Button
                type="submit"
                disabled={regenerate.isPending || code === ''}
              >
                {regenerate.isPending ? 'Regenerating...' : 'Regenerate'}
              </Button>
            </DialogFooter>
          </form>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <CheckCircleIcon className="text-green-700 dark:text-green-400" />
                New recovery codes generated
              </DialogTitle>
              <DialogDescription>
                Save these somewhere safe. The old codes no longer work, and
                these will not be shown again.
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
                Done
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
        Disable two-factor authentication
      </DialogTrigger>
      <DialogContent className="sm:max-w-sm">
        <form onSubmit={handleSubmit} className="space-y-3">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <WarningIcon className="size-4 text-destructive" />
              Disable two-factor authentication?
            </DialogTitle>
            <DialogDescription>
              Signing in will only require a password afterward. Enter a current
              code to confirm.
            </DialogDescription>
          </DialogHeader>
          <Field>
            <FieldLabel htmlFor="disable-code">
              {useRecoveryCode ? 'Recovery code' : 'Authenticator code'}
            </FieldLabel>
            <Input
              id="disable-code"
              autoComplete="one-time-code"
              value={code}
              onChange={(e) => {
                setCode(e.target.value)
              }}
              placeholder={useRecoveryCode ? 'xxxx-xxxx-xxxx-xxxx' : '123456'}
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
            {useRecoveryCode ? 'Use authenticator code' : 'Use a recovery code'}
          </button>
          {disable.isError ? (
            <Alert variant="destructive">
              <WarningIcon />
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
              Cancel
            </Button>
            <Button
              type="submit"
              variant="destructive"
              disabled={disable.isPending || code === ''}
            >
              {disable.isPending ? 'Disabling...' : 'Disable'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
