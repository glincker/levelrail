import { useEffect, useRef, useState } from 'react'
import {
  FingerprintPatternGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import {
  isConditionalPasskeyAvailable,
  isPasskeySupported,
  useBeginPasskeyLogin,
  useDiscoverablePasskeyLogin,
  useFinishPasskeyLogin,
  type PasskeyLoginChallenge,
} from '../queries/passkeys'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { loginOptionsQueryOptions } from '../queries/signIn'
import { LoginForm } from './LoginForm'
import { CodeLoginForm } from './CodeLoginForm'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Field, FieldLabel } from './ui/field'
import { Alert, AlertDescription } from './ui/alert'
import {
  isApprovalRequired,
  type ApprovalRequiredResponse,
  type AuthUser,
} from '../queries/auth'
import { classifyPasskeyError } from '../lib/passkeyErrors'
import { NewDeviceApprovalWait } from './NewDeviceApprovalWait'
import { ResumedApprovalWait } from './ResumedApprovalWait'

type Step =
  | { kind: 'username' }
  | { kind: 'passkey'; challenge: PasskeyLoginChallenge }
  | { kind: 'password' }
  | { kind: 'code' }
  | {
      kind: 'approval'
      id: string
      expiresAt: string
      match: number
    }
  | { kind: 'resume'; id: string }

