import { useState } from 'react'
import { FingerprintIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import {
  isPasskeySupported,
  useBeginPasskeyLogin,
  useFinishPasskeyLogin,
  type PasskeyLoginChallenge,
} from '../queries/passkeys'
import { LoginForm } from './LoginForm'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Field, FieldLabel } from './ui/field'
import { Alert, AlertDescription } from './ui/alert'

type Step =
  | { kind: 'username' }
  | { kind: 'passkey'; challenge: PasskeyLoginChallenge }
  | { kind: 'password' }

// Progressive sign-in: a username first, then whichever method that
// account actually has, instead of a password field and a passkey
// button both shown up front. beginPasskeyLogin doubles as the
// detection call (see queries/passkeys.ts), no separate endpoint.
export function SignInFlow({
  username,
  onUsernameChange,
}: {
  username: string
  onUsernameChange: (value: string) => void
}) {
  const [step, setStep] = useState<Step>({ kind: 'username' })
  const begin = useBeginPasskeyLogin()
  const finish = useFinishPasskeyLogin()

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
      </div>
    )
  }

  if (step.kind === 'passkey') {
    const challenge = step.challenge
    return (
      <div className="space-y-3">
        {finish.isError ? (
          <Alert variant="destructive">
            <WarningIcon />
            <AlertDescription>{finish.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <Button
          type="button"
          className="w-full"
          disabled={finish.isPending}
          onClick={() => {
            finish.mutate(challenge)
          }}
        >
          <FingerprintIcon />
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
      />
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
