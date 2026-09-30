import { type FormEvent, useState } from 'react'
import { FingerprintIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useLoginWithPasskey } from '../queries/passkeys'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Field, FieldLabel } from './ui/field'
import { Alert, AlertDescription } from './ui/alert'

// Username-first passkey sign-in: the operator types their username,
// then the browser's own passkey prompt (Touch ID, Windows Hello, a
// security key) proves who they are. A usernameless/discoverable flow
// (the browser suggesting a saved passkey via autofill, with no
// username typed first) is possible too, but needs its own resident-key
// ceremony on both ends; this simpler, explicit flow covers the common
// case without that added surface.
export function PasskeyLoginButton() {
  const [username, setUsername] = useState('')
  const login = useLoginWithPasskey()

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (username.trim() === '') {
      return
    }
    login.mutate(username.trim())
  }

  return (
    <form
      onSubmit={(e) => {
        handleSubmit(e)
      }}
      className="w-full max-w-sm space-y-3"
    >
      <div className="flex items-center gap-3 text-xs text-muted-foreground">
        <span className="h-px flex-1 bg-border" />
        or
        <span className="h-px flex-1 bg-border" />
      </div>
      <Field>
        <FieldLabel htmlFor="passkey-username">Username</FieldLabel>
        <Input
          id="passkey-username"
          autoComplete="username webauthn"
          value={username}
          onChange={(e) => {
            setUsername(e.target.value)
          }}
          placeholder="you@example.com"
        />
      </Field>
      {login.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{login.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <Button
        type="submit"
        variant="outline"
        className="w-full"
        disabled={login.isPending || username.trim() === ''}
      >
        <FingerprintIcon />
        {login.isPending ? 'Waiting for passkey...' : 'Sign in with a passkey'}
      </Button>
    </form>
  )
}