// Progressive sign-in: a username first, then whichever method that
// account actually has, instead of a password field and a passkey
// button both shown up front. beginPasskeyLogin doubles as the
// detection call (see queries/passkeys.ts), no separate endpoint.
export function SignInFlow({
  username,
  onUsernameChange,
  resumeApprovalId,
}: {
  username: string
  onUsernameChange: (value: string) => void
  // Set when an OAuth sign-in came back held for new browser approval.
  resumeApprovalId?: string
}) {
  const [step, setStep] = useState<Step>(
    resumeApprovalId
      ? { kind: 'resume', id: resumeApprovalId }
      : { kind: 'username' },
  )
  const begin = useBeginPasskeyLogin()
  const discoverable = useDiscoverablePasskeyLogin()
  const conditionalAbort = useRef<AbortController | null>(null)
  const finish = useFinishPasskeyLogin()
  const { t } = useTranslation('signIn')
  const { t: tp } = useTranslation('passkeys')
  const options = useQuery(loginOptionsQueryOptions())
  const codeLink = options.data?.code_login ? (
    <Button
      type="button"
      variant="link"
      className="w-full text-xs text-muted-foreground"
      onClick={() => {
        setStep({ kind: 'code' })
      }}
    >
      {t('code.useCode')}
    </Button>
  ) : null

  function handleDiscoverableResult(
    result: AuthUser | ApprovalRequiredResponse,
  ) {
    if (isApprovalRequired(result)) {
      setStep({
        kind: 'approval',
        id: result.approval_id,
        expiresAt: result.approval_expires_at,
        match: result.approval_match,
      })
    }
  }

  const onUsernameStep = step.kind === 'username'
  const { mutate: runDiscoverable } = discoverable
  useEffect(() => {
    if (!onUsernameStep) {
      return
    }
    const controller = new AbortController()
    conditionalAbort.current = controller
    void isConditionalPasskeyAvailable().then((ok) => {
      if (ok && !controller.signal.aborted) {
        runDiscoverable(
          { conditional: true, signal: controller.signal },
          { onSuccess: handleDiscoverableResult },
        )
      }
    })
    return () => {
      controller.abort()
    }
  }, [onUsernameStep, runDiscoverable])

  function handlePasskeyButton() {
    conditionalAbort.current?.abort()
    runDiscoverable({}, { onSuccess: handleDiscoverableResult })
  }

  if (step.kind === 'approval' || step.kind === 'resume') {
    const back = () => {
      setStep({ kind: 'username' })
    }
    const useCode = () => {
      setStep({ kind: 'code' })
    }
    return step.kind === 'approval' ? (
      <NewDeviceApprovalWait
        approvalId={step.id}
        expiresAt={step.expiresAt}
        matchNumber={step.match}
        onBack={back}
        onUseCode={useCode}
      />
    ) : (
      <ResumedApprovalWait
        approvalId={step.id}
        onBack={back}
        onUseCode={useCode}
      />
    )
  }

  if (step.kind === 'code') {
    return (
      <CodeLoginForm
        username={username}
        onUsernameChange={onUsernameChange}
        onBack={() => {
          setStep({ kind: 'username' })
        }}
      />
    )
  }

  function handleContinue() {
    const trimmed = username.trim()
    if (trimmed === '') {
      return
    }
    if (!isPasskeySupported()) {
      setStep({ kind: 'password' })
      return
    }
    begin.mutate(trimmed, {
      onSuccess: (challenge) => {
        setStep({ kind: 'passkey', challenge })
      },
      // Any failure of the passkey check (no passkey, passkeys not configured
      // on this server, a network error) falls back to the password: a
      // passkey must never be the only way in.
      onError: () => {
        setStep({ kind: 'password' })
      },
    })
  }

  function backToUsername() {
    setStep({ kind: 'username' })
  }

  if (step.kind === 'username') {
    return (
      <div className="space-y-4">
        <Field>
          <FieldLabel htmlFor="login-username">Username</FieldLabel>
          <Input
            id="login-username"
            autoComplete="username webauthn"
            value={username}
            onChange={(e) => {
              onUsernameChange(e.target.value)
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                handleContinue()
              }
            }}
          />
        </Field>
        <Button
          type="button"
          className="w-full"
          disabled={begin.isPending || username.trim() === ''}
          onClick={handleContinue}
        >
          {begin.isPending ? 'Checking...' : 'Continue'}
        </Button>
        {isPasskeySupported() ? (
          <Button
            type="button"
            variant="outline"
            className="w-full"
            disabled={
              discoverable.isPending && !discoverable.variables?.conditional
            }
            onClick={handlePasskeyButton}
          >
            <FingerprintPatternGlassIcon size="1.2em" />
            {t('passkey.discoverable')}
          </Button>
        ) : null}
        {discoverable.isError && !discoverable.variables?.conditional ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
            />
            <AlertDescription>
              {tp(`errors.${classifyPasskeyError(discoverable.error)}`)}
            </AlertDescription>
          </Alert>
        ) : null}
        {codeLink}
      </div>
    )
  }

  if (step.kind === 'passkey') {
    const challenge = step.challenge
    return (
      <div className="space-y-3">
        {finish.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
            />
            <AlertDescription>
              {tp(`errors.${classifyPasskeyError(finish.error)}`)}
            </AlertDescription>
          </Alert>
        ) : null}
        <Button
          type="button"
          className="w-full"
          disabled={finish.isPending}
          onClick={() => {
            finish.mutate(challenge, {
              onSuccess: (result) => {
                if (isApprovalRequired(result)) {
                  setStep({
                    kind: 'approval',
                    id: result.approval_id,
                    expiresAt: result.approval_expires_at,
                    match: result.approval_match,
                  })
                }
              },
            })
          }}
        >
          <FingerprintPatternGlassIcon size="1.2em" />
          {finish.isPending
            ? 'Waiting for passkey...'
            : `Sign in as ${username.trim()}`}
        </Button>
        <Button
          type="button"
          variant="ghost"
          className="w-full"
          onClick={() => {
            setStep({ kind: 'password' })
          }}
        >
          Use your password instead
        </Button>
        <Button
          type="button"
          variant="link"
          className="w-full text-xs text-muted-foreground"
          onClick={backToUsername}
        >
          Not you?
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        Signing in as <span className="text-foreground">{username.trim()}</span>
      </p>
      <LoginForm
        username={username}
        onUsernameChange={onUsernameChange}
        showUsernameField={false}
        onUseCode={() => {
          setStep({ kind: 'code' })
        }}
        onApproval={(a) => {
          setStep({
            kind: 'approval',
            id: a.approval_id,
            expiresAt: a.approval_expires_at,
            match: a.approval_match,
          })
        }}
      />
      {codeLink}
      <Button
        type="button"
        variant="link"
        className="w-full text-xs text-muted-foreground"
        onClick={backToUsername}
      >
        Not you?
      </Button>
    </div>
  )
}
