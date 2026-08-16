import { useState } from 'react'
import { CheckCircleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { useForgotPassword } from '../queries/passwordReset'
import { Button } from './ui/button'
import { Input } from './ui/input'
import { Field, FieldLabel } from './ui/field'
import { Alert, AlertDescription, AlertTitle } from './ui/alert'

// The response is always the same generic success regardless of
// whether the email matched an account, so this form never tries to
// guess or validate the address beyond "non-empty".
export function ForgotPasswordForm() {
  const [email, setEmail] = useState('')
  const forgotPassword = useForgotPassword()

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault()
        forgotPassword.mutate(email)
      }}
    >
      <p className="text-sm text-muted-foreground">
        Enter your account email and, if it matches, a password reset link will
        be sent to it.
      </p>

      <Field>
        <FieldLabel htmlFor="forgot-password-email">Email</FieldLabel>
        <Input
          id="forgot-password-email"
          type="email"
          required
          autoComplete="email"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          disabled={forgotPassword.isPending || forgotPassword.isSuccess}
        />
      </Field>

      {forgotPassword.isSuccess ? (
        <Alert>
          <CheckCircleIcon />
          <AlertTitle>Check your email</AlertTitle>
          <AlertDescription>
            If that email matches an account, a reset link is on its way.
          </AlertDescription>
        </Alert>
      ) : forgotPassword.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{forgotPassword.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <Button
        type="submit"
        className="w-full"
        disabled={forgotPassword.isPending || forgotPassword.isSuccess}
      >
        {forgotPassword.isPending ? 'Sending...' : 'Send reset link'}
      </Button>
    </form>
  )
}
